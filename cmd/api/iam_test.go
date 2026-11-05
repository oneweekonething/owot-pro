package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	iampersistence "oneweekonething.com/owot-pro/internal/iam/adapter/persistence"
	iamapp "oneweekonething.com/owot-pro/internal/iam/app"
	iamdomain "oneweekonething.com/owot-pro/internal/iam/domain"
	infrapersistence "oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/testhelper"
	systempersistence "oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	systemapp "oneweekonething.com/owot-pro/internal/system/app"
)

type iamFixture struct {
	t               *testing.T
	handler         http.Handler
	service         *iamapp.Service
	redis           *redis.Client
	db              *gorm.DB
	admin           string
	legacyAvatarDir string
}
type iamEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

func newIAMFixture(t *testing.T) *iamFixture {
	t.Helper()
	db := testhelper.SQLite(t)
	for _, migrate := range []func(*gorm.DB) error{iampersistence.Migrate, systempersistence.Migrate, infrapersistence.Migrate, migrateFileStorage, outbox.Migrate} {
		if err := migrate(db); err != nil {
			t.Fatal(err)
		}
	}
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := outbox.NewStore(db, newBackendBroadcaster(client), log)
	configs, configCache, err := newConfigService(db, store, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(configCache.Close)
	files, err := newFileService(db, store, client, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	legacyAvatarDir := t.TempDir()
	handler, service, cache, err := newBackendHandler(db, store, client, configs, files, httpx.Dependencies{Logger: log}, legacyAvatarDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	dicts := systempersistence.NewDictRepository(db)
	if err := systemapp.NewDictService(dicts, dicts, dbx.NewTxManager(db), store).SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.SeedDefaults(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.SeedDefaults(context.Background()); err != nil {
		t.Fatalf("idempotent seed: %v", err)
	}
	f := &iamFixture{t: t, handler: handler, service: service, redis: client, db: db, legacyAvatarDir: legacyAvatarDir}
	f.admin = f.login(1, "admin", "admin123").AccessToken
	return f
}
func rootIAMContext(tenant int64) context.Context {
	return ctxmeta.WithLoginUser(ctxmeta.WithTenantID(context.Background(), tenant), ctxmeta.LoginUser{ID: 1, TenantID: 1, UserType: 1, SuperAdmin: true})
}
func (f *iamFixture) call(method, path string, body any, token string, tenant int64) iamEnvelope {
	f.t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, prefixIAM+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if tenant > 0 {
		req.Header.Set("tenant-id", fmt.Sprint(tenant))
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 200 {
		f.t.Fatalf("HTTP %d: %s", w.Code, w.Body)
	}
	var out iamEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		f.t.Fatalf("decode %s: %s", path, w.Body)
	}
	return out
}

const prefixIAM = "/admin-api/system/"

func (f *iamFixture) ok(method, path string, body any, token string, tenant int64) json.RawMessage {
	f.t.Helper()
	out := f.call(method, path, body, token, tenant)
	if out.Code != 0 {
		f.t.Fatalf("%s %s: %d %s", method, path, out.Code, out.Msg)
	}
	return out.Data
}
func (f *iamFixture) create(path string, body any) int64 {
	f.t.Helper()
	data := f.ok("POST", path+"/create", body, f.admin, 1)
	var id int64
	if err := json.Unmarshal(data, &id); err != nil || id <= 0 {
		f.t.Fatalf("invalid create response %s", data)
	}
	return id
}
func (f *iamFixture) login(tenant int64, username, password string) struct {
	AccessToken, RefreshToken string
	UserID, ExpiresTime       int64
} {
	f.t.Helper()
	data := f.ok("POST", "auth/login", map[string]any{"username": username, "password": password}, "", tenant)
	var out struct {
		AccessToken, RefreshToken string
		UserID, ExpiresTime       int64
	}
	if err := json.Unmarshal(data, &out); err != nil {
		f.t.Fatal(err)
	}
	if out.AccessToken == "" || out.RefreshToken == "" || out.ExpiresTime <= time.Now().UnixMilli() {
		f.t.Fatalf("invalid login %s", data)
	}
	return out
}
func TestIAMAuthenticationAndSessionRevocation(t *testing.T) {
	f := newIAMFixture(t)
	if got := f.call("POST", "auth/login", map[string]any{"username": "admin", "password": "wrong"}, "", 1); got.Code != 1002000000 {
		t.Fatal(got)
	}
	info := f.ok("GET", "auth/get-permission-info", nil, f.admin, 1)
	if !bytes.Contains(info, []byte("super_admin")) || !bytes.Contains(info, []byte("system:user:query")) {
		t.Fatal(string(info))
	}
	if strings.Contains(string(info), "PasswordHash") || strings.Contains(string(info), `"password":`) {
		t.Fatal("password leaked")
	}
	id := f.create("user", map[string]any{"username": "userone", "nickname": "用户一", "password": "password", "postIds": []int64{}})
	tokens := f.login(1, "userone", "password")
	f.ok("GET", "user/profile/get", nil, tokens.AccessToken, 1)
	if got := f.call("GET", "user/page", nil, tokens.AccessToken, 1); got.Code != 403 {
		t.Fatal("RBAC bypass", got)
	}
	f.ok("PUT", "user/update-status", map[string]any{"id": id, "status": 1}, f.admin, 1)
	if exists := f.redis.Exists(context.Background(), "oauth2_access_token:"+tokens.AccessToken).Val(); exists != 0 {
		t.Fatal("disabled token still cached")
	}
	if got := f.call("GET", "user/profile/get", nil, tokens.AccessToken, 1); got.Code != 401 {
		t.Fatal("disabled token still accepted", got)
	}
	if got := f.call("POST", "auth/refresh-token?refreshToken="+tokens.RefreshToken, nil, "", 1); got.Code != 401 {
		t.Fatal("disabled refresh accepted", got)
	}
	if got := f.call("POST", "auth/login", map[string]any{"username": "userone", "password": "password"}, "", 1); got.Code != 1002000001 {
		t.Fatal("disabled login", got)
	}
	f.ok("PUT", "user/update-status", map[string]any{"id": id, "status": 0}, f.admin, 1)
	tokens = f.login(1, "userone", "password")
	data := f.ok("POST", "auth/refresh-token?refreshToken="+tokens.RefreshToken, nil, "", 1)
	var rotated struct{ AccessToken, RefreshToken string }
	if err := json.Unmarshal(data, &rotated); err != nil {
		t.Fatal(err)
	}
	if got := f.call("POST", "auth/refresh-token?refreshToken="+tokens.RefreshToken, nil, "", 1); got.Code != 401 {
		t.Fatal("refresh replay", got)
	}
	if got := f.call("GET", "user/profile/get", nil, tokens.AccessToken, 1); got.Code != 401 {
		t.Fatal("old access survived rotation", got)
	}
	f.ok("DELETE", "oauth2-token/delete?accessToken="+rotated.AccessToken, nil, f.admin, 1)
	if got := f.call("GET", "user/profile/get", nil, rotated.AccessToken, 1); got.Code != 401 {
		t.Fatal("force logout failed", got)
	}
	tokens = f.login(1, "userone", "password")
	f.ok("POST", "auth/logout", nil, tokens.AccessToken, 1)
	if got := f.call("POST", "auth/refresh-token?refreshToken="+tokens.RefreshToken, nil, "", 1); got.Code != 401 {
		t.Fatal("logout refresh survived", got)
	}
	for _, path := range []string{"user/delete?id=1", "role/delete?id=1", "tenant/delete?id=1", "oauth2-client/delete?id=1"} {
		if got := f.call("DELETE", path, nil, f.admin, 1); got.Code == 0 {
			t.Fatal("built-in deleted", path)
		}
	}
}

func TestIAMOrganizationAndDataPermissions(t *testing.T) {
	f := newIAMFixture(t)
	a := f.create("dept", map[string]any{"name": "部门A"})
	b := f.create("dept", map[string]any{"name": "部门B", "parentId": a})
	c := f.create("dept", map[string]any{"name": "部门C"})
	if got := f.call("PUT", "dept/update", map[string]any{"id": a, "name": "部门A", "parentId": b}, f.admin, 1); got.Code == 0 {
		t.Fatal("department cycle accepted")
	}
	post := f.create("post", map[string]any{"name": "工程师", "code": "engineer"})
	user := f.create("user", map[string]any{"username": "scopeuser", "nickname": "范围用户", "password": "password", "deptId": a, "postIds": []int64{post}})
	sibling := f.create("user", map[string]any{"username": "sibling", "nickname": "同部门", "password": "password", "deptId": a})
	child := f.create("user", map[string]any{"username": "childuser", "nickname": "子部门", "password": "password", "deptId": b})
	other := f.create("user", map[string]any{"username": "otheruser", "nickname": "其他部门", "password": "password", "deptId": c})
	role := f.create("role", map[string]any{"name": "范围角色", "code": "scope"})
	menus, err := f.service.Menus.List(rootIAMContext(1), iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		t.Fatal(err)
	}
	menuIDs := []int64{}
	for _, m := range menus.List {
		if m.Permission == "system:user:query" || m.Permission == "system:user:update" || m.Permission == "system:user:create" {
			menuIDs = append(menuIDs, m.ID)
		}
	}
	f.ok("POST", "permission/assign-role-menu", map[string]any{"roleId": role, "menuIds": menuIDs}, f.admin, 1)
	f.ok("POST", "permission/assign-user-role", map[string]any{"userId": user, "roleIds": []int64{role}}, f.admin, 1)
	token := f.login(1, "scopeuser", "password").AccessToken
	for _, tt := range []struct {
		Scope      int
		Depts, IDs []int64
	}{{5, nil, []int64{user}}, {3, nil, []int64{user, sibling}}, {4, nil, []int64{user, sibling, child}}, {2, []int64{c}, []int64{other}}, {1, nil, []int64{1, user, sibling, child, other}}} {
		f.ok("POST", "permission/assign-role-data-scope", map[string]any{"roleId": role, "dataScope": tt.Scope, "dataScopeDeptIds": tt.Depts}, f.admin, 1)
		data := f.ok("GET", "user/page?pageSize=-1", nil, token, 1)
		var page struct {
			List  []struct{ ID int64 }
			Total int64
		}
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, u := range page.List {
			ids = append(ids, u.ID)
		}
		if !slices.Equal(ids, tt.IDs) || page.Total != int64(len(tt.IDs)) {
			rawRole, _ := f.service.GetRole(rootIAMContext(1), role)
			tokenUser, _ := f.service.Verify(context.Background(), token)
			scope, _ := f.service.DataScope(rootIAMContext(1), tokenUser)
			t.Fatalf("scope %d got %v want %v; role %+v; user %+v; scope %+v; epoch %s", tt.Scope, ids, tt.IDs, rawRole, tokenUser, scope, f.redis.Get(context.Background(), "iam:perm_ver:1").Val())
		}
	}
	f.ok("POST", "permission/assign-role-data-scope", map[string]any{"roleId": role, "dataScope": 5}, f.admin, 1)
	if got := f.call("POST", "user/create", map[string]any{"username": "scopeescape", "nickname": "范围外新增", "password": "password"}, token, 1); got.Code != 403 {
		t.Fatal("self scope created an unrelated account without a department", got)
	}
	f.ok("PUT", "user/update", map[string]any{"id": user, "username": "scopeuser", "nickname": "本人修改", "deptId": a, "postIds": []int64{post}}, token, 1)
	if got := f.call("PUT", "user/update", map[string]any{"id": other, "username": "otheruser", "nickname": "越权修改", "deptId": c}, token, 1); got.Code == 0 {
		t.Fatal("cross-scope user update accepted")
	}
	f.ok("POST", "permission/assign-role-menu", map[string]any{"roleId": role, "menuIds": []int64{}}, f.admin, 1)
	if got := f.call("GET", "user/page", nil, token, 1); got.Code != 403 {
		t.Fatal("permission cache retained revoked menu", got)
	}
	if got := f.call("DELETE", fmt.Sprintf("post/delete?id=%d", post), nil, f.admin, 1); got.Code == 0 {
		t.Fatal("referenced post deleted")
	}
	if got := f.call("DELETE", fmt.Sprintf("dept/delete?id=%d", a), nil, f.admin, 1); got.Code == 0 {
		t.Fatal("referenced department deleted")
	}
	f.ok("DELETE", fmt.Sprintf("user/delete-list?ids=%d&ids=%d", sibling, child), nil, f.admin, 1)
	empty := f.ok("GET", "user/page?username=absent", nil, f.admin, 1)
	if !bytes.Contains(empty, []byte(`"list":[]`)) {
		t.Fatal("null empty list", string(empty))
	}
}

func TestIAMTenantCascadeQuotaAndPackageReclamation(t *testing.T) {
	f := newIAMFixture(t)
	ctx := rootIAMContext(1)
	menus, err := f.service.Menus.List(ctx, iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		t.Fatal(err)
	}
	menuIDs := []int64{}
	for _, m := range menus.List {
		if m.Permission == "system:user:query" || m.Permission == "system:post:query" {
			menuIDs = append(menuIDs, m.ID)
		}
	}
	if len(menuIDs) != 2 {
		t.Fatal(menuIDs)
	}
	pack := f.create("tenant-package", map[string]any{"name": "测试套餐", "menuIds": menuIDs})
	tenant := f.create("tenant", map[string]any{"name": "测试租户", "contactName": "联系人", "packageId": pack, "accountCount": 2, "expireTime": time.Now().Add(time.Hour).UnixMilli(), "websites": []string{"tenant.example"}, "username": "tenantadmin", "password": "password"})
	tokens := f.login(tenant, "tenantadmin", "password")
	info := f.ok("GET", "auth/get-permission-info", nil, tokens.AccessToken, tenant)
	if !bytes.Contains(info, []byte("tenant_admin")) {
		t.Fatal(string(info))
	}
	if got := f.call("GET", "user/get?id=1", nil, tokens.AccessToken, tenant); got.Code == 0 {
		t.Fatal("cross-tenant user visible")
	}
	if got := f.call("GET", "user/page", nil, tokens.AccessToken, 1); got.Code != 403 {
		t.Fatal("forged tenant header accepted", got)
	}
	target := rootIAMContext(tenant)
	users, err := f.service.Users.List(target, iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil || len(users.List) != 1 {
		t.Fatal(users, err)
	}
	admin := users.List[0]
	depts, err := f.service.Depts.List(target, iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil || len(depts.List) != 1 || admin.DeptID != depts.List[0].ID {
		t.Fatal("missing cascade", err)
	}
	_, err = f.service.SaveUser(target, iamdomain.AdminUser{Username: "seconduser", Nickname: "第二用户", DeptID: admin.DeptID}, "password")
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.SaveUser(target, iamdomain.AdminUser{Username: "thirduser", Nickname: "第三用户"}, "password")
	if !errors.Is(err, iamdomain.ErrQuotaExceeded) {
		t.Fatal("quota ignored", err)
	}
	custom, err := f.service.SaveRole(target, iamdomain.Role{Name: "自定义", Code: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.AssignRoleMenus(target, custom, menuIDs); err != nil {
		t.Fatal(err)
	}
	f.ok("PUT", "tenant-package/update", map[string]any{"id": pack, "name": "测试套餐", "menuIds": menuIDs[:1]}, f.admin, 1)
	roles, err := f.service.Roles.List(target, iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range roles.List {
		ids, err := f.service.RoleMenuIDs(target, role.ID)
		if err != nil || !slices.Equal(ids, menuIDs[:1]) {
			t.Fatal("package reclamation", role, ids, err)
		}
	}
	info = f.ok("GET", "auth/get-permission-info", nil, tokens.AccessToken, tenant)
	if bytes.Contains(info, []byte("system:post:query")) {
		t.Fatal("old package permissions retained", string(info))
	}
	if got := f.call("DELETE", fmt.Sprintf("tenant-package/delete?id=%d", pack), nil, f.admin, 1); got.Code == 0 {
		t.Fatal("referenced package deleted")
	}
	f.ok("GET", "tenant/get-id-by-name?name=测试租户", nil, "", 0)
	f.ok("GET", "tenant/get-by-website?website=tenant.example", nil, "", 0)
	f.ok("DELETE", fmt.Sprintf("tenant/delete?id=%d", tenant), nil, f.admin, 1)
	if got := f.call("GET", "user/profile/get", nil, tokens.AccessToken, tenant); got.Code != 401 {
		t.Fatal("deleted tenant session survived", got)
	}
}

func (f *iamFixture) upload(path, field, name string, data []byte) iamEnvelope {
	f.t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile(field, name)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		f.t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		f.t.Fatal(err)
	}
	req := httptest.NewRequest("POST", prefixIAM+path, &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("tenant-id", "1")
	req.Header.Set("Authorization", "Bearer "+f.admin)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	var out iamEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		f.t.Fatal(err, w.Body.String())
	}
	return out
}
func TestIAMExcelAvatarClientsAndAudit(t *testing.T) {
	f := newIAMFixture(t)
	ctx := context.Background()
	var workbook bytes.Buffer
	headers := []string{"用户账号", "用户昵称", "部门编号", "用户邮箱", "手机号码", "用户性别", "帐号状态", "初始密码"}
	if err := excelx.Write(ctx, &workbook, headers, [][]any{{"importuser", "导入用户", 0, "test@example.com", "13800138000", "男", "开启", "password"}}); err != nil {
		t.Fatal(err)
	}
	result := f.upload("user/import?updateSupport=false", "file", "users.xlsx", workbook.Bytes())
	if result.Code != 0 || !bytes.Contains(result.Data, []byte(`"createUsernames":["importuser"]`)) {
		t.Fatal(result, string(result.Data))
	}
	result = f.upload("user/import?updateSupport=false", "file", "users.xlsx", workbook.Bytes())
	if result.Code != 0 || !bytes.Contains(result.Data, []byte("failureUsernames")) {
		t.Fatal(result)
	}
	token := f.login(1, "importuser", "password").AccessToken
	f.ok("PUT", "user/profile/update", map[string]any{"nickname": "更新昵称", "email": "test@example.com", "mobile": "13800138000", "sex": 2}, token, 1)
	profile := f.ok("GET", "user/profile/get", nil, token, 1)
	if !bytes.Contains(profile, []byte("更新昵称")) || bytes.Contains(profile, []byte(`"password":`)) {
		t.Fatal(string(profile))
	}
	req := httptest.NewRequest("GET", prefixIAM+"user/export-excel", nil)
	req.Header.Set("tenant-id", "1")
	req.Header.Set("Authorization", "Bearer "+f.admin)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err, w.Body.String())
	}
	defer func() { _ = book.Close() }()
	rows, err := book.GetRows("Sheet1")
	if err != nil || len(rows) != 3 || rows[2][5] != "女" || rows[2][6] != "开启" {
		t.Fatal("dictionary export", rows, err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	result = f.upload("user/profile/update-avatar", "avatarFile", "avatar.png", imageBytes.Bytes())
	if result.Code != 0 {
		t.Fatal(result)
	}
	var url string
	if err := json.Unmarshal(result.Data, &url); err != nil {
		t.Fatal(err)
	}
	imageRequest := httptest.NewRequest("GET", url, nil)
	imageResponse := httptest.NewRecorder()
	f.handler.ServeHTTP(imageResponse, imageRequest)
	if imageResponse.Code != 200 || !bytes.Equal(imageResponse.Body.Bytes(), imageBytes.Bytes()) {
		t.Fatal("avatar inaccessible", imageResponse.Body.String())
	}
	const legacyName = "0123456789abcdef0123456789abcdef.png"
	if err := os.WriteFile(filepath.Join(f.legacyAvatarDir, legacyName), imageBytes.Bytes(), 0640); err != nil {
		t.Fatal(err)
	}
	legacy := httptest.NewRecorder()
	f.handler.ServeHTTP(legacy, httptest.NewRequest("GET", prefixIAM+"user/profile/avatar/"+legacyName, nil))
	if legacy.Code != 200 || !bytes.Equal(legacy.Body.Bytes(), imageBytes.Bytes()) {
		t.Fatalf("legacy avatar inaccessible: %d %s", legacy.Code, legacy.Body)
	}
	result = f.upload("user/profile/update-avatar", "file", "bad.svg", []byte(`<svg/>`))
	if result.Code != 400 {
		t.Fatal("non-image avatar accepted", result)
	}
	clientID := f.create("oauth2-client", map[string]any{"clientId": "test-client", "secret": "test-secret", "name": "测试客户端", "accessTokenValiditySeconds": 60, "refreshTokenValiditySeconds": 600})
	f.ok("GET", fmt.Sprintf("oauth2-client/get?id=%d", clientID), nil, f.admin, 1)
	f.ok("DELETE", fmt.Sprintf("oauth2-client/delete?id=%d", clientID), nil, f.admin, 1)
	f.ok("PUT", "user/profile/update-password", map[string]any{"oldPassword": "password", "newPassword": "changed123"}, token, 1)
	if got := f.call("GET", "user/profile/get", nil, token, 1); got.Code != 401 {
		t.Fatal("password change kept old sessions", got)
	}
	f.login(1, "importuser", "changed123")
	logData := f.ok("GET", "login-log/page?pageSize=-1", nil, f.admin, 1)
	if !bytes.Contains(logData, []byte("importuser")) {
		t.Fatal("IAM login audit bridge missing", string(logData))
	}
	operateData := f.ok("GET", "operate-log/page?pageSize=-1", nil, f.admin, 1)
	if !bytes.Contains(operateData, []byte("管理员")) {
		t.Fatal("IAM nickname bridge missing", string(operateData))
	}
}

func TestIAMSuperAdministratorStatusAndVisitLogout(t *testing.T) {
	f := newIAMFixture(t)
	for _, request := range []struct {
		path string
		body map[string]any
	}{
		{"role/update-status", map[string]any{"id": 1, "status": 1}},
		{"role/update", map[string]any{"id": 1, "name": "超级管理员", "code": "super_admin", "status": 1}},
		{"oauth2-client/update", map[string]any{"id": 1, "clientId": "default", "secret": "default", "name": "默认客户端", "status": 1, "accessTokenValiditySeconds": 60, "refreshTokenValiditySeconds": 120}},
	} {
		if got := f.call("PUT", request.path, request.body, f.admin, 1); got.Code == 0 {
			t.Fatal("seeded administrator can be locked out", request.path)
		}
	}
	secondary := f.create("user", map[string]any{"username": "secondary", "nickname": "备用管理员", "password": "password"})
	f.ok("POST", "permission/assign-user-role", map[string]any{"userId": secondary, "roleIds": []int64{1}}, f.admin, 1)
	token := f.login(1, "secondary", "password").AccessToken
	f.ok("GET", "user/page", nil, token, 1)
	f.ok("POST", "permission/assign-user-role", map[string]any{"userId": secondary, "roleIds": []int64{}}, f.admin, 1)
	if got := f.call("GET", "user/page", nil, token, 1); got.Code != 403 {
		t.Fatal("removed super administrator role retained bypass", got)
	}
	pack := f.create("tenant-package", map[string]any{"name": "访问套餐", "menuIds": []int64{}})
	tenant := f.create("tenant", map[string]any{"name": "访问租户", "contactName": "管理员", "packageId": pack, "accountCount": 2, "expireTime": time.Now().Add(time.Hour).UnixMilli(), "username": "visitadmin", "password": "password"})
	req := httptest.NewRequest("POST", prefixIAM+"auth/logout", nil)
	req.Header.Set("tenant-id", "1")
	req.Header.Set("visit-tenant-id", fmt.Sprint(tenant))
	req.Header.Set("Authorization", "Bearer "+f.admin)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	var result iamEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 0 {
		t.Fatal("visit logout", result, err)
	}
	if _, err := f.service.Verify(context.Background(), f.admin); err == nil {
		t.Fatal("visit logout revoked the target tenant instead of the principal")
	}
}
