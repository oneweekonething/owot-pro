package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func realSystemDependencies(t *testing.T) (*gorm.DB, *redis.Client) {
	t.Helper()
	dsn, addr := os.Getenv("OWOT_TEST_MYSQL_DSN"), os.Getenv("OWOT_TEST_REDIS_ADDR")
	if dsn == "" && addr == "" && os.Getenv("OWOT_RUN_INTEGRATION") == "" {
		t.Skip("real System integration requires MySQL and Redis configuration")
	}
	if dsn == "" || addr == "" {
		t.Fatal("OWOT_TEST_MYSQL_DSN and OWOT_TEST_REDIS_ADDR are both required")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid test MySQL DSN")
	}
	config.ParseTime, config.Loc, config.Timeout = true, time.UTC, 3*time.Second
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{TranslateError: true,
		Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }})
	if err != nil {
		t.Fatalf("open MySQL %s: %v", redactMySQLDSN(dsn), err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := dbx.Install(db); err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../../migrations/mysql/001_system.sql")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(ddl), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(statement) != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatalf("apply System migration: %v", err)
			}
		}
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("OWOT_TEST_REDIS_PASSWORD"), ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	return db, client
}

type controlledTypeLocks struct {
	app.DictTypeRepository
	onAttempt func(string)
	onLocked  func(string)
}

func (r *controlledTypeLocks) LockType(ctx context.Context, typ string) (domain.DictType, error) {
	if r.onAttempt != nil {
		r.onAttempt(typ)
	}
	value, err := r.DictTypeRepository.LockType(ctx, typ)
	if err == nil && r.onLocked != nil {
		r.onLocked(typ)
	}
	return value, err
}

func TestDictionaryBatchDeleteAndMoveLockOrder(t *testing.T) {
	db, client := realSystemDependencies(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	prefix := fmt.Sprintf("owot_lock_%d", time.Now().UnixNano())
	t.Cleanup(func() {
		for _, statement := range []string{
			"DELETE FROM system_dict_data WHERE dict_type LIKE ?",
			"DELETE FROM system_dict_type WHERE type LIKE ?",
			"DELETE FROM sys_outbox_event WHERE payload LIKE ?",
		} {
			if err := db.Exec(statement, "%"+prefix+"%").Error; err != nil {
				t.Error(err)
			}
		}
	})
	repo := persistence.NewDictRepository(db)
	tx := dbx.NewTxManager(db)
	store := outbox.NewStore(db, outbox.NewRedisBroadcaster(client), slog.New(slog.NewTextHandler(io.Discard, nil)))
	service := app.NewDictService(repo, repo, tx, store)
	// IDs and lexical type order intentionally disagree.
	z, err := service.CreateType(ctx, domain.DictType{Name: "Z", Type: prefix + "_z"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := service.CreateType(ctx, domain.DictType{Name: "A", Type: prefix + "_a"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := service.CreateData(ctx, domain.DictData{DictType: prefix + "_a", Label: "item", Value: "0"})
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan string, 1)
	release := make(chan struct{})
	var releaseOnce, firstLock sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	deleteRepo := &controlledTypeLocks{DictTypeRepository: repo, onLocked: func(typ string) {
		firstLock.Do(func() {
			locked <- typ
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
	}}
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- app.NewDictService(deleteRepo, repo, tx, store).DeleteTypes(ctx, []int64{z, a}) }()
	select {
	case first := <-locked:
		if first != prefix+"_a" {
			t.Errorf("batch deletion locked by ID instead of type: %s", first)
		}
	case err := <-deleteDone:
		t.Fatalf("deletion failed before locking: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	attempted := make(chan struct{})
	var firstAttempt sync.Once
	moveRepo := &controlledTypeLocks{DictTypeRepository: repo, onAttempt: func(string) {
		firstAttempt.Do(func() { close(attempted) })
	}}
	moveDone := make(chan error, 1)
	go func() {
		moveDone <- app.NewDictService(moveRepo, repo, tx, store).UpdateData(ctx,
			domain.DictData{ID: item, DictType: prefix + "_z", Label: "moved", Value: "0"})
	}()
	select {
	case <-attempted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-moveDone:
		t.Fatalf("move bypassed held parent lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	if err := <-deleteDone; !errors.Is(err, domain.ErrDictTypeHasData) {
		t.Fatalf("batch delete should reject nonempty type without deadlocking: %v", err)
	}
	if err := <-moveDone; err != nil {
		t.Fatalf("move failed after deletion released locks: %v", err)
	}
	value, err := service.GetData(ctx, item)
	if err != nil || value.DictType != prefix+"_z" {
		t.Fatalf("move result: %+v, %v", value, err)
	}
}

type unavailablePublisher struct{}

func (unavailablePublisher) Publish(context.Context, string, []byte) error {
	return errors.New("outbox insert unavailable")
}

type flakyBroadcast struct {
	fail bool
	next outbox.Broadcaster
}

func (b *flakyBroadcast) Broadcast(ctx context.Context, value outbox.Event) error {
	if b.fail {
		return errors.New("simulated Redis disconnection")
	}
	return b.next.Broadcast(ctx, value)
}

func TestSystemRealMySQLAndRedis(t *testing.T) {
	db, client := realSystemDependencies(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var version string
	if err := db.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("MySQL version: %s", version)
	prefix := fmt.Sprintf("owot_it_%d", time.Now().UnixNano())
	tenantA, tenantB := time.Now().UnixNano()%1_000_000_000+1_000_000_000, time.Now().UnixNano()%1_000_000_000+2_000_000_000
	t.Cleanup(func() {
		for _, statement := range []string{
			"DELETE FROM system_dict_data WHERE dict_type LIKE ?",
			"DELETE FROM system_dict_type WHERE type LIKE ?",
			"DELETE FROM sys_outbox_event WHERE payload LIKE ?",
		} {
			if err := db.Exec(statement, "%"+prefix+"%").Error; err != nil {
				t.Error(err)
			}
		}
		for _, table := range []string{"system_notice", "system_operate_log", "system_login_log"} {
			if err := db.Exec("DELETE FROM "+table+" WHERE tenant_id IN (?, ?)", tenantA, tenantB).Error; err != nil {
				t.Error(err)
			}
		}
	})
	broadcast := &flakyBroadcast{next: outbox.NewRedisBroadcaster(client)}
	store := outbox.NewStore(db, broadcast, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tx := dbx.NewTxManager(db)
	repo := persistence.NewDictRepository(db)
	dicts := app.NewDictService(repo, repo, tx, store)
	createType := func(t *testing.T, typ string) int64 {
		t.Helper()
		id, err := dicts.CreateType(ctx, domain.DictType{Name: "真实集成", Type: typ})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	t.Run("rollback and uniqueness", func(t *testing.T) {
		failing := app.NewDictService(repo, repo, tx, unavailablePublisher{})
		if _, err := failing.CreateType(ctx, domain.DictType{Name: "回滚", Type: prefix + "_rollback"}); err == nil {
			t.Fatal("outbox failure ignored")
		}
		result, err := dicts.PageTypes(ctx, app.DictTypeFilter{Type: prefix + "_rollback"})
		if err != nil || result.Total != 0 {
			t.Fatalf("transaction did not roll back: %+v, %v", result, err)
		}
		typ := prefix + "_unique"
		id := createType(t, typ)
		if _, err := dicts.CreateType(ctx, domain.DictType{Name: "重复", Type: typ}); !errors.Is(err, domain.ErrDictTypeExists) {
			t.Fatalf("unique constraint mapping = %v", err)
		}
		for range 3 {
			item, err := dicts.CreateData(ctx, domain.DictData{Label: "零", Value: "0", DictType: typ})
			if err != nil {
				t.Fatal(err)
			}
			if err := dicts.DeleteTypes(ctx, []int64{id}); !errors.Is(err, domain.ErrDictTypeHasData) {
				t.Fatalf("nonempty dictionary deleted: %v", err)
			}
			if err := dicts.DeleteData(ctx, []int64{item}); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("parent row lock prevents orphaned items", func(t *testing.T) {
		typ := prefix + "_locking"
		id := createType(t, typ)
		locked, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- tx.WithinTx(ctx, func(ctx context.Context) error {
				if _, err := repo.LockType(ctx, typ); err != nil {
					return err
				}
				close(locked)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				value := domain.DictData{Label: "并发项", Value: "0", DictType: typ}
				if err := repo.SaveData(ctx, &value); err != nil {
					return err
				}
				return store.Publish(ctx, "system.dict.invalidate", []byte(fmt.Sprintf(`{"dictType":%q}`, typ)))
			})
		}()
		select {
		case <-locked:
		case err := <-done:
			t.Fatalf("row lock failed: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		deleting := make(chan error, 1)
		go func() { deleting <- dicts.DeleteTypes(ctx, []int64{id}) }()
		select {
		case err := <-deleting:
			close(release)
			t.Fatalf("delete bypassed parent row lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := <-deleting; !errors.Is(err, domain.ErrDictTypeHasData) {
			t.Fatalf("delete ignored freshly committed child under REPEATABLE READ: %v", err)
		}
	})

	t.Run("tenant isolation and audit", func(t *testing.T) {
		ctxA := ctxmeta.WithLoginUser(ctxmeta.WithTenantID(ctx, tenantA), ctxmeta.LoginUser{ID: 7, TenantID: tenantA, UserType: 1})
		ctxB := ctxmeta.WithTenantID(ctx, tenantB)
		notices := app.NewNoticeService(persistence.NewNoticeRepository(db))
		id, err := notices.Create(ctxA, domain.Notice{Title: "租户公告", Content: "内容", Type: 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := notices.Get(ctxB, id); !errors.Is(err, domain.ErrNoticeNotFound) {
			t.Fatalf("cross-tenant read: %v", err)
		}
		if err := notices.Delete(ctxB, id); !errors.Is(err, domain.ErrNoticeNotFound) {
			t.Fatalf("cross-tenant delete: %v", err)
		}
		if err := notices.Update(ctxA, domain.Notice{ID: id, Title: "租户公告", Content: "内容", Type: 2}); err != nil {
			t.Fatalf("no-op update falsely reported missing: %v", err)
		}
		if err := db.Model(&struct{ TenantID int64 }{}).Table("system_notice").Count(new(int64)).Error; !errors.Is(err, dbx.ErrTenantRequired) {
			t.Fatalf("query without tenant context: %v", err)
		}
	})

	t.Run("durable relay recovery and Redis broadcast", func(t *testing.T) {
		broadcast.fail = true
		typ := prefix + "_relay"
		createType(t, typ) // Commit succeeds even when immediate delivery fails.
		var status struct {
			ID                 int64
			Status, RetryCount int
		}
		if err := db.Table("sys_outbox_event").Where("payload LIKE ?", "%"+typ+"%").Take(&status).Error; err != nil || status.Status != 0 || status.RetryCount != 1 {
			t.Fatalf("failed broadcast was not retained: %+v, %v", status, err)
		}
		before, err := client.Get(ctx, "system.dict.invalidate:version:0").Int64()
		if err != nil {
			t.Fatal(err)
		}
		sub := client.Subscribe(ctx, "system.dict.invalidate")
		defer func() { _ = sub.Close() }()
		if _, err := sub.Receive(ctx); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE sys_outbox_event SET next_retry_time = ? WHERE id = ?", time.Now().UTC().Add(-time.Second), status.ID).Error; err != nil {
			t.Fatal(err)
		}
		broadcast.fail = false
		if err := store.Flush(ctx, 100); err != nil {
			t.Fatal(err)
		}
		if err := db.Table("sys_outbox_event").Where("id = ?", status.ID).Take(&status).Error; err != nil || status.Status != 1 {
			t.Fatalf("relay did not publish: %+v, %v", status, err)
		}
		after, err := client.Get(ctx, "system.dict.invalidate:version:0").Int64()
		if err != nil || after <= before {
			t.Fatalf("Redis version did not advance: %d -> %d, %v", before, after, err)
		}
		readCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		if _, err := sub.ReceiveMessage(readCtx); err != nil {
			t.Fatalf("Redis Pub/Sub replay missing: %v", err)
		}
	})
}
