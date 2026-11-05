package http

import (
	"context"
	"strconv"
	"strings"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
)

type APILogWriter struct{ service *app.APILogService }

func NewAPILogWriter(service *app.APILogService) *APILogWriter {
	return &APILogWriter{service: service}
}

func requestLog(ctx context.Context, access httpx.AccessLog) domain.RequestLog {
	user, _ := ctxmeta.User(ctx)
	return domain.RequestLog{TraceID: ctxmeta.TraceID(ctx), ApplicationName: "owot-pro", UserID: user.ID, UserType: user.UserType,
		RequestMethod: access.Method, RequestURL: access.URL, RequestParams: access.Params, UserIP: access.UserIP,
		UserAgent: access.UserAgent, ResultCode: access.ResultCode}
}

func (w *APILogWriter) WriteAccess(ctx context.Context, access httpx.AccessLog) error {
	return w.service.RecordAccess(ctx, domain.APIAccessLog{RequestLog: requestLog(ctx, access),
		ResponseBody: `{"code":` + strconv.Itoa(access.ResultCode) + `}`, ResultMsg: access.ResultMessage,
		OperateModule: access.Module, OperateName: access.Operation, OperateType: access.OperationType,
		BeginTime: access.BeginTime.UTC(), EndTime: access.EndTime.UTC(), Duration: access.EndTime.Sub(access.BeginTime).Milliseconds()})
}

func (w *APILogWriter) WriteError(ctx context.Context, access httpx.AccessLog, fault httpx.ErrorLog) error {
	class, method := "", fault.Function
	base := strings.LastIndexByte(method, '/') + 1
	if i := strings.IndexByte(method[base:], '.'); i >= 0 {
		offset := base + i
		class, method = method[:offset], method[offset+1:]
	}
	return w.service.RecordError(ctx, domain.APIErrorLog{RequestLog: requestLog(ctx, access), ExceptionTime: fault.At,
		ExceptionName: fault.Name, ExceptionMessage: access.ResultMessage, ExceptionRootCauseMessage: access.ResultMessage,
		ExceptionStackTrace: fault.Stack, ExceptionClassName: class, ExceptionFileName: fault.File, ExceptionMethodName: method, ExceptionLineNumber: fault.Line})
}
