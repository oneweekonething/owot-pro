package integration

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestRealDependenciesReadiness 真实 MySQL / Redis 集成测试入口。
//
// 行为契约：
//  1. 默认执行 `go test ./...`（未设置 OWOT_RUN_INTEGRATION 且未设置 OWOT_TEST_MYSQL_DSN / OWOT_TEST_REDIS_ADDR）时跳过。
//  2. 一旦显式执行集成测试（设置了 OWOT_RUN_INTEGRATION=1，或设置了 OWOT_TEST_MYSQL_DSN / OWOT_TEST_REDIS_ADDR 中任意一个）：
//     - 缺少任一配置直接失败（t.Fatalf）；
//     - 使用官方成熟的 go-sql-driver/mysql 执行 PingContext 与 `SELECT 1` 查询校验；失败日志对 DSN 脱敏，绝不输出密码；
//     - 真实连接 Redis 并执行 PING / SET / GET / DEL 闭环校验。
func TestRealDependenciesReadiness(t *testing.T) {
	runFlag := strings.TrimSpace(os.Getenv("OWOT_RUN_INTEGRATION"))
	mysqlDSN := strings.TrimSpace(os.Getenv("OWOT_TEST_MYSQL_DSN"))
	redisAddr := strings.TrimSpace(os.Getenv("OWOT_TEST_REDIS_ADDR"))

	if runFlag == "" && mysqlDSN == "" && redisAddr == "" {
		t.Skip("skipping real-dependency integration test; set OWOT_RUN_INTEGRATION=1 with OWOT_TEST_MYSQL_DSN and OWOT_TEST_REDIS_ADDR")
	}

	if mysqlDSN == "" {
		t.Fatalf("OWOT_TEST_MYSQL_DSN is required when running integration tests")
	}
	if redisAddr == "" {
		t.Fatalf("OWOT_TEST_REDIS_ADDR is required when running integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := probeMySQL(ctx, mysqlDSN); err != nil {
		t.Fatalf("real MySQL probe failed for target %s: %v", redactMySQLDSN(mysqlDSN), err)
	}
	if err := probeRedis(ctx, redisAddr); err != nil {
		t.Fatalf("real Redis probe failed for addr %q: %v", redisAddr, err)
	}
}

// TestProbeRejectsInvalidEndpoints 回归验证：
//  1. 传入无效 MySQL DSN 或不可达地址时必须失败；
//  2. 模拟 caching_sha2_password 认证未完成（返回 AuthMoreData 0x01 0x04 后拒绝后续请求）时，标准驱动必须失败而不会误判成功；
//  3. 失败信息与脱敏函数绝不泄露数据库密码。
func TestProbeRejectsInvalidEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	invalidDSNs := []string{
		"",
		"invalid-dsn-without-slash",
		"root:SuperSecretPwd123!@tcp(127.0.0.1:1)/owot_pro_test",
	}
	for _, dsn := range invalidDSNs {
		err := probeMySQL(ctx, dsn)
		if err == nil {
			t.Fatalf("expected probeMySQL(%s) to fail, but got nil", redactMySQLDSN(dsn))
		}
		if strings.Contains(err.Error(), "SuperSecretPwd123!") {
			t.Fatalf("probeMySQL error leaked raw password: %v", err)
		}
	}

	// 复现 caching_sha2_password 半程握手场景：服务端发送 Initial Handshake 后返回 AuthMoreData(0x01,0x04)，随后返回 ERR 包
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake mysql: %v", err)
	}
	defer func() { _ = ln.Close() }()

	go serveIncompleteCachingSHA2MySQL(ln)

	fakeDSN := fmt.Sprintf("root:SecretPass@tcp(%s)/owot_pro_test?timeout=1s", ln.Addr().String())
	if err := probeMySQL(ctx, fakeDSN); err == nil {
		t.Fatal("expected probeMySQL to fail when caching_sha2_password authentication is incomplete, but got nil")
	}

	invalidRedis := []string{
		"",
		"not-a-host-port",
		"127.0.0.1:1",
	}
	for _, addr := range invalidRedis {
		if err := probeRedis(ctx, addr); err == nil {
			t.Fatalf("expected probeRedis(%q) to fail, but got nil", addr)
		}
	}
}

func TestRedactMySQLDSNHidesPassword(t *testing.T) {
	raw := "admin:MyTopSecret#2026@tcp(10.0.0.8:3306)/owot_prod?charset=utf8mb4&parseTime=true"
	redacted := redactMySQLDSN(raw)
	if strings.Contains(redacted, "MyTopSecret#2026") {
		t.Fatalf("redacted DSN still contains secret password: %s", redacted)
	}
	if !strings.Contains(redacted, "10.0.0.8:3306") || !strings.Contains(redacted, "owot_prod") {
		t.Fatalf("redacted DSN lost non-sensitive target info: %s", redacted)
	}

	// 即使是非法格式的 DSN（包括密码自身含 '@' 且参数错误导致 ParseDSN 失败），也绝不能泄露任何密码片段
	for _, malformed := range []string{
		"root:LeakedPass123@invalid-host-format",
		"review:Prefix@ReviewSecretTail@tcp(127.0.0.1:3306)/db?timeout=invalid",
	} {
		got := redactMySQLDSN(malformed)
		if got != "<invalid-mysql-dsn>" {
			t.Fatalf("expected fixed placeholder for malformed DSN, got: %s", got)
		}
		if strings.Contains(got, "LeakedPass123") || strings.Contains(got, "ReviewSecretTail") {
			t.Fatalf("redacted malformed DSN still contains secret password: %s", got)
		}
	}
}

// redactMySQLDSN 提取非敏感连接目标信息（user:***@net(addr)/dbname）；解析失败时直接返回固定占位符，杜绝包含 '@' 的密码泄露。
func redactMySQLDSN(dsn string) string {
	if cfg, err := mysql.ParseDSN(dsn); err == nil {
		return fmt.Sprintf("%s:***@%s(%s)/%s", cfg.User, cfg.Net, cfg.Addr, cfg.DBName)
	}
	return "<invalid-mysql-dsn>"
}

// probeMySQL 使用标准 go-sql-driver/mysql 完成连接、完整认证协议及 `SELECT 1` 探活。
func probeMySQL(ctx context.Context, dsn string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid MySQL DSN configuration: %w", err)
	}
	if cfg.Net != "tcp" && cfg.Net != "unix" {
		return fmt.Errorf("unsupported MySQL network %q", cfg.Net)
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("MySQL address is empty")
	}
	if strings.TrimSpace(cfg.User) == "" {
		return fmt.Errorf("MySQL user is empty")
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return fmt.Errorf("MySQL database name is empty")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return fmt.Errorf("create MySQL connector: %w", err)
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql (%s/%s): %w", cfg.Addr, cfg.DBName, err)
	}

	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("query SELECT 1 on mysql (%s/%s): %w", cfg.Addr, cfg.DBName, err)
	}
	if one != 1 {
		return fmt.Errorf("unexpected SELECT 1 result: %d", one)
	}
	return nil
}

// serveIncompleteCachingSHA2MySQL 模拟服务端返回 caching_sha2_password fast-auth 0x01 0x03 + OK，
// 但随后对任何命令（如 COM_PING / SELECT 1）返回 0xff 错误包，用于回归测试绝不把认证包当成探活成功响应。
func serveIncompleteCachingSHA2MySQL(ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	// 构造最小合法的 HandshakeV10 (protocol=10, version="8.0.32\x00",plugin="caching_sha2_password\x00")
	var hs []byte
	hs = append(hs, 0x0a)
	hs = append(hs, []byte("8.0.32\x00")...)
	hs = append(hs, 1, 0, 0, 0)                    // connection id
	hs = append(hs, []byte("12345678")...)         // auth-plugin-data-part-1 (8 bytes)
	hs = append(hs, 0x00)                          // filler
	hs = append(hs, 0xff, 0xf7)                    // capability flags lower 2 bytes
	hs = append(hs, 45)                            // charset utf8mb4
	hs = append(hs, 0x02, 0x00)                    // status flags
	hs = append(hs, 0xff, 0x81)                    // capability flags upper 2 bytes (CLIENT_PLUGIN_AUTH)
	hs = append(hs, 21)                            // auth plugin data len
	hs = append(hs, make([]byte, 10)...)           // reserved
	hs = append(hs, []byte("123456789012\x00")...) // auth-plugin-data-part-2 (13 bytes)
	hs = append(hs, []byte("caching_sha2_password\x00")...)
	_ = writePacket(conn, 0, hs)

	// 读取客户端 HandshakeResponse41
	if _, _, err := readPacket(conn); err != nil {
		return
	}
	// 发送 AuthMoreData (0x01, 0x03 = fast auth success) 与随后的认证 OK 包 (0x00)，但紧接着对下一条探活命令返回 ERR (0xff)
	_ = writePacket(conn, 2, []byte{0x01, 0x03})
	_ = writePacket(conn, 3, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00})

	if _, seq, err := readPacket(conn); err == nil {
		errPayload := append([]byte{0xff, 0x15, 0x04}, []byte("#28000Access denied / command rejected")...)
		_ = writePacket(conn, seq+1, errPayload)
	}
}

func readPacket(r io.Reader) ([]byte, byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, 0, err
	}
	length := int(uint32(hdr[0]) | uint32(hdr[1])<<8 | uint32(hdr[2])<<16)
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, hdr[3], err
	}
	return body, hdr[3], nil
}

func writePacket(w io.Writer, seq byte, payload []byte) error {
	length := len(payload)
	hdr := []byte{byte(length), byte(length >> 8), byte(length >> 16), seq}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// probeRedis 通过 RESP 协议执行真实 TCP 连接、PING、SET、GET、DEL 校验。
func probeRedis(ctx context.Context, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("invalid redis address %q: %w", addr, err)
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial redis %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}

	rd := bufio.NewReader(conn)

	// 1. PING -> +PONG
	if _, err := io.WriteString(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		return fmt.Errorf("send PING: %w", err)
	}
	line, err := rd.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read PING reply: %w", err)
	}
	if strings.TrimSpace(line) != "+PONG" {
		return fmt.Errorf("unexpected PING reply: %q", line)
	}

	// 2. SET owot:integration:probe ok PX 5000 -> +OK
	const key = "owot:integration:probe"
	const val = "ok"
	setCmd := fmt.Sprintf("*5\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n$2\r\nPX\r\n$4\r\n5000\r\n", len(key), key, len(val), val)
	if _, err := io.WriteString(conn, setCmd); err != nil {
		return fmt.Errorf("send SET: %w", err)
	}
	line, err = rd.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read SET reply: %w", err)
	}
	if strings.TrimSpace(line) != "+OK" {
		return fmt.Errorf("unexpected SET reply: %q", line)
	}

	// 3. GET owot:integration:probe -> $2\r\nok\r\n
	getCmd := fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key)
	if _, err := io.WriteString(conn, getCmd); err != nil {
		return fmt.Errorf("send GET: %w", err)
	}
	hdr, err := rd.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read GET header: %w", err)
	}
	if strings.TrimSpace(hdr) != "$2" {
		return fmt.Errorf("unexpected GET bulk header: %q", hdr)
	}
	body, err := rd.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read GET body: %w", err)
	}
	if strings.TrimSpace(body) != val {
		return fmt.Errorf("unexpected GET body: %q", body)
	}

	// 4. DEL owot:integration:probe -> :1
	delCmd := fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(key), key)
	if _, err := io.WriteString(conn, delCmd); err != nil {
		return fmt.Errorf("send DEL: %w", err)
	}
	delReply, err := rd.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read DEL reply: %w", err)
	}
	if strings.TrimSpace(delReply) != ":1" {
		return fmt.Errorf("unexpected DEL reply: %q", delReply)
	}

	return nil
}
