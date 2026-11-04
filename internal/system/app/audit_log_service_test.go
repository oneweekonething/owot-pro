package app

import (
	"context"
	"errors"
	"testing"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type auditStub struct {
	operate domain.OperateLog
	login   domain.LoginLog
	err     error
}

func (s *auditStub) AppendOperateLog(_ context.Context, value *domain.OperateLog) error {
	s.operate = *value
	return s.err
}

func (s *auditStub) AppendLoginLog(_ context.Context, value *domain.LoginLog) error {
	s.login = *value
	return s.err
}

func (s *auditStub) PageOperateLogs(context.Context, OperateLogFilter) (pagination.PageResult[domain.OperateLog], error) {
	return pagination.PageResult[domain.OperateLog]{}, s.err
}

func (s *auditStub) PageLoginLogs(context.Context, LoginLogFilter) (pagination.PageResult[domain.LoginLog], error) {
	return pagination.PageResult[domain.LoginLog]{}, s.err
}

func TestAuditServiceOwnsTenantAndTrace(t *testing.T) {
	stub := &auditStub{}
	service := NewAuditLogService(stub, stub, nil)
	ctx := ctxmeta.WithTraceID(ctxmeta.WithTenantID(context.Background(), 1), "request-trace")
	if err := service.RecordOperateLog(ctx, domain.OperateLog{ID: 99, TenantID: 2, UserID: 7, UserType: 1, Type: "dict", SubType: "create"}); err != nil {
		t.Fatal(err)
	}
	if stub.operate.ID != 0 || stub.operate.TenantID != 1 || stub.operate.TraceID != "request-trace" {
		t.Fatalf("caller forged audit identity: %+v", stub.operate)
	}
	if err := service.RecordLoginLog(ctx, domain.LoginLog{ID: 99, TenantID: 2, Username: "unknown", UserType: 1, LogType: 100, Result: 10}); err != nil {
		t.Fatal(err)
	}
	if stub.login.ID != 0 || stub.login.TenantID != 1 || stub.login.TraceID != "request-trace" {
		t.Fatalf("caller forged login identity: %+v", stub.login)
	}
	want := errors.New("database unavailable")
	stub.err = want
	if err := service.RecordLoginLog(ctx, domain.LoginLog{Username: "user", UserType: 1, LogType: 100}); !errors.Is(err, want) {
		t.Fatalf("infrastructure error chain lost: %v", err)
	}
	if _, err := service.PageLoginLogs(context.Background(), LoginLogFilter{}); !errors.Is(err, domain.ErrTenantRequired) {
		t.Fatalf("missing tenant allowed: %v", err)
	}
}
