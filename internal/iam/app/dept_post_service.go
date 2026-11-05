package app

import (
	"context"
	"slices"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func (s *Service) ListDepts(ctx context.Context, q Query) (pagination.PageResult[domain.Dept], error) {
	return checkedPage(ctx, s.Depts, q)
}
func (s *Service) GetDept(ctx context.Context, id int64) (domain.Dept, error) {
	return checkedGet(ctx, s.Depts, id)
}
func (s *Service) SaveDept(ctx context.Context, v domain.Dept) (int64, error) {
	err := s.mutate(ctx, false, func(ctx context.Context) error {
		if v.ID > 0 {
			if _, err := s.Depts.Get(ctx, v.ID); err != nil {
				return err
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		depts, err := s.Depts.List(ctxmeta.WithoutDataScope(ctx), all())
		if err != nil {
			return err
		}
		if err := domain.ValidateDeptParent(depts.List, v.ID, v.ParentID); err != nil {
			return err
		}
		if v.ParentID > 0 {
			if _, err := s.Depts.Get(ctx, v.ParentID); err != nil {
				return err
			}
		}
		if v.LeaderUserID > 0 {
			if _, err := s.Users.Get(ctx, v.LeaderUserID); err != nil {
				return err
			}
		}
		if err := s.Depts.Save(ctx, &v); err != nil {
			return err
		}
		return s.invalidate(ctx, "departments")
	})
	return v.ID, err
}
func (s *Service) DeleteDepts(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		depts, err := s.Depts.List(ctxmeta.WithoutDataScope(ctx), all())
		if err != nil {
			return err
		}
		users, err := s.Users.List(ctxmeta.WithoutDataScope(ctx), all())
		if err != nil {
			return err
		}
		for _, id := range domain.UniqueIDs(ids) {
			if _, err := s.Depts.Get(ctx, id); err != nil {
				return err
			}
			for _, d := range depts.List {
				if d.ParentID == id && !slices.Contains(ids, d.ID) {
					return domain.ErrHasChildren
				}
			}
			for _, u := range users.List {
				if u.DeptID == id {
					return domain.ErrHasChildren
				}
			}
			if err := s.Depts.Delete(ctx, id); err != nil {
				return err
			}
		}
		return s.invalidate(ctx, "departments")
	})
}
func (s *Service) PagePosts(ctx context.Context, q Query) (pagination.PageResult[domain.Post], error) {
	return checkedPage(ctx, s.Posts, q)
}
func (s *Service) GetPost(ctx context.Context, id int64) (domain.Post, error) {
	return checkedGet(ctx, s.Posts, id)
}
func (s *Service) SavePost(ctx context.Context, v domain.Post) (int64, error) {
	err := s.mutate(ctx, false, func(ctx context.Context) error {
		if v.ID > 0 {
			if _, err := s.Posts.Get(ctx, v.ID); err != nil {
				return err
			}
		}
		if err := v.Validate(); err != nil {
			return err
		}
		return s.Posts.Save(ctx, &v)
	})
	return v.ID, err
}
func (s *Service) DeletePosts(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, false, func(ctx context.Context) error {
		users, err := s.Users.List(ctxmeta.WithoutDataScope(ctx), all())
		if err != nil {
			return err
		}
		for _, id := range domain.UniqueIDs(ids) {
			if _, err := s.Posts.Get(ctx, id); err != nil {
				return err
			}
			for _, u := range users.List {
				if slices.Contains(u.PostIDs, id) {
					return domain.ErrHasChildren
				}
			}
			if err := s.Posts.Delete(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
}
