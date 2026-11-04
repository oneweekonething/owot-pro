package app

import (
	"context"
	"slices"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type AuditLogService struct {
	operate OperateLogRepository
	login   LoginLogRepository
	users   UserNicknameReader
}

func NewAuditLogService(operate OperateLogRepository, login LoginLogRepository, users UserNicknameReader) *AuditLogService {
	return &AuditLogService{operate: operate, login: login, users: users}
}

func (s *AuditLogService) RecordOperateLog(ctx context.Context, value domain.OperateLog) error {
	if err := requireTenant(ctx); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	value.ID = 0
	value.TenantID = ctxmeta.TenantID(ctx)
	if value.TraceID == "" {
		value.TraceID = ctxmeta.TraceID(ctx)
	}
	return wrap("record operation audit", s.operate.AppendOperateLog(ctx, &value))
}

func (s *AuditLogService) RecordLoginLog(ctx context.Context, value domain.LoginLog) error {
	if err := requireTenant(ctx); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	value.ID = 0
	value.TenantID = ctxmeta.TenantID(ctx)
	if value.TraceID == "" {
		value.TraceID = ctxmeta.TraceID(ctx)
	}
	return wrap("record login audit", s.login.AppendLoginLog(ctx, &value))
}

func (s *AuditLogService) PageOperateLogs(ctx context.Context, filter OperateLogFilter) (pagination.PageResult[domain.OperateLog], error) {
	empty := pagination.PageResult[domain.OperateLog]{List: []domain.OperateLog{}}
	if err := requireTenant(ctx); err != nil {
		return empty, err
	}
	if !validRange(filter.From, filter.To) || (filter.UserID != nil && *filter.UserID <= 0) ||
		(filter.BizID != nil && *filter.BizID < 0) {
		return empty, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	if filter.UserNickname != "" {
		if s.users == nil {
			return empty, nil
		}
		ids, err := s.users.FindUserIDsByNickname(ctx, filter.UserNickname)
		if err != nil {
			return empty, wrap("find audit users", err)
		}
		if len(ids) == 0 {
			return empty, nil
		}
		filter.UserIDs = ids
	}
	result, err := s.operate.PageOperateLogs(ctx, filter)
	if err != nil {
		return empty, wrap("page operation audits", err)
	}
	if s.users != nil && len(result.List) > 0 {
		ids := make([]int64, 0, len(result.List))
		for _, log := range result.List {
			ids = append(ids, log.UserID)
		}
		slices.Sort(ids)
		names, err := s.users.Nicknames(ctx, slices.Compact(ids))
		if err != nil {
			return empty, wrap("read audit nicknames", err)
		}
		for i := range result.List {
			result.List[i].UserNickname = names[result.List[i].UserID]
		}
	}
	return result, nil
}

func (s *AuditLogService) PageLoginLogs(ctx context.Context, filter LoginLogFilter) (pagination.PageResult[domain.LoginLog], error) {
	if err := requireTenant(ctx); err != nil {
		return pagination.PageResult[domain.LoginLog]{}, err
	}
	if !validRange(filter.From, filter.To) {
		return pagination.PageResult[domain.LoginLog]{}, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.login.PageLoginLogs(ctx, filter)
	return result, wrap("page login audits", err)
}
