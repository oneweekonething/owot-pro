package app

import (
	"context"
	"slices"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func (s *Service) Snapshot(ctx context.Context, userID int64) (PermissionSnapshot, error) {
	ctx = ctxmeta.WithoutDataScope(ctx)
	load := func(ctx context.Context) (PermissionSnapshot, error) {
		var out PermissionSnapshot
		var err error
		out.User, err = s.Users.Get(ctx, userID)
		if err != nil {
			return out, err
		}
		ids, err := s.Links.UserRoles(ctx, userID)
		if err != nil {
			return out, err
		}
		out.Roles = []domain.Role{}
		out.Menus = []domain.Menu{}
		out.Depts = []domain.Dept{}
		menuIDs := []int64{}
		super := false
		for _, id := range ids {
			r, err := s.Roles.Get(ctx, id)
			if err != nil {
				return out, err
			}
			if r.Status != domain.StatusEnabled {
				continue
			}
			out.Roles = append(out.Roles, r)
			if r.Code == "super_admin" && r.BuiltIn() && out.User.TenantID == 1 {
				super = true
			}
			m, err := s.Links.RoleMenus(ctx, id)
			if err != nil {
				return out, err
			}
			menuIDs = append(menuIDs, m...)
		}
		menus, err := s.Menus.List(ctx, all())
		if err != nil {
			return out, err
		}
		t, err := s.Tenants.Get(ctx, ctxmeta.TenantID(ctx))
		if err != nil {
			return out, err
		}
		allowed := []int64{}
		if t.PackageID != 0 {
			p, err := s.Packages.Get(ctx, t.PackageID)
			if err != nil {
				return out, err
			}
			if p.Status != 0 {
				return out, domain.ErrTenantUnavailable
			}
			allowed = p.MenuIDs
		}
		for _, m := range domain.EnabledMenus(menus.List) {
			if m.Status == 0 && (super || slices.Contains(menuIDs, m.ID)) && (t.PackageID == 0 || slices.Contains(allowed, m.ID)) {
				out.Menus = append(out.Menus, m)
			}
		}
		depts, err := s.Depts.List(ctx, all())
		if err != nil {
			return out, err
		}
		out.Depts = depts.List
		out.User.PasswordHash = ""
		return out, nil
	}
	if s.Permissions == nil {
		return load(ctx)
	}
	return s.Permissions.Load(ctx, userID, load)
}

func (s *Service) HasPermission(ctx context.Context, user ctxmeta.LoginUser, permission string) (bool, error) {
	// A system administrator visiting a tenant is still the system principal.
	if user.SuperAdmin {
		return true, nil
	}
	info, err := s.Snapshot(ctx, user.ID)
	if err != nil {
		return false, err
	}
	for _, m := range info.Menus {
		if m.Permission == permission {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) DataScope(ctx context.Context, user ctxmeta.LoginUser) (ctxmeta.DataScope, error) {
	if user.SuperAdmin {
		return ctxmeta.DataScope{All: true}, nil
	}
	info, err := s.Snapshot(ctx, user.ID)
	if err != nil {
		return ctxmeta.DataScope{}, err
	}
	d := domain.MergeDataPermissions(info.Roles, info.User.DeptID, info.Depts)
	return ctxmeta.DataScope{All: d.All, Self: d.Self, DeptIDs: d.DeptIDs, UserID: user.ID}, nil
}

func (s *Service) PageRoles(ctx context.Context, q Query) (pagination.PageResult[domain.Role], error) {
	return checkedPage(ctx, s.Roles, q)
}
func (s *Service) GetRole(ctx context.Context, id int64) (domain.Role, error) {
	return checkedGet(ctx, s.Roles, id)
}
func (s *Service) SaveRole(ctx context.Context, v domain.Role) (int64, error) {
	err := s.mutate(ctx, false, func(ctx context.Context) error {
		if v.ID == 0 {
			v.Type = 2
			v.DataScope = domain.DataScopeAll
			if v.Code == "super_admin" || v.Code == "tenant_admin" {
				return domain.ErrProtected
			}
		} else {
			old, err := s.Roles.Get(ctx, v.ID)
			if err != nil {
				return err
			}
			v.Type, v.DataScope, v.DataScopeDeptIDs = old.Type, old.DataScope, old.DataScopeDeptIDs
			if err := old.ValidateChange(v); err != nil {
				return err
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if err := s.Roles.Save(ctx, &v); err != nil {
			return err
		}
		return s.invalidate(ctx, "roles")
	})
	return v.ID, err
}
func (s *Service) DeleteRoles(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		for _, id := range domain.UniqueIDs(ids) {
			v, err := s.Roles.Get(ctx, id)
			if err != nil {
				return err
			}
			if v.BuiltIn() {
				return domain.ErrProtected
			}
			if err := s.Links.RemoveReferences(ctx, "role", id); err != nil {
				return err
			}
			if err := s.Roles.Delete(ctx, id); err != nil {
				return err
			}
		}
		return s.invalidate(ctx, "roles")
	})
}

func (s *Service) UpdateRoleStatus(ctx context.Context, id int64, status int) error {
	if !domain.ValidStatus(status) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		role, err := s.Roles.Get(ctx, id)
		if err != nil {
			return err
		}
		next := role
		next.Status = status
		if err := role.ValidateChange(next); err != nil {
			return err
		}
		if err := s.Roles.Save(ctx, &next); err != nil {
			return err
		}
		return s.invalidate(ctx, "roles")
	})
}
func (s *Service) RoleMenuIDs(ctx context.Context, id int64) ([]int64, error) {
	if _, err := s.Roles.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.Links.RoleMenus(ctx, id)
}
func (s *Service) UserRoleIDs(ctx context.Context, id int64) ([]int64, error) {
	if _, err := s.Users.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.Links.UserRoles(ctx, id)
}
func (s *Service) AssignUserRoles(ctx context.Context, userID int64, ids []int64) error {
	if !domain.ValidIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Users.Get(ctx, userID)
		if err != nil {
			return err
		}
		for _, id := range domain.UniqueIDs(ids) {
			r, err := s.Roles.Get(ctx, id)
			if err != nil {
				return err
			}
			if r.Code == "super_admin" {
				if err := requireSuper(ctx); err != nil {
					return err
				}
			}
		}
		if u.IsInitialAdmin() {
			old, err := s.Links.UserRoles(ctx, userID)
			if err != nil {
				return err
			}
			for _, id := range old {
				r, err := s.Roles.Get(ctx, id)
				if err != nil {
					return err
				}
				if r.Code == "super_admin" && !slices.Contains(ids, id) {
					return domain.ErrProtected
				}
			}
		}
		if err := s.Links.AssignUserRoles(ctx, userID, domain.UniqueIDs(ids)); err != nil {
			return err
		}
		return s.invalidate(ctx, "user-roles")
	})
}
func (s *Service) validateMenus(ctx context.Context, ids []int64) error {
	if !domain.ValidIDs(ids) {
		return domain.ErrInvalidArgument
	}
	for _, id := range domain.UniqueIDs(ids) {
		if _, err := s.Menus.Get(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) AssignRoleMenus(ctx context.Context, roleID int64, ids []int64) error {
	return s.mutate(ctx, false, func(ctx context.Context) error {
		if _, err := s.Roles.Get(ctx, roleID); err != nil {
			return err
		}
		if err := s.validateMenus(ctx, ids); err != nil {
			return err
		}
		t, err := s.Tenants.Get(ctx, ctxmeta.TenantID(ctx))
		if err != nil {
			return err
		}
		if t.PackageID != 0 {
			p, err := s.Packages.Get(ctx, t.PackageID)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if !slices.Contains(p.MenuIDs, id) {
					return domain.ErrForbidden
				}
			}
		}
		if err := s.Links.AssignRoleMenus(ctx, roleID, domain.UniqueIDs(ids)); err != nil {
			return err
		}
		return s.invalidate(ctx, "role-menus")
	})
}
func (s *Service) AssignDataScope(ctx context.Context, roleID int64, scope int, ids []int64) error {
	return s.mutate(ctx, false, func(ctx context.Context) error {
		r, err := s.Roles.Get(ctx, roleID)
		if err != nil {
			return err
		}
		r.DataScope, r.DataScopeDeptIDs = scope, domain.UniqueIDs(ids)
		if err := r.Validate(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := s.Depts.Get(ctx, id); err != nil {
				return err
			}
		}
		if err := s.Roles.Save(ctx, &r); err != nil {
			return err
		}
		return s.invalidate(ctx, "data-scope")
	})
}

func (s *Service) ListMenus(ctx context.Context, q Query) (pagination.PageResult[domain.Menu], error) {
	result, err := checkedPage(ctx, s.Menus, q)
	if err != nil {
		return result, err
	}
	t, err := s.Tenants.Get(ctx, ctxmeta.TenantID(ctx))
	if err != nil {
		return result, err
	}
	if t.PackageID == 0 {
		return result, nil
	}
	p, err := s.Packages.Get(ctx, t.PackageID)
	if err != nil {
		return result, err
	}
	list := []domain.Menu{}
	for _, m := range result.List {
		if slices.Contains(p.MenuIDs, m.ID) {
			list = append(list, m)
		}
	}
	result.List, result.Total = list, int64(len(list))
	return result, nil
}
func (s *Service) GetMenu(ctx context.Context, id int64) (domain.Menu, error) {
	if err := requireSuper(ctx); err != nil {
		return domain.Menu{}, err
	}
	return checkedGet(ctx, s.Menus, id)
}
func (s *Service) SaveMenu(ctx context.Context, v domain.Menu) (int64, error) {
	err := s.mutate(ctx, true, func(ctx context.Context) error {
		if v.ID > 0 {
			if _, err := s.Menus.Get(ctx, v.ID); err != nil {
				return err
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		menus, err := s.Menus.List(ctx, all())
		if err != nil {
			return err
		}
		if err := domain.ValidateMenuParent(menus.List, v); err != nil {
			return err
		}
		if v.Type == 3 {
			for _, m := range menus.List {
				if m.ParentID == v.ID && v.ID > 0 {
					return domain.ErrHasChildren
				}
			}
		}
		if err := s.Menus.Save(ctx, &v); err != nil {
			return err
		}
		return s.invalidate(ctxmeta.WithTenantID(ctx, 0), "menus")
	})
	return v.ID, err
}
func (s *Service) DeleteMenu(ctx context.Context, id int64) error {
	return s.mutate(ctx, true, func(ctx context.Context) error {
		if _, err := s.Menus.Get(ctx, id); err != nil {
			return err
		}
		menus, err := s.Menus.List(ctx, all())
		if err != nil {
			return err
		}
		for _, m := range menus.List {
			if m.ParentID == id {
				return domain.ErrHasChildren
			}
		}
		tenants, err := s.Tenants.List(ctx, all())
		if err != nil {
			return err
		}
		for _, t := range tenants.List {
			target := ctxmeta.WithTenantID(ctx, t.ID)
			if err := s.Locks.LockTenant(target, t.ID); err != nil {
				return err
			}
			if err := s.Links.RemoveReferences(target, "menu", id); err != nil {
				return err
			}
		}
		packages, err := s.Packages.List(ctx, all())
		if err != nil {
			return err
		}
		for _, p := range packages.List {
			next := slices.DeleteFunc(p.MenuIDs, func(v int64) bool { return v == id })
			if len(next) != len(p.MenuIDs) {
				p.MenuIDs = next
				if err := s.Packages.Save(ctx, &p); err != nil {
					return err
				}
			}
		}
		if err := s.Menus.Delete(ctx, id); err != nil {
			return err
		}
		return s.invalidate(ctxmeta.WithTenantID(ctx, 0), "menus")
	})
}
