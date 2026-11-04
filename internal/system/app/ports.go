// Package app implements System use cases through consumer-defined ports.
package app

import (
	"context"
	"time"

	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type TxRunner interface {
	WithinTx(context.Context, func(context.Context) error) error
}

type OutboxPublisher interface {
	Publish(context.Context, string, []byte) error
}

type DictTypeFilter struct {
	pagination.PageQuery
	Name, Type string
	Status     *int
	From, To   *time.Time
}

type DictDataFilter struct {
	pagination.PageQuery
	DictType, Label string
	Status          *int
}

type DictTypeRepository interface {
	FindType(context.Context, int64) (domain.DictType, error)
	// LockType serializes mutations of a type and its items in the current transaction.
	LockType(context.Context, string) (domain.DictType, error)
	SaveType(context.Context, *domain.DictType) error
	DeleteType(context.Context, int64) error
	PageTypes(context.Context, DictTypeFilter) (pagination.PageResult[domain.DictType], error)
}

type DictDataRepository interface {
	FindData(context.Context, int64) (domain.DictData, error)
	HasData(context.Context, string) (bool, error)
	SaveData(context.Context, *domain.DictData) error
	DeleteData(context.Context, int64) error
	PageData(context.Context, DictDataFilter) (pagination.PageResult[domain.DictData], error)
}

type NoticeFilter struct {
	pagination.PageQuery
	Title  string
	Status *int
	Type   *int
}

type NoticeRepository interface {
	FindNotice(context.Context, int64) (domain.Notice, error)
	SaveNotice(context.Context, *domain.Notice) error
	DeleteNotice(context.Context, int64) error
	PageNotices(context.Context, NoticeFilter) (pagination.PageResult[domain.Notice], error)
}

type OperateLogFilter struct {
	pagination.PageQuery
	UserID, BizID               *int64
	Type, SubType, UserNickname string
	Action                      string
	From, To                    *time.Time
	// UserIDs is resolved by the service from a nickname query within the tenant.
	UserIDs []int64
}

type LoginLogFilter struct {
	pagination.PageQuery
	Username, UserIP string
	Status           *bool
	From, To         *time.Time
}

type OperateLogRepository interface {
	AppendOperateLog(context.Context, *domain.OperateLog) error
	PageOperateLogs(context.Context, OperateLogFilter) (pagination.PageResult[domain.OperateLog], error)
}

type LoginLogRepository interface {
	AppendLoginLog(context.Context, *domain.LoginLog) error
	PageLoginLogs(context.Context, LoginLogFilter) (pagination.PageResult[domain.LoginLog], error)
}

// UserNicknameReader is implemented by an IAM bridge in the composition root.
// Both methods must honor the tenant carried by ctx.
type UserNicknameReader interface {
	Nicknames(context.Context, []int64) (map[int64]string, error)
	FindUserIDsByNickname(context.Context, string) ([]int64, error)
}
