package app

import (
	"context"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type ConfigFilter struct {
	pagination.PageQuery
	Name, Key string
	Type      *int
	From, To  *time.Time
}

type ConfigRepository interface {
	Get(context.Context, int64) (domain.Config, error)
	FindByKey(context.Context, string) (domain.Config, error)
	Save(context.Context, *domain.Config) error
	Delete(context.Context, int64) error
	Page(context.Context, ConfigFilter) (pagination.PageResult[domain.Config], error)
}

type TxRunner interface {
	WithinTx(context.Context, func(context.Context) error) error
}

type OutboxPublisher interface {
	Publish(context.Context, string, []byte) error
}

type ConfigValueCache interface {
	Load(context.Context, string, func(context.Context) (string, error)) (string, error)
}
