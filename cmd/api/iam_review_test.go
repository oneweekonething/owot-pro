package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	iamapp "oneweekonething.com/owot-pro/internal/iam/app"
	iamdomain "oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func (f *iamFixture) delegate(userID int64, permissions ...string) {
	f.t.Helper()
	roleID := f.create("role", map[string]any{"name": "委派角色", "code": fmt.Sprintf("delegated_%d", userID)})
	menus, err := f.service.Menus.List(rootIAMContext(1), iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		f.t.Fatal(err)
	}
	ids := []int64{}
	for _, p := range permissions {
		for _, m := range menus.List {
			if m.Permission == p {
				ids = append(ids, m.ID)
				break
			}
		}
	}
	if len(ids) != len(permissions) {
		f.t.Fatal("missing seeded permission", permissions)
	}
	f.ok("POST", "permission/assign-role-menu", map[string]any{"roleId": roleID, "menuIds": ids}, f.admin, 1)
	f.ok("POST", "permission/assign-user-role", map[string]any{"userId": userID, "roleIds": []int64{roleID}}, f.admin, 1)
}

func TestIAMReviewDelegatedPasswordResetCannotTakeOverSuperAdmin(t *testing.T) {
	f := newIAMFixture(t)
	id := f.create("user", map[string]any{"username": "helpdesk", "nickname": "支持人员", "password": "password"})
	f.delegate(id, "system:user:update-password")
	token := f.login(1, "helpdesk", "password").AccessToken
	result := f.call("PUT", "user/update-password", map[string]any{"id": 1, "password": "takeover123"}, token, 1)
	if result.Code != 403 {
		t.Fatalf("delegated reset of the system administrator was not denied: code %d", result.Code)
	}
	f.login(1, "admin", "admin123")
}

func TestIAMReviewOnlineSessionsNeverReturnBearerCredentials(t *testing.T) {
	f := newIAMFixture(t)
	id := f.create("user", map[string]any{"username": "observer", "nickname": "会话观察者", "password": "password"})
	f.delegate(id, "system:oauth2-token:page")
	token := f.login(1, "observer", "password").AccessToken
	data := f.ok("GET", "oauth2-token/page?pageSize=-1", nil, token, 1)
	if bytes.Contains(data, []byte(f.admin)) || bytes.Contains(data, []byte(token)) {
		t.Fatal("session list exposed an active bearer credential")
	}
	var page struct {
		List []struct {
			UserID      int64
			AccessToken string
		}
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	for _, row := range page.List {
		if row.UserID == id {
			if row.AccessToken == "" {
				t.Fatal("missing frontend revocation handle")
			}
			if got := f.call("GET", "user/profile/get", nil, row.AccessToken, 1); got.Code != 401 {
				t.Fatal("revocation handle works as a bearer token")
			}
			f.ok("DELETE", "oauth2-token/delete?accessToken="+row.AccessToken, nil, f.admin, 1)
			if got := f.call("GET", "user/profile/get", nil, token, 1); got.Code != 401 {
				t.Fatal("revocation handle did not revoke the session")
			}
			return
		}
		if got := f.call("DELETE", "oauth2-token/delete?accessToken="+row.AccessToken, nil, token, 1); got.Code != 403 {
			t.Fatal("monitor was allowed to revoke sessions")
		}
	}
	t.Fatal("observer session missing")
}

func TestIAMReviewRejectedUserMutationsKeepSession(t *testing.T) {
	f := newIAMFixture(t)
	id := f.create("user", map[string]any{"username": "rollbackuser", "nickname": "测试用户", "password": "password"})
	token := f.login(1, "rollbackuser", "password").AccessToken
	if result := f.call("PUT", "user/update", map[string]any{"id": id, "username": "rollbackuser", "nickname": "", "status": 1}, f.admin, 1); result.Code == 0 {
		t.Fatal("invalid update succeeded")
	}
	f.ok("GET", "user/profile/get", nil, token, 1)
	if result := f.call("DELETE", fmt.Sprintf("user/delete-list?ids=%d,1", id), nil, f.admin, 1); result.Code == 0 {
		t.Fatal("batch deleted protected account")
	}
	f.ok("GET", "user/profile/get", nil, token, 1)
	f.service.Outbox = failedIAMOutbox{}
	if result := f.call("PUT", "user/update-status", map[string]any{"id": id, "status": 1}, f.admin, 1); result.Code != 500 {
		t.Fatal("outbox write failure was not surfaced", result.Code)
	}
	f.ok("GET", "user/profile/get", nil, token, 1)
}

type failedIAMOutbox struct{}

func (failedIAMOutbox) Publish(context.Context, string, []byte) error {
	return errors.New("outbox unavailable")
}

type failedAccessLookup struct {
	iamapp.OAuth2Repository
	err error
}

func (f failedAccessLookup) FindAccess(context.Context, iamapp.AccessSelector) (iamdomain.AccessToken, error) {
	return iamdomain.AccessToken{}, f.err
}

func TestIAMReviewVerificationDistinguishesDatabaseFailure(t *testing.T) {
	f := newIAMFixture(t)
	cause := errors.New("private database diagnostic")
	f.service.Tokens = failedAccessLookup{OAuth2Repository: f.service.Tokens, err: cause}
	if _, err := f.service.Verify(context.Background(), f.admin); !errors.Is(err, cause) {
		t.Fatal("lost database error cause", err)
	}
	if got := f.call("GET", "user/profile/get", nil, f.admin, 1); got.Code != 500 || got.Msg != "系统异常" {
		t.Fatal("verification did not return a sanitized service error", got.Code, got.Msg)
	}
}

type blockedLoginLock struct {
	iamapp.MutationLocker
	called bool
}

func (l *blockedLoginLock) LockTenant(context.Context, int64) error {
	l.called = true
	return errors.New("unexpected tenant write lock")
}

func TestIAMReviewInvalidPasswordDoesNotAcquireTenantWriteLock(t *testing.T) {
	f := newIAMFixture(t)
	lock := &blockedLoginLock{MutationLocker: f.service.Locks}
	f.service.Locks = lock
	if _, err := f.service.Login(rootIAMContext(1), iamapp.LoginRequest{Username: "admin", Password: "incorrect"}); !errors.Is(err, iamdomain.ErrCredentials) || lock.called {
		t.Fatal("failed login interferes with tenant mutations", err)
	}
}

func TestIAMReviewLogoutRevokesRefreshAfterAccessExpiry(t *testing.T) {
	f := newIAMFixture(t)
	f.create("user", map[string]any{"username": "expireuser", "nickname": "过期会话", "password": "password"})
	tokens := f.login(1, "expireuser", "password")
	if err := f.db.Exec("UPDATE system_oauth2_access_token SET expires_time = ? WHERE access_token = ?", time.Now().Add(-time.Minute), tokens.AccessToken).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.redis.Del(context.Background(), "oauth2_access_token:"+tokens.AccessToken).Err(); err != nil {
		t.Fatal(err)
	}
	f.ok("POST", "auth/logout", nil, tokens.AccessToken, 1)
	if got := f.call("POST", "auth/refresh-token?refreshToken="+tokens.RefreshToken, nil, "", 1); got.Code != 401 {
		t.Fatal("logout left the paired refresh token active")
	}
}

func TestIAMReviewUserListDTOsExposeOnlyExpectedFields(t *testing.T) {
	f := newIAMFixture(t)
	dept := f.create("dept", map[string]any{"name": "技术部"})
	id := f.create("user", map[string]any{"username": "listuser", "nickname": "列表用户", "password": "password", "deptId": dept, "email": "person@example.com", "mobile": "13800138000", "remark": "仅管理可见"})
	f.delegate(id, "system:post:query")
	token := f.login(1, "listuser", "password").AccessToken
	data := f.ok("GET", "user/simple-list", nil, token, 1)
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) < 2 {
		t.Fatalf("unexpected simple list: %d rows, %v", len(rows), err)
	}
	for _, row := range rows {
		for _, key := range []string{"username", "email", "mobile", "remark", "loginIp", "loginDate"} {
			if _, ok := row[key]; ok {
				t.Fatalf("simple list exposes %s", key)
			}
		}
	}
	data = f.ok("GET", "user/page?username=listuser", nil, f.admin, 1)
	if !bytes.Contains(data, []byte(`"deptName":"技术部"`)) {
		t.Fatal("yudao user table department column is missing")
	}
}
