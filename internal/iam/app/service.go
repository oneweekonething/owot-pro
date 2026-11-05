package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type Service struct{ Dependencies }

func NewService(deps Dependencies) *Service { return &Service{Dependencies: deps} }
func all() Query                            { return Query{PageQuery: pagination.PageQuery{PageSize: -1}} }
func validQuery(q Query) bool {
	return (q.Status == nil || domain.ValidStatus(*q.Status)) && (q.From == nil || q.To == nil || !q.From.After(*q.To))
}
func validIDs(ids []int64) bool { return len(ids) > 0 && len(ids) <= 1000 && domain.ValidIDs(ids) }
func requireSuper(ctx context.Context) error {
	u, ok := ctxmeta.User(ctx)
	if !ok || !u.SuperAdmin || u.TenantID != 1 {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) mutate(ctx context.Context, global bool, fn func(context.Context) error) error {
	if global {
		if err := requireSuper(ctx); err != nil {
			return err
		}
	}
	err := s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if global {
			if err := s.Locks.LockGlobal(ctx); err != nil {
				return fmt.Errorf("lock IAM catalog: %w", err)
			}
		} else {
			if ctxmeta.TenantID(ctx) <= 0 {
				return domain.ErrForbidden
			}
			if err := s.Locks.LockTenant(ctx, ctxmeta.TenantID(ctx)); err != nil {
				return fmt.Errorf("lock IAM tenant: %w", err)
			}
		}
		return fn(ctx)
	})
	return wrap("mutate IAM", err)
}

func (s *Service) invalidate(ctx context.Context, scope string) error {
	payload, err := json.Marshal(struct {
		Scope string `json:"scope"`
	}{scope})
	if err != nil {
		return err
	}
	if err := s.Outbox.Publish(ctx, "iam.perm.invalidate", payload); err != nil {
		return fmt.Errorf("publish IAM invalidation: %w", err)
	}
	return nil
}

func (s *Service) ValidateTenant(ctx context.Context, id int64) error {
	if id <= 0 {
		return domain.ErrTenantUnavailable
	}
	t, err := s.Tenants.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := t.Available(time.Now()); err != nil {
		return err
	}
	if t.PackageID > 0 {
		p, err := s.Packages.Get(ctx, t.PackageID)
		if err != nil {
			return err
		}
		if p.Status != 0 {
			return domain.ErrTenantUnavailable
		}
	}
	return nil
}

func checkedPage[T any](ctx context.Context, repo Repository[T], q Query) (pagination.PageResult[T], error) {
	if !validQuery(q) {
		return pagination.PageResult[T]{}, domain.ErrInvalidArgument
	}
	q.PageQuery = q.Normalize()
	result, err := repo.List(ctx, q)
	return result, wrap("list IAM records", err)
}

func checkedGet[T any](ctx context.Context, repo Repository[T], id int64) (T, error) {
	value, err := repo.Get(ctx, id)
	return value, wrap("get IAM record", err)
}
func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
