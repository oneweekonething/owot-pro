package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func ownContext(ctx context.Context) context.Context {
	u, _ := ctxmeta.User(ctx)
	return ctxmeta.WithTenantID(ctxmeta.WithoutDataScope(ctx), u.TenantID)
}

func (s *Service) PageUsers(ctx context.Context, q Query) (pagination.PageResult[domain.AdminUser], error) {
	if len(q.DeptIDs) > 0 {
		depts, err := s.Depts.List(ctxmeta.WithoutDataScope(ctx), all())
		if err != nil {
			return pagination.PageResult[domain.AdminUser]{}, err
		}
		q.DeptIDs = domain.DescendantIDs(depts.List, q.DeptIDs[0])
	}
	return checkedPage(ctx, s.Users, q)
}
func (s *Service) GetUser(ctx context.Context, id int64) (domain.AdminUser, error) {
	return checkedGet(ctx, s.Users, id)
}

func (s *Service) SaveUser(ctx context.Context, v domain.AdminUser, password string) (int64, error) {
	err := s.mutate(ctx, false, func(ctx context.Context) error {
		scope, scoped := ctxmeta.Scope(ctx)
		deptCtx := ctx
		if v.ID == 0 {
			// A newly assigned ID cannot be covered by a self-only scope. A
			// restricted creator must place the account in an authorized department.
			if scoped && !scope.All && !slices.Contains(scope.DeptIDs, v.DeptID) {
				return domain.ErrForbidden
			}
			if err := domain.ValidatePassword(password); err != nil {
				return err
			}
			t, err := s.Tenants.Get(ctx, ctxmeta.TenantID(ctx))
			if err != nil {
				return err
			}
			users, err := s.Users.List(ctxmeta.WithoutDataScope(ctx), Query{PageQuery: pagination.PageQuery{PageSize: 1}})
			if err != nil {
				return err
			}
			if err := t.CheckQuota(users.Total); err != nil {
				return err
			}
			v.PasswordHash, err = s.Hasher.Hash(password)
			if err != nil {
				return fmt.Errorf("hash new user password: %w", err)
			}
		} else {
			old, err := s.Users.Get(ctx, v.ID)
			if err != nil {
				return err
			}
			if old.DeptID != v.DeptID && scoped && !scope.All && !slices.Contains(scope.DeptIDs, v.DeptID) {
				return domain.ErrForbidden
			}
			if old.DeptID == v.DeptID {
				deptCtx = ctxmeta.WithoutDataScope(ctx)
			}
			v.PasswordHash, v.Avatar, v.LoginIP, v.LoginDate = old.PasswordHash, old.Avatar, old.LoginIP, old.LoginDate
			if v.Username == "" {
				v.Username = old.Username
			}
			if old.IsInitialAdmin() && (v.Status != 0 || v.Username != old.Username) {
				return domain.ErrProtected
			}
			if v.Status != 0 {
				if err := s.revoke(ctx, TokenSelection{UserID: v.ID}); err != nil {
					return err
				}
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.DeptID > 0 {
			dept, err := s.Depts.Get(deptCtx, v.DeptID)
			if err != nil {
				return err
			}
			if dept.Status != 0 {
				return domain.ErrDisabled
			}
		}
		for _, id := range domain.UniqueIDs(v.PostIDs) {
			post, err := s.Posts.Get(ctx, id)
			if err != nil {
				return err
			}
			if post.Status != 0 {
				return domain.ErrDisabled
			}
		}
		v.PostIDs = domain.UniqueIDs(v.PostIDs)
		if err := s.Users.Save(ctx, &v); err != nil {
			return err
		}
		return s.invalidate(ctx, "users")
	})
	return v.ID, err
}
func (s *Service) DeleteUsers(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		for _, id := range domain.UniqueIDs(ids) {
			u, err := s.Users.Get(ctx, id)
			if err != nil {
				return err
			}
			if u.IsInitialAdmin() {
				return domain.ErrProtected
			}
			t, err := s.Tenants.Get(ctx, ctxmeta.TenantID(ctx))
			if err != nil {
				return err
			}
			if t.ContactUserID == id {
				return domain.ErrProtected
			}
			if err := s.revoke(ctx, TokenSelection{UserID: id}); err != nil {
				return err
			}
			if err := s.Links.RemoveReferences(ctx, "user", id); err != nil {
				return err
			}
			if err := s.Users.Delete(ctx, id); err != nil {
				return err
			}
		}
		return s.invalidate(ctx, "users")
	})
}
func (s *Service) UpdateUserStatus(ctx context.Context, id int64, status int) error {
	if !domain.ValidStatus(status) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Users.Get(ctx, id)
		if err != nil {
			return err
		}
		if u.IsInitialAdmin() && status != 0 {
			return domain.ErrProtected
		}
		if status != 0 {
			if err := s.revoke(ctx, TokenSelection{UserID: id}); err != nil {
				return err
			}
		}
		u.Status = status
		if err := s.Users.Save(ctx, &u); err != nil {
			return err
		}
		return s.invalidate(ctx, "users")
	})
}
func (s *Service) UpdatePassword(ctx context.Context, id int64, password string) error {
	if err := domain.ValidatePassword(password); err != nil {
		return err
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Users.Get(ctx, id)
		if err != nil {
			return err
		}
		if err := s.authorizePasswordReset(ctx, u); err != nil {
			return err
		}
		return s.replacePassword(ctx, u, password)
	})
}

func (s *Service) authorizePasswordReset(ctx context.Context, target domain.AdminUser) error {
	if requireSuper(ctx) == nil {
		return nil
	}
	if target.IsInitialAdmin() {
		return domain.ErrForbidden
	}
	roles, err := s.Links.UserRoles(ctx, target.ID)
	if err != nil {
		return err
	}
	for _, id := range roles {
		r, err := s.Roles.Get(ctx, id)
		if err != nil {
			return err
		}
		if !r.BuiltIn() {
			continue
		}
		switch r.Code {
		case "super_admin":
			return domain.ErrForbidden
		case "tenant_admin":
			actor, ok := ctxmeta.User(ctx)
			if !ok {
				return domain.ErrForbidden
			}
			ids, err := s.Links.UserRoles(ctx, actor.ID)
			if err != nil {
				return err
			}
			if r.Status != domain.StatusEnabled || !slices.Contains(ids, r.ID) {
				return domain.ErrForbidden
			}
		}
	}
	return nil
}

func (s *Service) replacePassword(ctx context.Context, user domain.AdminUser, password string) error {
	hash, err := s.Hasher.Hash(password)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	if err := s.revoke(ctx, TokenSelection{UserID: user.ID}); err != nil {
		return err
	}
	return s.Users.Save(ctx, &user)
}
func (s *Service) Profile(ctx context.Context) (domain.AdminUser, error) {
	u, ok := ctxmeta.User(ctx)
	if !ok {
		return domain.AdminUser{}, domain.ErrForbidden
	}
	return checkedGet(ownContext(ctx), s.Users, u.ID)
}
func (s *Service) UpdateProfile(ctx context.Context, nickname, email, mobile string, sex int) error {
	ctx = ownContext(ctx)
	return s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Profile(ctx)
		if err != nil {
			return err
		}
		u.Nickname, u.Email, u.Mobile, u.Sex = nickname, email, mobile, sex
		if err := u.Validate(); err != nil {
			return err
		}
		if err := s.Users.Save(ctx, &u); err != nil {
			return err
		}
		return s.invalidate(ctx, "users")
	})
}
func (s *Service) UpdateOwnPassword(ctx context.Context, oldPassword, newPassword string) error {
	if err := domain.ValidatePassword(newPassword); err != nil {
		return err
	}
	ctx = ownContext(ctx)
	return s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Profile(ctx)
		if err != nil {
			return err
		}
		if s.Hasher.Compare(u.PasswordHash, oldPassword) != nil {
			return domain.ErrCredentials
		}
		return s.replacePassword(ctx, u, newPassword)
	})
}
func (s *Service) UpdateAvatar(ctx context.Context, name string, content []byte) (string, error) {
	if s.Avatars == nil {
		return "", domain.ErrInvalidArgument
	}
	ctx = ownContext(ctx)
	url, err := s.Avatars.Upload(ctx, name, content)
	if err != nil {
		return "", err
	}
	err = s.mutate(ctx, false, func(ctx context.Context) error {
		u, err := s.Profile(ctx)
		if err != nil {
			return err
		}
		u.Avatar = url
		if err := s.Users.Save(ctx, &u); err != nil {
			return err
		}
		return s.invalidate(ctx, "users")
	})
	return url, err
}

type ImportUser struct {
	User     domain.AdminUser
	Password string
}
type ImportResult struct {
	CreateUsernames  []string
	UpdateUsernames  []string
	FailureUsernames map[string]string
}

func (s *Service) ImportUsers(ctx context.Context, rows []ImportUser, update bool) (ImportResult, error) {
	result := ImportResult{CreateUsernames: []string{}, UpdateUsernames: []string{}, FailureUsernames: map[string]string{}}
	if len(rows) == 0 || len(rows) > 1000 {
		return result, domain.ErrInvalidArgument
	}
	for _, row := range rows {
		q := all()
		q.ExactUsername = row.User.Username
		found, err := s.Users.List(ctx, q)
		if err != nil {
			return result, err
		}
		if len(found.List) > 0 {
			if !update {
				result.FailureUsernames[row.User.Username] = "用户账号已经存在"
				continue
			}
			row.User.ID = found.List[0].ID
		}
		_, err = s.SaveUser(ctx, row.User, row.Password)
		if err != nil {
			if !errors.Is(err, domain.ErrInvalidArgument) && !errors.Is(err, domain.ErrDuplicate) && !errors.Is(err, domain.ErrProtected) && !errors.Is(err, domain.ErrQuotaExceeded) && !errors.Is(err, domain.ErrPassword) && !errors.Is(err, domain.ErrDeptNotFound) && !errors.Is(err, domain.ErrPostNotFound) {
				return result, err
			}
			result.FailureUsernames[row.User.Username] = "用户数据无效、账号重复或超出租户配额"
			continue
		}
		if row.User.ID == 0 {
			result.CreateUsernames = append(result.CreateUsernames, row.User.Username)
		} else {
			result.UpdateUsernames = append(result.UpdateUsernames, row.User.Username)
		}
	}
	return result, nil
}
