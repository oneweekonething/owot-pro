package app

import (
	"context"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type NoticeService struct{ repo NoticeRepository }

func NewNoticeService(repo NoticeRepository) *NoticeService { return &NoticeService{repo: repo} }

func (s *NoticeService) Create(ctx context.Context, value domain.Notice) (int64, error) {
	if err := requireTenant(ctx); err != nil {
		return 0, err
	}
	if err := value.Validate(); err != nil {
		return 0, err
	}
	value.ID = 0
	value.TenantID = ctxmeta.TenantID(ctx)
	err := s.repo.SaveNotice(ctx, &value)
	return value.ID, wrap("create notice", err)
}

func (s *NoticeService) Update(ctx context.Context, value domain.Notice) error {
	if err := requireTenant(ctx); err != nil {
		return err
	}
	if err := value.Validate(); err != nil || value.ID <= 0 {
		return domain.ErrInvalidArgument
	}
	value.TenantID = ctxmeta.TenantID(ctx)
	return wrap("update notice", s.repo.SaveNotice(ctx, &value))
}

func (s *NoticeService) Delete(ctx context.Context, id int64) error {
	if err := requireTenant(ctx); err != nil {
		return err
	}
	if id <= 0 {
		return domain.ErrInvalidArgument
	}
	return wrap("delete notice", s.repo.DeleteNotice(ctx, id))
}

func (s *NoticeService) Get(ctx context.Context, id int64) (domain.Notice, error) {
	if err := requireTenant(ctx); err != nil {
		return domain.Notice{}, err
	}
	if id <= 0 {
		return domain.Notice{}, domain.ErrInvalidArgument
	}
	value, err := s.repo.FindNotice(ctx, id)
	return value, wrap("get notice", err)
}

func (s *NoticeService) Page(ctx context.Context, filter NoticeFilter) (pagination.PageResult[domain.Notice], error) {
	if err := requireTenant(ctx); err != nil {
		return pagination.PageResult[domain.Notice]{}, err
	}
	if !validStatusFilter(filter.Status) || (filter.Type != nil && *filter.Type != 1 && *filter.Type != 2) {
		return pagination.PageResult[domain.Notice]{}, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.repo.PageNotices(ctx, filter)
	return result, wrap("page notices", err)
}
