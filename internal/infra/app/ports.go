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

type JobFilter struct {
	pagination.PageQuery
	Name, HandlerName string
	Status            *int
}

type JobLogFilter struct {
	pagination.PageQuery
	Export                             bool
	JobID                              int64
	HandlerName                        string
	Status                             *int
	BeginFrom, BeginTo, EndFrom, EndTo *time.Time
}

type JobRepository interface {
	Get(context.Context, int64) (domain.Job, error)
	Save(context.Context, *domain.Job) error
	Delete(context.Context, int64) error
	Page(context.Context, JobFilter) (pagination.PageResult[domain.Job], error)
}

type JobLogReader interface {
	GetLog(context.Context, int64) (domain.JobLog, error)
	PageLogs(context.Context, JobLogFilter) (pagination.PageResult[domain.JobLog], error)
}

type JobQueue interface {
	Due(context.Context, time.Time, int) ([]domain.Job, error)
	Claim(context.Context, domain.Job, *time.Time, *domain.JobLog) (bool, error)
}

type JobLogRecorder interface {
	StartLog(context.Context, *domain.JobLog) error
	FinishLog(context.Context, domain.JobLog) error
}

type JobSchedule interface {
	Next(string, time.Time) (time.Time, error)
}

type JobLocker interface {
	Acquire(context.Context, string, time.Duration) (func(context.Context) error, error)
}

type RequestLogFilter struct {
	pagination.PageQuery
	Export                      bool
	UserID                      *int64
	UserType                    *int
	ApplicationName, RequestURL string
	From, To                    *time.Time
}

type AccessLogFilter struct {
	RequestLogFilter
	Duration   *int64
	ResultCode *int
}

type ErrorLogFilter struct {
	RequestLogFilter
	ProcessStatus *int
}

type AccessLogRepository interface {
	CreateAccess(context.Context, domain.APIAccessLog) error
	GetAccess(context.Context, int64) (domain.APIAccessLog, error)
	PageAccess(context.Context, AccessLogFilter) (pagination.PageResult[domain.APIAccessLog], error)
}

type ErrorLogRepository interface {
	CreateError(context.Context, domain.APIErrorLog) error
	GetError(context.Context, int64) (domain.APIErrorLog, error)
	PageError(context.Context, ErrorLogFilter) (pagination.PageResult[domain.APIErrorLog], error)
	ProcessError(context.Context, int64, int, int64, time.Time) error
}

type RedisInspector interface {
	Snapshot(context.Context) (info, commandStats string, dbSize int64, err error)
}
