package app

import (
	"context"
	"fmt"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type APILogService struct {
	access AccessLogRepository
	errors ErrorLogRepository
}

func NewAPILogService(access AccessLogRepository, errors ErrorLogRepository) *APILogService {
	return &APILogService{access: access, errors: errors}
}

func validateLogTenant(ctx context.Context) error {
	if ctxmeta.TenantID(ctx) <= 0 || ctxmeta.IsIgnoreTenant(ctx) {
		return domain.ErrInvalidArgument
	}
	return nil
}

func (s *APILogService) RecordAccess(ctx context.Context, entry domain.APIAccessLog) error {
	if err := validateLogTenant(ctx); err != nil {
		return err
	}
	if err := s.access.CreateAccess(ctx, entry); err != nil {
		return fmt.Errorf("record API access: %w", err)
	}
	return nil
}

func (s *APILogService) RecordError(ctx context.Context, entry domain.APIErrorLog) error {
	if err := validateLogTenant(ctx); err != nil {
		return err
	}
	entry.ProcessStatus, entry.ProcessUserID, entry.ProcessTime = domain.ErrorUnhandled, 0, nil
	if err := s.errors.CreateError(ctx, entry); err != nil {
		return fmt.Errorf("record API error: %w", err)
	}
	return nil
}

func (s *APILogService) Access(ctx context.Context, id int64) (domain.APIAccessLog, error) {
	if err := validateLogTenant(ctx); err != nil {
		return domain.APIAccessLog{}, err
	}
	result, err := s.access.GetAccess(ctx, id)
	if err != nil {
		return result, fmt.Errorf("get API access log %d: %w", id, err)
	}
	return result, nil
}

func (s *APILogService) Error(ctx context.Context, id int64) (domain.APIErrorLog, error) {
	if err := validateLogTenant(ctx); err != nil {
		return domain.APIErrorLog{}, err
	}
	result, err := s.errors.GetError(ctx, id)
	if err != nil {
		return result, fmt.Errorf("get API error log %d: %w", id, err)
	}
	return result, nil
}

func (s *APILogService) AccessPage(ctx context.Context, filter AccessLogFilter) (pagination.PageResult[domain.APIAccessLog], error) {
	if err := validateLogTenant(ctx); err != nil {
		return pagination.PageResult[domain.APIAccessLog]{}, err
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.access.PageAccess(ctx, filter)
	if err != nil {
		return result, fmt.Errorf("page API access logs: %w", err)
	}
	return result, nil
}

func (s *APILogService) ErrorPage(ctx context.Context, filter ErrorLogFilter) (pagination.PageResult[domain.APIErrorLog], error) {
	if err := validateLogTenant(ctx); err != nil {
		return pagination.PageResult[domain.APIErrorLog]{}, err
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.errors.PageError(ctx, filter)
	if err != nil {
		return result, fmt.Errorf("page API error logs: %w", err)
	}
	return result, nil
}

func (s *APILogService) ProcessError(ctx context.Context, id int64, status int) error {
	if err := validateLogTenant(ctx); err != nil {
		return err
	}
	user, ok := ctxmeta.User(ctx)
	if !ok || user.ID <= 0 || id <= 0 || status != domain.ErrorProcessed && status != domain.ErrorIgnored {
		return domain.ErrInvalidArgument
	}
	if err := s.errors.ProcessError(ctx, id, status, user.ID, time.Now().UTC()); err != nil {
		return fmt.Errorf("process API error log %d: %w", id, err)
	}
	return nil
}
