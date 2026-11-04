package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/testhelper"
	"oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

// These security ports belong exclusively to tests; production has no static-token bypass.
type testSecurity struct{}

func (testSecurity) Verify(_ context.Context, token string) (ctxmeta.LoginUser, error) {
	switch token {
	case "tenant-a":
		return ctxmeta.LoginUser{ID: 7, TenantID: 1, UserType: 1}, nil
	case "tenant-b":
		return ctxmeta.LoginUser{ID: 9, TenantID: 2, UserType: 1}, nil
	case "denied":
		return ctxmeta.LoginUser{ID: 8, TenantID: 1, UserType: 1}, nil
	case "super":
		return ctxmeta.LoginUser{ID: 1, TenantID: 1, UserType: 1, SuperAdmin: true}, nil
	case "member":
		return ctxmeta.LoginUser{ID: 3, TenantID: 1, UserType: 2}, nil
	}
	return ctxmeta.LoginUser{}, errors.New("invalid token")
}

func (testSecurity) HasPermission(_ context.Context, user ctxmeta.LoginUser, _ string) (bool, error) {
	return user.ID != 8, nil
}

func (testSecurity) ValidateTenant(_ context.Context, id int64) error {
	if id != 1 && id != 2 {
		return errors.New("tenant unavailable")
	}
	return nil
}

type testNicknames struct{}

func (testNicknames) Nicknames(ctx context.Context, ids []int64) (map[int64]string, error) {
	result := map[int64]string{}
	for _, id := range ids {
		if ctxmeta.TenantID(ctx) == 1 && id == 7 {
			result[id] = "小王"
		}
	}
	return result, nil
}

func (testNicknames) FindUserIDsByNickname(ctx context.Context, name string) ([]int64, error) {
	if ctxmeta.TenantID(ctx) == 1 && strings.Contains("小王", name) {
		return []int64{7}, nil
	}
	return []int64{}, nil
}

type systemFixture struct {
	db      *gorm.DB
	handler http.Handler
	dict    *app.DictService
	audit   *app.AuditLogService
}

func newSystemFixture(t *testing.T) systemFixture {
	t.Helper()
	db := testhelper.SQLite(t)
	if err := errors.Join(persistence.Migrate(db), outbox.Migrate(db)); err != nil {
		t.Fatal(err)
	}
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := outbox.NewStore(db, outbox.NewRedisBroadcaster(client), log)
	security := testSecurity{}
	deps := httpx.Dependencies{Tokens: security, Permissions: security, Tenants: security, Logger: log}
	dicts := persistence.NewDictRepository(db)
	logs := persistence.NewAuditLogRepository(db)
	return systemFixture{db: db, handler: newSystemHandler(db, store, deps, testNicknames{}),
		dict: app.NewDictService(dicts, dicts, dbx.NewTxManager(db), store), audit: app.NewAuditLogService(logs, logs, testNicknames{})}
}

func request(t *testing.T, handler http.Handler, method, path, token, tenant, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenant != "" {
		req.Header.Set("tenant-id", tenant)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("HTTP status = %d: %s", w.Code, w.Body.String())
	}
	if len(w.Header().Get("trace-id")) != 32 {
		t.Fatalf("missing trace ID: %s", w.Header().Get("trace-id"))
	}
	return w
}

func response[T any](t *testing.T, w *httptest.ResponseRecorder, wantCode int) T {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Code != wantCode {
		t.Fatalf("response = %s; want code %d; decode error %v", w.Body.String(), wantCode, err)
	}
	var value T
	if wantCode == 0 {
		if err := json.Unmarshal(envelope.Data, &value); err != nil {
			t.Fatal(err)
		}
	}
	return value
}

func TestSystemSecurityAndProtocol(t *testing.T) {
	f := newSystemFixture(t)
	for _, tt := range []struct {
		path, token, tenant string
		code                int
	}{
		{"/admin-api/system/notice/page", "", "", 401},
		{"/admin-api/system/notice/page", "bad", "1", 401},
		{"/admin-api/system/notice/page", "tenant-a", "", 400},
		{"/admin-api/system/notice/page", "tenant-a", "2", 403},
		{"/admin-api/system/notice/page", "member", "1", 401},
		{"/admin-api/system/notice/page", "denied", "1", 403},
		{"/admin-api/system/notice/page?pageNo=bad", "tenant-a", "1", 400},
		{"/admin-api/system/notice/page?status=2", "tenant-a", "1", 400},
		{"/admin-api/system/dict-type/page?createTime%5B0%5D=bad", "tenant-a", "1", 400},
		{"/admin-api/system/login-log/page?status=bad", "tenant-a", "1", 400},
		{"/app-api/system/dict-data/type", "", "", 400},
		{"/not-found", "", "", 404},
	} {
		response[any](t, request(t, f.handler, "GET", tt.path, tt.token, tt.tenant, ""), tt.code)
	}
	w := request(t, f.handler, "GET", "/admin-api/system/notice/page", "tenant-a", "1", "")
	if !strings.Contains(w.Body.String(), `"list":[]`) {
		t.Fatalf("empty page is not []: %s", w.Body.String())
	}
	response[any](t, request(t, f.handler, "POST", "/admin-api/system/notice/page", "tenant-a", "1", ""), 405)
	for _, resource := range []string{"dict-type", "dict-data"} {
		for _, action := range []string{"list-all-simple", "simple-list"} {
			value := response[[]any](t, request(t, f.handler, "GET", "/admin-api/system/"+resource+"/"+action, "tenant-a", "1", ""), 0)
			if value == nil {
				t.Fatal("simple list is null")
			}
		}
	}
	// Production wiring without IAM fails closed even if a caller invents a token.
	closed := newSystemHandler(f.db, nil, httpx.Dependencies{}, nil)
	response[any](t, request(t, closed, "GET", "/admin-api/system/notice/page", "tenant-a", "1", ""), 401)
}

func TestDictionaryLifecycleAndExports(t *testing.T) {
	f := newSystemFixture(t)
	call := func(method, path, body string, code int) *httptest.ResponseRecorder {
		w := request(t, f.handler, method, "/admin-api/system/"+path, "tenant-a", "1", body)
		if code != -1 {
			response[any](t, w, code)
		}
		return w
	}
	typeID := response[int64](t, call("POST", "dict-type/create", `{"name":"通用状态","type":"status","status":0}`, 0), 0)
	call("POST", "dict-type/create", `{"name":"重复","type":"status","status":0}`, 1_002_006_001)
	value := response[map[string]any](t, call("GET", fmt.Sprintf("dict-type/get?id=%d", typeID), "", 0), 0)
	if timestamp, ok := value["createTime"].(float64); !ok || timestamp < 1e12 {
		t.Fatalf("timestamp is not milliseconds: %v", value["createTime"])
	}
	call("PUT", "dict-type/update", fmt.Sprintf(`{"id":%d,"name":"状态","type":"changed","status":0}`, typeID), 1_002_006_002)
	dataID := response[int64](t, call("POST", "dict-data/create", `{"label":"开启","value":"0","dictType":"status","status":0,"sort":10}`, 0), 0)
	call("POST", "dict-data/create", `{"label":"重复","value":"0","dictType":"status","status":0}`, 1_002_006_005)
	call("POST", "dict-data/create", `{"label":"无父类型","value":"x","dictType":"missing","status":0}`, 1_002_006_000)
	call("DELETE", fmt.Sprintf("dict-type/delete?id=%d", typeID), "", 1_002_006_003)
	disabledID := response[int64](t, call("POST", "dict-data/create", `{"label":"关闭","value":"1","dictType":"status","status":1}`, 0), 0)
	public := func() []map[string]any {
		return response[[]map[string]any](t, request(t, f.handler, "GET", "/app-api/system/dict-data/type?type=status", "", "", ""), 0)
	}
	if items := public(); len(items) != 1 || items[0]["value"] != "0" {
		t.Fatalf("public dictionary contains disabled items: %+v", items)
	}
	call("PUT", "dict-type/update", fmt.Sprintf(`{"id":%d,"name":"状态","status":1}`, typeID), 0)
	if items := public(); len(items) != 0 {
		t.Fatalf("disabled type remains public: %+v", items)
	}
	call("PUT", "dict-type/update", fmt.Sprintf(`{"id":%d,"name":"状态","status":0}`, typeID), 0)
	call("PUT", "dict-data/update", fmt.Sprintf(`{"id":%d,"label":"启用","value":"0","dictType":"status","status":0,"sort":0}`, dataID), 0)
	if label, err := f.dict.GetLabel(context.Background(), "status", "0"); err != nil || label != "启用" {
		t.Fatalf("label = %q, %v", label, err)
	}
	if value, err := f.dict.GetValue(context.Background(), "status", "启用"); err != nil || value != "0" {
		t.Fatalf("value = %q, %v", value, err)
	}
	call("DELETE", fmt.Sprintf("dict-data/delete-list?ids=%d&ids=999999", dataID), "", 1_002_006_004)
	call("GET", fmt.Sprintf("dict-data/get?id=%d", dataID), "", 0) // batch failure rolled back the first deletion
	w := call("GET", "dict-data/export?dictType=status&pageSize=1", "", -1)
	book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("invalid XLSX: %v: %s", err, w.Body.String())
	}
	defer func() { _ = book.Close() }()
	rows, err := book.GetRows("Sheet1")
	if err != nil || len(rows) != 3 || rows[1][1] != "启用" {
		t.Fatalf("XLSX export ignored filter/order/all rows: %v, %v", rows, err)
	}
	call("DELETE", fmt.Sprintf("dict-data/delete-list?ids=%d,%d", dataID, disabledID), "", 0)
	call("DELETE", fmt.Sprintf("dict-type/delete?id=%d", typeID), "", 0)
	// Reuse a soft-deleted key more than once; a (key, deleted) unique index would fail here.
	for range 3 {
		id := response[int64](t, call("POST", "dict-type/create", `{"name":"状态","type":"status","status":0}`, 0), 0)
		call("DELETE", fmt.Sprintf("dict-type/delete?id=%d", id), "", 0)
	}
	var published int64
	if err := f.db.Table("sys_outbox_event").Where("status = 1 AND tenant_id = 0").Count(&published).Error; err != nil || published == 0 {
		t.Fatalf("global dictionary events were not published: %d, %v", published, err)
	}
}

func TestNoticeTenantIsolationAndOperationAudit(t *testing.T) {
	f := newSystemFixture(t)
	id := response[int64](t, request(t, f.handler, "POST", "/admin-api/system/notice/create", "tenant-a", "1",
		`{"title":"公告","type":2,"content":"内容","status":0,"tenantId":2}`), 0)
	path := fmt.Sprintf("/admin-api/system/notice/get?id=%d", id)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		endpoint, body := path, ""
		switch method {
		case "PUT":
			endpoint = "/admin-api/system/notice/update"
			body = fmt.Sprintf(`{"id":%d,"title":"越权","type":1,"content":"changed","status":1}`, id)
		case "DELETE":
			endpoint = fmt.Sprintf("/admin-api/system/notice/delete?id=%d", id)
		}
		response[any](t, request(t, f.handler, method, endpoint, "tenant-b", "2", body), 1_002_008_000)
	}
	pageB := response[pagination.PageResult[map[string]any]](t, request(t, f.handler, "GET", "/admin-api/system/notice/page", "tenant-b", "2", ""), 0)
	if pageB.Total != 0 || len(pageB.List) != 0 {
		t.Fatal("tenant-a notices leaked to tenant-b")
	}
	value := response[map[string]any](t, request(t, f.handler, "GET", path, "tenant-a", "1", ""), 0)
	if value["creator"] != "7" || value["title"] != "公告" {
		t.Fatalf("tenant/audit fields missing: %v", value)
	}
	response[bool](t, request(t, f.handler, "PUT", "/admin-api/system/notice/update", "tenant-a", "1",
		fmt.Sprintf(`{"id":%d,"title":"修改","type":1,"content":"新内容","status":1}`, id)), 0)
	response[bool](t, request(t, f.handler, "PUT", "/admin-api/system/notice/update", "tenant-a", "1",
		fmt.Sprintf(`{"id":%d,"title":"修改","type":1,"content":"新内容","status":0}`, id)), 0)
	value = response[map[string]any](t, request(t, f.handler, "GET", path, "tenant-a", "1", ""), 0)
	if value["status"] != float64(0) {
		t.Fatal("zero-valued enabled status was not persisted")
	}
	logs := response[pagination.PageResult[map[string]any]](t, request(t, f.handler, "GET", "/admin-api/system/operate-log/page?type=notice", "tenant-a", "1", ""), 0)
	if logs.Total != 3 || logs.List[0]["userName"] != "小王" || logs.List[0]["bizId"] != float64(id) {
		t.Fatalf("operation audit missing or failed requests audited as success: %+v", logs)
	}
	response[bool](t, request(t, f.handler, "DELETE", fmt.Sprintf("/admin-api/system/notice/delete?id=%d", id), "tenant-a", "1", ""), 0)
	response[any](t, request(t, f.handler, "GET", path, "tenant-a", "1", ""), 1_002_008_000)
}

type rejectingOutbox struct{}

func (rejectingOutbox) Publish(context.Context, string, []byte) error {
	return errors.New("outbox insert failed")
}

func TestDictionaryOutboxFailureRollsBackBusinessMutation(t *testing.T) {
	f := newSystemFixture(t)
	repo := persistence.NewDictRepository(f.db)
	service := app.NewDictService(repo, repo, dbx.NewTxManager(f.db), rejectingOutbox{})
	if _, err := service.CreateType(context.Background(), domain.DictType{Name: "回滚", Type: "rollback"}); err == nil {
		t.Fatal("outbox failure was ignored")
	}
	result, err := f.dict.PageTypes(context.Background(), app.DictTypeFilter{Type: "rollback"})
	if err != nil || result.Total != 0 {
		t.Fatalf("business data committed without event: %+v, %v", result, err)
	}
}

func TestAuditQueriesAndExports(t *testing.T) {
	f := newSystemFixture(t)
	ctxA := ctxmeta.WithTraceID(ctxmeta.WithTenantID(context.Background(), 1), "0123456789abcdef0123456789abcdef")
	ctxB := ctxmeta.WithTenantID(context.Background(), 2)
	for _, ctx := range []context.Context{ctxA, ctxB} {
		if err := f.audit.RecordOperateLog(ctx, domain.OperateLog{UserID: 7, UserType: 1, Type: "字典", SubType: "新增", Action: "=1+1"}); err != nil {
			t.Fatal(err)
		}
		for _, result := range []int{0, 10, 20} {
			if err := f.audit.RecordLoginLog(ctx, domain.LoginLog{Username: "user", UserID: 7, UserType: 1, LogType: 100, Result: result}); err != nil {
				t.Fatal(err)
			}
		}
	}
	query := "/admin-api/system/operate-log/page?userNickname=" + url.QueryEscape("小王")
	result := response[pagination.PageResult[map[string]any]](t, request(t, f.handler, "GET", query, "tenant-a", "1", ""), 0)
	if result.Total != 1 || result.List[0]["traceId"] != ctxmeta.TraceID(ctxA) {
		t.Fatalf("nickname/tenant/trace query failed: %+v", result)
	}
	for _, tt := range []struct {
		query string
		total int64
	}{
		{"status=true", 1}, {"status=false", 2}, {"username=missing", 0},
		{"createTime%5B0%5D=4102444800000", 0},
		{"createTime%5B0%5D=2000-01-01+00:00:00&createTime%5B1%5D=2100-01-01+00:00:00", 3},
	} {
		result := response[pagination.PageResult[map[string]any]](t, request(t, f.handler, "GET", "/admin-api/system/login-log/page?"+tt.query, "tenant-a", "1", ""), 0)
		if result.Total != tt.total {
			t.Fatalf("filter %q: total=%d want=%d", tt.query, result.Total, tt.total)
		}
	}
	w := request(t, f.handler, "GET", "/admin-api/system/operate-log/export", "tenant-a", "1", "")
	book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = book.Close() }()
	if formula, err := book.GetCellFormula("Sheet1", "G2"); err != nil || formula != "" {
		t.Fatalf("untrusted action became spreadsheet formula: %q, %v", formula, err)
	}
	if value, _ := book.GetCellValue("Sheet1", "G2"); value != "=1+1" {
		t.Fatalf("action missing in export: %q", value)
	}
	if err := f.audit.RecordLoginLog(context.Background(), domain.LoginLog{Username: "user", UserType: 1, LogType: 100}); !errors.Is(err, domain.ErrTenantRequired) {
		t.Fatalf("missing tenant allowed: %v", err)
	}
	// Audit timestamps are server-owned, even if a caller provides an old one.
	if err := f.audit.RecordLoginLog(ctxA, domain.LoginLog{Username: "server-time", UserType: 1, LogType: 100, CreateTime: time.Unix(1, 0)}); err != nil {
		t.Fatal(err)
	}
}

func TestSeedDefaultsIsIdempotentAndPreservesEdits(t *testing.T) {
	f := newSystemFixture(t)
	ctx := context.Background()
	if err := f.dict.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	types, err := f.dict.PageTypes(ctx, app.DictTypeFilter{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil || types.Total != 5 {
		t.Fatalf("default types: %+v, %v", types, err)
	}
	items, err := f.dict.PageData(ctx, app.DictDataFilter{DictType: "common_status", PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil || len(items.List) != 2 {
		t.Fatalf("default status items: %+v, %v", items, err)
	}
	item := items.List[0]
	item.Label = "已自定义"
	if err := f.dict.UpdateData(ctx, item); err != nil {
		t.Fatal(err)
	}
	if err := f.dict.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	if label, err := f.dict.GetLabel(ctx, "common_status", item.Value); err != nil || label != "已自定义" {
		t.Fatalf("seed overwrote existing value: %q, %v", label, err)
	}
	types, err = f.dict.PageTypes(ctx, app.DictTypeFilter{})
	if err != nil || types.Total != 5 {
		t.Fatalf("seed duplicated types: %+v, %v", types, err)
	}
}

func TestTenantVisitIsRestrictedToSuperAdministrator(t *testing.T) {
	f := newSystemFixture(t)
	for _, tt := range []struct {
		token string
		code  int
	}{{"tenant-a", 403}, {"super", 0}} {
		req := httptest.NewRequest("GET", "/admin-api/system/notice/page", nil)
		req.Header.Set("Authorization", "Bearer "+tt.token)
		req.Header.Set("tenant-id", "1")
		req.Header.Set("visit-tenant-id", "2")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		response[any](t, w, tt.code)
	}
}

func TestInfrastructureErrorsAreNotDisclosed(t *testing.T) {
	f := newSystemFixture(t)
	if err := f.db.Exec("DROP TABLE system_notice").Error; err != nil {
		t.Fatal(err)
	}
	w := request(t, f.handler, "GET", "/admin-api/system/notice/get?id=1", "tenant-a", "1", "")
	response[any](t, w, 500)
	if strings.Contains(w.Body.String(), "system_notice") || strings.Contains(w.Body.String(), "SELECT") {
		t.Fatalf("SQL details leaked: %s", w.Body.String())
	}
}

func TestUnconfiguredStartupAndHealth(t *testing.T) {
	t.Setenv("OWOT_MYSQL_DSN", "")
	t.Setenv("OWOT_REDIS_ADDR", "")
	application, err := newApplication(context.Background(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = application.close() })
	w := httptest.NewRecorder()
	application.handler.ServeHTTP(w, httptest.NewRequest("GET", "/actuator/health", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := newApplication(context.Background(), true, false); err == nil {
		t.Fatal("migration without dependencies succeeded")
	}
	t.Setenv("OWOT_REDIS_ADDR", "127.0.0.1:1")
	if _, err := newApplication(context.Background(), false, false); err == nil {
		t.Fatal("partially configured application succeeded")
	}
}
