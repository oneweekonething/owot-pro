package domain

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPasswordAndBuiltInProtection(t *testing.T) {
	for _, value := range []string{"", "abc", "a bcd", "abcd\n", "12345678901234567"} {
		if !errors.Is(ValidatePassword(value), ErrPassword) {
			t.Fatalf("accepted invalid password %q", value)
		}
	}
	for _, value := range []string{"abcd", "admin123", "中文密码"} {
		if err := ValidatePassword(value); err != nil {
			t.Fatal(err)
		}
	}
	r := Role{Name: "管理员", Code: "tenant_admin", Type: 1, DataScope: 1}
	next := r
	next.Code = "other"
	if !errors.Is(r.ValidateChange(next), ErrProtected) {
		t.Fatal("built-in role code changed")
	}
	u := AdminUser{TenantID: 1, Username: "admin"}
	if !u.IsInitialAdmin() {
		t.Fatal("initial administrator not protected")
	}
	u.TenantID = 2
	if u.IsInitialAdmin() {
		t.Fatal("unrelated admin username protected")
	}
}
func TestDepartmentTreeCycleAndDescendants(t *testing.T) {
	depts := []Dept{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3, ParentID: 2}, {ID: 4}}
	for _, parent := range []int64{1, 2, 3} {
		if !errors.Is(ValidateDeptParent(depts, 1, parent), ErrTreeCycle) {
			t.Fatalf("cycle accepted: parent %d", parent)
		}
	}
	if !errors.Is(ValidateDeptParent(depts, 2, 99), ErrDeptNotFound) {
		t.Fatal("missing parent accepted")
	}
	if err := ValidateDeptParent(depts, 2, 4); err != nil {
		t.Fatal(err)
	}
	if got := DescendantIDs(depts, 1); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatal(got)
	}
	depts[0].ParentID = 3
	if got := DescendantIDs(depts, 1); len(got) != 3 {
		t.Fatal("corrupt tree traversal", got)
	}
}
func TestPermissionUnionAllFiveScopes(t *testing.T) {
	depts := []Dept{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3, ParentID: 2}, {ID: 4}}
	cases := []struct {
		Name  string
		Roles []Role
		Want  DataPermission
	}{
		{"none", nil, DataPermission{DeptIDs: []int64{}}},
		{"self", []Role{{DataScope: 5}}, DataPermission{Self: true, DeptIDs: []int64{}}},
		{"department", []Role{{DataScope: 3}}, DataPermission{DeptIDs: []int64{2}}},
		{"descendants", []Role{{DataScope: 4}}, DataPermission{DeptIDs: []int64{2, 3}}},
		{"union", []Role{{DataScope: 2, DataScopeDeptIDs: []int64{4, 2, 4}}, {DataScope: 4}, {DataScope: 5}}, DataPermission{Self: true, DeptIDs: []int64{2, 3, 4}}},
		{"all", []Role{{DataScope: 1}}, DataPermission{All: true, DeptIDs: []int64{}}},
		{"disabled", []Role{{DataScope: 1, Status: 1}}, DataPermission{DeptIDs: []int64{}}},
	}
	for _, tt := range cases {
		t.Run(tt.Name, func(t *testing.T) {
			if got := MergeDataPermissions(tt.Roles, 2, depts); !reflect.DeepEqual(got, tt.Want) {
				t.Fatalf("got %+v want %+v", got, tt.Want)
			}
		})
	}
}
func TestRoleMenuDiffAndPackageReclamation(t *testing.T) {
	added, removed := MenuDiff([]int64{1, 2, 3}, []int64{2, 4, 4})
	if !reflect.DeepEqual(added, []int64{4}) || !reflect.DeepEqual(removed, []int64{1, 3}) {
		t.Fatal(added, removed)
	}
	allowed := []int64{2, 4}
	admin := Role{Type: 1, Code: "tenant_admin"}
	if got := PackageRoleMenus(admin, []int64{1}, allowed); !reflect.DeepEqual(got, allowed) {
		t.Fatal(got)
	}
	if got := PackageRoleMenus(Role{Type: 2}, []int64{1, 2, 3}, allowed); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatal(got)
	}
	if got := PackageRoleMenus(Role{Type: 2}, []int64{1}, nil); got == nil || len(got) != 0 {
		t.Fatal(got)
	}
}
func TestMenuHierarchyAndTree(t *testing.T) {
	menus := []Menu{{ID: 1, Type: 1}, {ID: 2, ParentID: 1, Type: 2}, {ID: 3, ParentID: 2, Type: 3}, {ID: 4, ParentID: 1, Type: 2, Status: 1}, {ID: 5, ParentID: 4, Type: 2}}
	if !errors.Is(ValidateMenuParent(menus, Menu{ID: 1, ParentID: 2}), ErrTreeCycle) {
		t.Fatal("menu cycle accepted")
	}
	if !errors.Is(ValidateMenuParent(menus, Menu{ParentID: 3}), ErrInvalidArgument) {
		t.Fatal("button parent accepted")
	}
	tree := MenuTree(menus)
	if len(tree) != 1 || len(tree[0].Children) != 1 || len(tree[0].Children[0].Children) != 0 {
		t.Fatal(tree)
	}
	if enabled := EnabledMenus(menus); len(enabled) != 3 {
		t.Fatal("disabled ancestors retained permissions", enabled)
	}
}
func TestTenantExpiryAndQuota(t *testing.T) {
	now := time.Now()
	tenant := Tenant{PackageID: 2, AccountCount: 2, ExpireTime: now.Add(time.Hour)}
	if err := tenant.CheckQuota(1); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(tenant.CheckQuota(2), ErrQuotaExceeded) {
		t.Fatal("quota exceeded")
	}
	if err := tenant.Available(now); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(tenant.Available(now.Add(2*time.Hour)), ErrTenantUnavailable) {
		t.Fatal("expiry ignored")
	}
	tenant.PackageID = 0
	if err := tenant.CheckQuota(999); err != nil {
		t.Fatal("system tenant quota", err)
	}
}
