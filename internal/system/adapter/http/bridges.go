package http

import (
	"context"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type OperationRecorder struct{ audit *app.AuditLogService }

func NewOperationRecorder(audit *app.AuditLogService) *OperationRecorder {
	return &OperationRecorder{audit: audit}
}

func (b *OperationRecorder) Record(ctx context.Context, value httpx.OperateLog) error {
	user, ok := ctxmeta.User(ctx)
	if !ok {
		return domain.ErrInvalidArgument
	}
	// Headers are client-controlled; preserve the audit when metadata exceeds storage limits.
	if agent := []rune(value.UserAgent); len(agent) > domain.MaxUserAgentLength {
		value.UserAgent = string(agent[:domain.MaxUserAgentLength])
	}
	return b.audit.RecordOperateLog(ctx, domain.OperateLog{UserID: user.ID, UserType: user.UserType,
		Type: value.Type, SubType: value.SubType, BizID: value.BizID, Action: value.Action,
		RequestMethod: value.RequestMethod, RequestURL: value.RequestURL, UserIP: value.UserIP, UserAgent: value.UserAgent})
}

type DictionaryTranslator struct{ dict *app.DictService }

func NewDictionaryTranslator(dict *app.DictService) *DictionaryTranslator {
	return &DictionaryTranslator{dict: dict}
}

func (b *DictionaryTranslator) Label(ctx context.Context, typ, value string) (string, error) {
	return b.dict.GetLabel(ctx, typ, value)
}

func (b *DictionaryTranslator) Value(ctx context.Context, typ, label string) (string, error) {
	return b.dict.GetValue(ctx, typ, label)
}

var _ httpx.OperateLogRecorder = (*OperationRecorder)(nil)
var _ excelx.DictTranslator = (*DictionaryTranslator)(nil)
