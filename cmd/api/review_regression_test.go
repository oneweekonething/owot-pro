package main

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func TestCORSPreflightAndProtectedResponse(t *testing.T) {
	f := newSystemFixture(t)
	for _, method := range []string{"OPTIONS", "GET"} {
		req := httptest.NewRequest(method, "/admin-api/system/notice/page", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		if method == "OPTIONS" {
			req.Header.Set("Access-Control-Request-Method", "GET")
			req.Header.Set("Access-Control-Request-Headers", "authorization,tenant-id,visit-tenant-id")
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("%s response lacks CORS headers: %v", method, w.Header())
		}
		if method == "OPTIONS" {
			response[bool](t, w, 0) // Preflight must not require credentials.
			if !strings.Contains(strings.ToLower(w.Header().Get("Access-Control-Allow-Headers")), "tenant-id") {
				t.Fatal("tenant header not permitted")
			}
		} else {
			response[any](t, w, 401) // Actual requests still require authentication.
		}
	}
}

func TestLongUserAgentDoesNotSuppressAudit(t *testing.T) {
	for _, unit := range []string{"a", "界"} {
		t.Run(unit, func(t *testing.T) {
			f := newSystemFixture(t)
			req := httptest.NewRequest("POST", "/admin-api/system/notice/create", strings.NewReader(`{"title":"审计","type":1,"content":"内容","status":0}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer tenant-a")
			req.Header.Set("tenant-id", "1")
			req.Header.Set("User-Agent", strings.Repeat(unit, 1025))
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, req)
			id := response[int64](t, w, 0)
			logs, err := f.audit.PageOperateLogs(ctxmeta.WithTenantID(context.Background(), 1), app.OperateLogFilter{})
			if err != nil || logs.Total != 1 {
				t.Fatalf("successful mutation lost audit: total=%d err=%v", logs.Total, err)
			}
			if logs.List[0].BizID != id || logs.List[0].UserAgent != strings.Repeat(unit, 1024) || !utf8.ValidString(logs.List[0].UserAgent) {
				t.Fatalf("audit metadata was not safely truncated: %+v", logs.List[0])
			}
		})
	}
}

func TestExportRoutesAcceptFrontendAndLegacyPaths(t *testing.T) {
	f := newSystemFixture(t)
	for _, resource := range []string{"dict-type", "dict-data", "operate-log", "login-log"} {
		for _, action := range []string{"export", "export-excel"} {
			w := request(t, f.handler, "GET", "/admin-api/system/"+resource+"/"+action, "tenant-a", "1", "")
			book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatalf("%s/%s did not return XLSX: %s", resource, action, w.Body.String())
			}
			if err := book.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSimpleDictTypesIncludeDisabledTypes(t *testing.T) {
	f := newSystemFixture(t)
	for _, value := range []domain.DictType{{Name: "开启", Type: "enabled"}, {Name: "禁用", Type: "disabled", Status: 1}} {
		if _, err := f.dict.CreateType(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []string{"simple-list", "list-all-simple"} {
		items := response[[]map[string]any](t, request(t, f.handler, "GET", "/admin-api/system/dict-type/"+action, "tenant-a", "1", ""), 0)
		if len(items) != 2 {
			t.Fatalf("%s omitted disabled type: %+v", action, items)
		}
	}
}

func TestOperateLogFrontendFieldsAndFilters(t *testing.T) {
	f := newSystemFixture(t)
	ctx := ctxmeta.WithTenantID(context.Background(), 1)
	for _, value := range []domain.OperateLog{
		{UserID: 7, UserType: 1, Type: "dict", SubType: "create", BizID: 11, Action: "created record"},
		{UserID: 7, UserType: 1, Type: "dict", SubType: "delete", BizID: 12, Action: "deleted record"},
	} {
		if err := f.audit.RecordOperateLog(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		query string
		total int64
		code  int
	}{
		{"action=created", 1, 0}, {"bizId=11", 1, 0}, {"bizId=0", 0, 0},
		{"bizId=999&action=definitely_missing", 0, 0}, {"bizId=12&action=created", 0, 0},
		{"bizId=-1", 0, 400}, {"bizId=bad", 0, 400},
	} {
		result := response[pagination.PageResult[map[string]any]](t, request(t, f.handler, "GET", "/admin-api/system/operate-log/page?"+tt.query, "tenant-a", "1", ""), tt.code)
		if tt.code != 0 {
			continue
		}
		if result.Total != tt.total {
			t.Fatalf("query %q: total=%d want=%d", tt.query, result.Total, tt.total)
		}
		for _, item := range result.List {
			if item["userName"] != "小王" {
				t.Fatalf("frontend userName field missing: %+v", item)
			}
		}
	}
	w := request(t, f.handler, "GET", "/admin-api/system/operate-log/export-excel?bizId=11&action=created", "tenant-a", "1", "")
	book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = book.Close() }()
	rows, err := book.GetRows("Sheet1")
	if err != nil || len(rows) != 2 || rows[1][5] != "11" || rows[1][6] != "created record" {
		t.Fatalf("export ignored audit filters: %+v, %v", rows, err)
	}
}

type recordingDictTypes struct {
	app.DictTypeRepository
	locked []string
}

func (r *recordingDictTypes) LockType(ctx context.Context, typ string) (domain.DictType, error) {
	r.locked = append(r.locked, typ)
	return r.DictTypeRepository.LockType(ctx, typ)
}

func TestDictionaryMutationsUseConsistentParentLockOrder(t *testing.T) {
	f := newSystemFixture(t)
	ctx := context.Background()
	z, err := f.dict.CreateType(ctx, domain.DictType{Name: "Z", Type: "z"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.dict.CreateType(ctx, domain.DictType{Name: "A", Type: "a"})
	if err != nil {
		t.Fatal(err)
	}
	itemZ, err := f.dict.CreateData(ctx, domain.DictData{DictType: "z", Label: "Z", Value: "0"})
	if err != nil {
		t.Fatal(err)
	}
	itemA, err := f.dict.CreateData(ctx, domain.DictData{DictType: "a", Label: "A", Value: "0"})
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.NewDictRepository(f.db)
	types := &recordingDictTypes{DictTypeRepository: repo}
	service := app.NewDictService(types, repo, dbx.NewTxManager(f.db), rejectingOutbox{})
	// The first dictionary contains data, so deletion must abort without mutation.
	if err := service.DeleteTypes(ctx, []int64{z, a}); !errors.Is(err, domain.ErrDictTypeHasData) {
		t.Fatal(err)
	}
	if !slices.Equal(types.locked, []string{"a", "z"}) {
		t.Fatalf("type deletion locks parents in ID order: %v", types.locked)
	}
	types.locked = nil
	// The rejecting publisher rolls back the deletion after the parent locks are acquired.
	if err := service.DeleteData(ctx, []int64{itemZ, itemA}); err == nil {
		t.Fatal("outbox failure ignored")
	}
	if !slices.Equal(types.locked, []string{"a", "z"}) {
		t.Fatalf("data deletion locks parents in item ID order: %v", types.locked)
	}
}
