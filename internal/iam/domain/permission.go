package domain

import "slices"

type DataPermission struct {
	All, Self bool
	DeptIDs   []int64
}

// MergeDataPermissions unions all enabled roles; an empty role set grants no rows.
func MergeDataPermissions(roles []Role, userDeptID int64, depts []Dept) DataPermission {
	result := DataPermission{DeptIDs: []int64{}}
	for _, r := range roles {
		if r.Status != StatusEnabled {
			continue
		}
		switch r.DataScope {
		case DataScopeAll:
			result.All = true
		case DataScopeCustom:
			result.DeptIDs = append(result.DeptIDs, r.DataScopeDeptIDs...)
		case DataScopeDept:
			if userDeptID > 0 {
				result.DeptIDs = append(result.DeptIDs, userDeptID)
			}
		case DataScopeDeptAndChildren:
			if userDeptID > 0 {
				result.DeptIDs = append(result.DeptIDs, DescendantIDs(depts, userDeptID)...)
			}
		case DataScopeSelf:
			result.Self = true
		}
	}
	result.DeptIDs = UniqueIDs(result.DeptIDs)
	slices.Sort(result.DeptIDs)
	return result
}

func MenuDiff(old, next []int64) (added, removed []int64) {
	added, removed = []int64{}, []int64{}
	for _, id := range UniqueIDs(next) {
		if !slices.Contains(old, id) {
			added = append(added, id)
		}
	}
	for _, id := range UniqueIDs(old) {
		if !slices.Contains(next, id) {
			removed = append(removed, id)
		}
	}
	return
}

func PackageRoleMenus(role Role, current, allowed []int64) []int64 {
	if role.Code == "tenant_admin" && role.BuiltIn() {
		return UniqueIDs(allowed)
	}
	result := []int64{}
	for _, id := range UniqueIDs(current) {
		if slices.Contains(allowed, id) {
			result = append(result, id)
		}
	}
	return result
}
