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

type FileFilter struct {
	pagination.PageQuery
	Name, Path, Type string
	Storage          int
	From, To         *time.Time
}

type FileConfigReader interface {
	Config(context.Context, int64) (domain.FileConfig, error)
	Master(context.Context) (domain.FileConfig, error)
	PageConfigs(context.Context, FileFilter) (pagination.PageResult[domain.FileConfig], error)
}

type FileConfigWriter interface {
	SaveConfig(context.Context, *domain.FileConfig) error
	SwitchMaster(context.Context, int64) error
	DeleteConfig(context.Context, int64) error
}

type FileMetadataReader interface {
	ConfigInUse(context.Context, int64, time.Time) (bool, error)
	FindFile(context.Context, int64, string) (domain.FileObject, error)
	FilesByID(context.Context, []int64) ([]domain.FileObject, error)
	PageFiles(context.Context, FileFilter) (pagination.PageResult[domain.FileObject], error)
}

type FileMetadataWriter interface {
	CreateFile(context.Context, *domain.FileObject) error
	CompleteFile(context.Context, domain.FileObject) error
	MarkFileForDeletion(context.Context, int64) error
	FinishFileDeletion(context.Context, int64) error
	PrunePending(context.Context, int64, time.Time) error
}

type FileStorageEngine interface {
	Put(ctx context.Context, path string, data []byte, contentType string) error
	Get(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
}

type DirectFileStorage interface {
	PresignUpload(ctx context.Context, path string, ttl time.Duration) (string, error)
	ReadUpload(ctx context.Context, path string) ([]byte, error)
	DeleteUpload(ctx context.Context, path string) error
}

type FileStorageFactory interface {
	Engine(context.Context, domain.FileConfig) (FileStorageEngine, error)
}
