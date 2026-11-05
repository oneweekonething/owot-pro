package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	iamapp "oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

const (
	configAdmin  = "/admin-api/infra/config/"
	configPublic = "/app-api/infra/config/get-value-by-key?key="
)

func configRequest(t *testing.T, f *iamFixture, method, path, token string, tenant int64, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if tenant > 0 {
		req.Header.Set("tenant-id", strconv.FormatInt(tenant, 10))
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("%s %s returned HTTP %d", method, path, w.Code)
	}
	return w
}

func configCall(t *testing.T, f *iamFixture, method, path, token string, tenant int64, body any, wantCode int) json.RawMessage {
	t.Helper()
	w := configRequest(t, f, method, path, token, tenant, body)
	var envelope iamEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Code != wantCode {
		t.Fatalf("%s %s: got %s (%v), want code %d", method, path, w.Body, err, wantCode)
	}
	return envelope.Data
}

func TestInfraConfigHTTP(t *testing.T) {
	f := newIAMFixture(t)
	for dict, values := range map[string][]string{"infra_config_type": {"1", "2"}, "infra_boolean_string": {"true", "false"}} {
		data := f.ok("GET", "dict-data/type?type="+dict, nil, f.admin, 1)
		for _, value := range values {
			if !bytes.Contains(data, []byte(`"value":"`+value+`"`)) {
				t.Fatalf("missing frontend dictionary option %s=%s", dict, value)
			}
		}
	}
	input := map[string]any{"category": "demo", "name": "前端参数", "key": "demo_config", "value": "=1+1", "visible": true, "type": 1}
	data := configCall(t, f, "POST", configAdmin+"create", f.admin, 1, input, 0)
	var id int64
	if err := json.Unmarshal(data, &id); err != nil || id <= 0 {
		t.Fatalf("invalid create response: %s", data)
	}
	configCall(t, f, "POST", configAdmin+"create", f.admin, 1, input, 1_001_000_002)
	configCall(t, f, "GET", configAdmin+"page", "", 1, nil, 401)
	data = configCall(t, f, "GET", configPublic+"demo_config", "", 0, nil, 0)
	if string(data) != `"=1+1"` {
		t.Fatalf("public read returned %s", data)
	}
	detailPath := configAdmin + "get?id=" + strconv.FormatInt(id, 10)
	data = configCall(t, f, "GET", detailPath, f.admin, 1, nil, 0)
	var detail struct {
		Type       int   `json:"type"`
		CreateTime int64 `json:"createTime"`
	}
	if err := json.Unmarshal(data, &detail); err != nil || detail.Type != 2 || detail.CreateTime <= 0 {
		t.Fatalf("create should ignore requested built-in type and return a timestamp: %s", data)
	}
	pagePath := fmt.Sprintf("%spage?key=demo_config&type=2&createTime[0]=%d&createTime[1]=%d", configAdmin, detail.CreateTime-1000, detail.CreateTime+1000)
	data = configCall(t, f, "GET", pagePath, f.admin, 1, nil, 0)
	if !bytes.Contains(data, []byte(`"total":1`)) {
		t.Fatalf("filtered page returned %s", data)
	}
	export := configRequest(t, f, "GET", configAdmin+"export-excel?key=demo_config", f.admin, 1, nil)
	book, err := excelize.OpenReader(bytes.NewReader(export.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = book.Close() }()
	cell, err := book.GetCellValue("Sheet1", "E2")
	if err != nil || cell != "=1+1" {
		t.Fatalf("export value = %q, %v", cell, err)
	}
	formula, err := book.GetCellFormula("Sheet1", "E2")
	if err != nil || formula != "" {
		t.Fatalf("config value became a formula: %q, %v", formula, err)
	}

	input["id"], input["key"], input["visible"] = id, "private_config", false
	configCall(t, f, "PUT", configAdmin+"update", f.admin, 1, input, 0)
	for _, key := range []string{"demo_config", "private_config"} {
		data = configCall(t, f, "GET", configPublic+key, "", 0, nil, 0)
		if string(data) != "null" {
			t.Fatalf("public read exposed %s: %s", key, data)
		}
	}
	configCall(t, f, "GET", configAdmin+"get-value-by-key?key=private_config", f.admin, 1, nil, 1_001_000_004)

	if err := f.db.Exec("INSERT INTO infra_config (category, name, config_key, value, type, visible) VALUES (?, ?, ?, ?, ?, ?)",
		"test", "内置项", "builtin_config", "sample", 1, true).Error; err != nil {
		t.Fatal(err)
	}
	var builtinID int64
	if err := f.db.Raw("SELECT id FROM infra_config WHERE config_key = ?", "builtin_config").Row().Scan(&builtinID); err != nil {
		t.Fatal(err)
	}
	if data := configCall(t, f, "GET", configPublic+"builtin_config", "", 0, nil, 0); string(data) != `"sample"` {
		t.Fatalf("SQL-provisioned config returned %s", data)
	}
	duplicate := map[string]any{"category": "demo", "name": "重复项", "key": "builtin_config", "value": "duplicate", "visible": false}
	configCall(t, f, "POST", configAdmin+"create", f.admin, 1, duplicate, 1_001_000_002)
	configCall(t, f, "DELETE", fmt.Sprintf("%sdelete-list?ids=%d&ids=%d", configAdmin, id, builtinID), f.admin, 1, nil, 1_001_000_003)
	configCall(t, f, "GET", detailPath, f.admin, 1, nil, 0)
	configCall(t, f, "DELETE", configAdmin+"delete?id="+strconv.FormatInt(id, 10), f.admin, 1, nil, 0)
	configCall(t, f, "GET", detailPath, f.admin, 1, nil, 1_001_000_001)
	configCall(t, f, "POST", configAdmin+"create", f.admin, 1, input, 0)
	data = configCall(t, f, "GET", configAdmin+"page?key=missing", f.admin, 1, nil, 0)
	if !bytes.Contains(data, []byte(`"list":[]`)) {
		t.Fatalf("empty list returned %s", data)
	}
}

func TestInfraConfigTenantPermissionBoundary(t *testing.T) {
	f := newIAMFixture(t)
	menus, err := f.service.Menus.List(rootIAMContext(1), iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		t.Fatal(err)
	}
	menuIDs := []int64{}
	for _, menu := range menus.List {
		if menu.Permission == "infra:config:query" || menu.Permission == "infra:config:create" {
			menuIDs = append(menuIDs, menu.ID)
		}
	}
	if len(menuIDs) != 2 {
		t.Fatal("config permissions are missing")
	}
	pack := f.create("tenant-package", map[string]any{"name": "配置套餐", "menuIds": menuIDs})
	tenant := f.create("tenant", map[string]any{"name": "配置租户", "contactName": "管理员", "packageId": pack,
		"accountCount": 1, "expireTime": time.Now().Add(time.Hour).UnixMilli(), "username": "tenantconfig", "password": "password"})
	token := f.login(tenant, "tenantconfig", "password").AccessToken
	info := f.ok("GET", "auth/get-permission-info", nil, token, tenant)
	if !bytes.Contains(info, []byte("infra:config:create")) || !bytes.Contains(info, []byte("infra:config:query")) {
		t.Fatal("test tenant is missing the relevant permissions")
	}
	input := map[string]any{"category": "demo", "name": "租户参数", "key": "tenant.config", "value": "value", "visible": false}
	configCall(t, f, "POST", configAdmin+"create", token, tenant, input, 403)
	configCall(t, f, "GET", configAdmin+"page", token, tenant, nil, 403)
	systemUser := f.create("user", map[string]any{"username": "configreader", "nickname": "配置查看员", "password": "password"})
	f.delegate(systemUser, "infra:config:query")
	token = f.login(1, "configreader", "password").AccessToken
	configCall(t, f, "GET", configAdmin+"page", token, 1, nil, 0)
	configCall(t, f, "POST", configAdmin+"create", token, 1, input, 403)
}
