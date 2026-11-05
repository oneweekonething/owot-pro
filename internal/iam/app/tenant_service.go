package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func (s *Service) PageTenants(ctx context.Context, q Query) (pagination.PageResult[domain.Tenant], error) {
	if err := requireSuper(ctx); err != nil {
		return pagination.PageResult[domain.Tenant]{}, err
	}
	return checkedPage(ctx, s.Tenants, q)
}
func (s *Service) GetTenant(ctx context.Context, id int64) (domain.Tenant, error) {
	if err := requireSuper(ctx); err != nil {
		return domain.Tenant{}, err
	}
	return checkedGet(ctx, s.Tenants, id)
}
func (s *Service) EnabledTenants(ctx context.Context) ([]domain.Tenant, error) {
	q := all()
	status := 0
	q.Status = &status
	page, err := s.Tenants.List(ctx, q)
	if err != nil {
		return nil, err
	}
	result := []domain.Tenant{}
	for _, t := range page.List {
		if err := s.ValidateTenant(ctx, t.ID); err == nil {
			result = append(result, t)
		} else if !errors.Is(err, domain.ErrTenantUnavailable) && !errors.Is(err, domain.ErrTenantNotFound) && !errors.Is(err, domain.ErrPackageNotFound) {
			return nil, err
		}
	}
	return result, nil
}
func (s *Service) ResolveTenant(ctx context.Context, name, website string) (domain.Tenant, error) {
	if name == "" && website == "" {
		return domain.Tenant{}, domain.ErrInvalidArgument
	}
	tenants, err := s.EnabledTenants(ctx)
	if err != nil {
		return domain.Tenant{}, err
	}
	for _, t := range tenants {
		if name != "" && t.Name == name || website != "" && slices.Contains(t.Websites, website) {
			return t, nil
		}
	}
	return domain.Tenant{}, domain.ErrTenantNotFound
}

func (s *Service) SaveTenant(ctx context.Context, v domain.Tenant, username, password string) (int64, error) {
	err := s.mutate(ctx, true, func(ctx context.Context) error {
		if err := v.Validate(); err != nil {
			return err
		}
		tenants, err := s.Tenants.List(ctx, all())
		if err != nil {
			return err
		}
		for _, other := range tenants.List {
			if other.ID == v.ID {
				continue
			}
			for _, website := range v.Websites {
				if slices.Contains(other.Websites, website) {
					return domain.ErrDuplicate
				}
			}
		}
		if v.ID == 0 && v.PackageID == 0 {
			return domain.ErrProtected
		}
		if v.PackageID > 0 {
			p, err := s.Packages.Get(ctx, v.PackageID)
			if err != nil {
				return err
			}
			if p.Status != 0 {
				return domain.ErrTenantUnavailable
			}
		}
		if v.ID > 0 {
			if err := s.Locks.LockTenant(ctx, v.ID); err != nil {
				return err
			}
			old, err := s.Tenants.Get(ctx, v.ID)
			if err != nil {
				return err
			}
			if old.PackageID == 0 && (v.Status != 0 || v.PackageID != 0 || !v.ExpireTime.After(time.Now())) || old.PackageID != 0 && v.PackageID == 0 {
				return domain.ErrProtected
			}
			v.ContactUserID = old.ContactUserID
			if err := s.Tenants.Save(ctx, &v); err != nil {
				return err
			}
			target := ctxmeta.WithTenantID(ctx, v.ID)
			if old.PackageID != v.PackageID {
				if err := s.syncTenantMenus(target, v); err != nil {
					return err
				}
			}
			return s.invalidate(target, "tenant")
		}
		if err := domain.ValidatePassword(password); err != nil {
			return err
		}
		if err := s.Tenants.Save(ctx, &v); err != nil {
			return err
		}
		target := ctxmeta.WithoutDataScope(ctxmeta.WithTenantID(ctx, v.ID))
		dept := domain.Dept{Name: v.Name}
		if err := s.Depts.Save(target, &dept); err != nil {
			return err
		}
		role := domain.Role{Name: "租户管理员", Code: "tenant_admin", Type: 1, DataScope: domain.DataScopeAll}
		if err := s.Roles.Save(target, &role); err != nil {
			return err
		}
		p, err := s.Packages.Get(ctx, v.PackageID)
		if err != nil {
			return err
		}
		if err := s.Links.AssignRoleMenus(target, role.ID, p.MenuIDs); err != nil {
			return err
		}
		hash, err := s.Hasher.Hash(password)
		if err != nil {
			return err
		}
		user := domain.AdminUser{Username: username, Nickname: v.ContactName, PasswordHash: hash, DeptID: dept.ID, PostIDs: []int64{}}
		if err := user.Validate(); err != nil {
			return err
		}
		if err := s.Users.Save(target, &user); err != nil {
			return err
		}
		if err := s.Links.AssignUserRoles(target, user.ID, []int64{role.ID}); err != nil {
			return err
		}
		v.ContactUserID = user.ID
		if err := s.Tenants.Save(ctx, &v); err != nil {
			return err
		}
		return s.invalidate(target, "tenant-created")
	})
	return v.ID, err
}

func (s *Service) syncTenantMenus(ctx context.Context, t domain.Tenant) error {
	if t.PackageID == 0 {
		return nil
	}
	p, err := s.Packages.Get(ctx, t.PackageID)
	if err != nil {
		return err
	}
	roles, err := s.Roles.List(ctx, all())
	if err != nil {
		return err
	}
	for _, r := range roles.List {
		ids, err := s.Links.RoleMenus(ctx, r.ID)
		if err != nil {
			return err
		}
		if err := s.Links.AssignRoleMenus(ctx, r.ID, domain.PackageRoleMenus(r, ids, p.MenuIDs)); err != nil {
			return err
		}
	}
	return s.invalidate(ctx, "tenant-package")
}
func (s *Service) DeleteTenants(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, true, func(ctx context.Context) error {
		ids = domain.UniqueIDs(ids)
		slices.Sort(ids)
		for _, id := range ids {
			t, err := s.Tenants.Get(ctx, id)
			if err != nil {
				return err
			}
			if t.PackageID == 0 {
				return domain.ErrProtected
			}
			if err := s.Locks.LockTenant(ctx, id); err != nil {
				return err
			}
			target := ctxmeta.WithoutDataScope(ctxmeta.WithTenantID(ctx, id))
			if err := s.revoke(target, TokenSelection{}); err != nil {
				return err
			}
			if err := s.Links.RemoveReferences(target, "tenant", id); err != nil {
				return err
			}
			if err := deleteAll(target, s.Users, func(v domain.AdminUser) int64 { return v.ID }); err != nil {
				return err
			}
			if err := deleteAll(target, s.Roles, func(v domain.Role) int64 { return v.ID }); err != nil {
				return err
			}
			if err := deleteAll(target, s.Depts, func(v domain.Dept) int64 { return v.ID }); err != nil {
				return err
			}
			if err := deleteAll(target, s.Posts, func(v domain.Post) int64 { return v.ID }); err != nil {
				return err
			}
			if err := s.Tenants.Delete(ctx, id); err != nil {
				return err
			}
			if err := s.invalidate(target, "tenant-deleted"); err != nil {
				return err
			}
		}
		return nil
	})
}
func deleteAll[T any](ctx context.Context, repo Repository[T], id func(T) int64) error {
	values, err := repo.List(ctx, all())
	if err != nil {
		return err
	}
	for _, v := range values.List {
		if err := repo.Delete(ctx, id(v)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) PagePackages(ctx context.Context, q Query) (pagination.PageResult[domain.TenantPackage], error) {
	if err := requireSuper(ctx); err != nil {
		return pagination.PageResult[domain.TenantPackage]{}, err
	}
	return checkedPage(ctx, s.Packages, q)
}
func (s *Service) GetPackage(ctx context.Context, id int64) (domain.TenantPackage, error) {
	if err := requireSuper(ctx); err != nil {
		return domain.TenantPackage{}, err
	}
	return checkedGet(ctx, s.Packages, id)
}
func (s *Service) SavePackage(ctx context.Context, v domain.TenantPackage) (int64, error) {
	err := s.mutate(ctx, true, func(ctx context.Context) error {
		if v.ID > 0 {
			if _, err := s.Packages.Get(ctx, v.ID); err != nil {
				return err
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if err := s.validateMenus(ctx, v.MenuIDs); err != nil {
			return err
		}
		v.MenuIDs = domain.UniqueIDs(v.MenuIDs)
		if err := s.Packages.Save(ctx, &v); err != nil {
			return err
		}
		q := all()
		q.PackageID = &v.ID
		tenants, err := s.Tenants.List(ctx, q)
		if err != nil {
			return err
		}
		for _, t := range tenants.List {
			target := ctxmeta.WithTenantID(ctx, t.ID)
			if err := s.Locks.LockTenant(target, t.ID); err != nil {
				return err
			}
			if err := s.syncTenantMenus(target, t); err != nil {
				return err
			}
		}
		return s.invalidate(ctxmeta.WithTenantID(ctx, 0), "packages")
	})
	return v.ID, err
}
func (s *Service) DeletePackages(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, true, func(ctx context.Context) error {
		for _, id := range domain.UniqueIDs(ids) {
			if _, err := s.Packages.Get(ctx, id); err != nil {
				return err
			}
			q := all()
			q.PackageID = &id
			tenants, err := s.Tenants.List(ctx, q)
			if err != nil {
				return err
			}
			if tenants.Total > 0 {
				return domain.ErrHasChildren
			}
			if err := s.Packages.Delete(ctx, id); err != nil {
				return err
			}
		}
		return s.invalidate(ctxmeta.WithTenantID(ctx, 0), "packages")
	})
}
