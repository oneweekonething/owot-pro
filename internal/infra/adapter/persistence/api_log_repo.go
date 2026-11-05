package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type RequestLogPO struct {
	TraceID         string `gorm:"size:32;not null"`
	ApplicationName string `gorm:"size:64;not null"`
	RequestMethod   string `gorm:"size:16;not null"`
	RequestURL      string `gorm:"size:512;not null"`
	RequestParams   string `gorm:"type:text;not null"`
	UserIP          string `gorm:"size:64;not null"`
	UserAgent       string `gorm:"size:512;not null"`
	UserID          int64  `gorm:"not null"`
	UserType        int    `gorm:"not null"`
	ResultCode      int    `gorm:"not null"`
}

func (p RequestLogPO) request() domain.RequestLog {
	return domain.RequestLog(p)
}

type apiAccessLogPO struct {
	dbx.TenantBasePO
	RequestLogPO
	ResponseBody  string    `gorm:"type:text;not null"`
	ResultMsg     string    `gorm:"size:512;not null"`
	OperateModule string    `gorm:"size:64;not null"`
	OperateName   string    `gorm:"size:64;not null"`
	OperateType   int       `gorm:"not null"`
	BeginTime     time.Time `gorm:"not null;index"`
	EndTime       time.Time `gorm:"not null"`
	Duration      int64     `gorm:"not null"`
}

func (apiAccessLogPO) TableName() string { return "infra_api_access_log" }

func (p apiAccessLogPO) entry() domain.APIAccessLog {
	return domain.APIAccessLog{RequestLog: p.request(), ID: p.ID, ResponseBody: p.ResponseBody,
		ResultMsg: p.ResultMsg, OperateModule: p.OperateModule, OperateName: p.OperateName, OperateType: p.OperateType,
		BeginTime: p.BeginTime, EndTime: p.EndTime, CreateTime: p.CreateTime, Duration: p.Duration}
}

type apiErrorLogPO struct {
	dbx.TenantBasePO
	RequestLogPO
	ExceptionTime             time.Time `gorm:"not null;index"`
	ExceptionName             string    `gorm:"size:128;not null"`
	ExceptionMessage          string    `gorm:"size:512;not null"`
	ExceptionRootCauseMessage string    `gorm:"size:512;not null"`
	ExceptionStackTrace       string    `gorm:"type:text;not null"`
	ExceptionClassName        string    `gorm:"size:255;not null"`
	ExceptionFileName         string    `gorm:"size:255;not null"`
	ExceptionMethodName       string    `gorm:"size:255;not null"`
	ExceptionLineNumber       int       `gorm:"not null"`
	ProcessStatus             int       `gorm:"not null"`
	ProcessTime               *time.Time
	ProcessUserID             int64 `gorm:"not null"`
}

func (apiErrorLogPO) TableName() string { return "infra_api_error_log" }

func (p apiErrorLogPO) entry() domain.APIErrorLog {
	return domain.APIErrorLog{RequestLog: p.request(), ID: p.ID, ProcessStatus: p.ProcessStatus,
		ProcessUserID: p.ProcessUserID, ProcessTime: p.ProcessTime, CreateTime: p.CreateTime,
		ExceptionTime: p.ExceptionTime, ExceptionName: p.ExceptionName, ExceptionMessage: p.ExceptionMessage,
		ExceptionRootCauseMessage: p.ExceptionRootCauseMessage, ExceptionStackTrace: p.ExceptionStackTrace,
		ExceptionClassName: p.ExceptionClassName, ExceptionFileName: p.ExceptionFileName,
		ExceptionMethodName: p.ExceptionMethodName, ExceptionLineNumber: p.ExceptionLineNumber}
}

type APILogRepository struct{ db *gorm.DB }

func NewAPILogRepository(db *gorm.DB) *APILogRepository { return &APILogRepository{db: db} }

func (r *APILogRepository) CreateAccess(ctx context.Context, entry domain.APIAccessLog) error {
	p := apiAccessLogPO{RequestLogPO: RequestLogPO(entry.RequestLog), ResponseBody: entry.ResponseBody,
		ResultMsg: entry.ResultMsg, OperateModule: entry.OperateModule, OperateName: entry.OperateName,
		OperateType: entry.OperateType, BeginTime: entry.BeginTime, EndTime: entry.EndTime, Duration: entry.Duration}
	return dbx.FromContext(ctx, r.db).Create(&p).Error
}

func (r *APILogRepository) CreateError(ctx context.Context, entry domain.APIErrorLog) error {
	p := apiErrorLogPO{RequestLogPO: RequestLogPO(entry.RequestLog), ExceptionTime: entry.ExceptionTime, ExceptionName: entry.ExceptionName,
		ExceptionMessage: entry.ExceptionMessage, ExceptionRootCauseMessage: entry.ExceptionRootCauseMessage,
		ExceptionStackTrace: entry.ExceptionStackTrace, ExceptionClassName: entry.ExceptionClassName, ExceptionFileName: entry.ExceptionFileName,
		ExceptionMethodName: entry.ExceptionMethodName, ExceptionLineNumber: entry.ExceptionLineNumber, ProcessStatus: entry.ProcessStatus}
	return dbx.FromContext(ctx, r.db).Create(&p).Error
}

func (r *APILogRepository) GetAccess(ctx context.Context, id int64) (domain.APIAccessLog, error) {
	var p apiAccessLogPO
	err := dbx.FromContext(ctx, r.db).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrAccessLogNotFound
	}
	return p.entry(), err
}

func (r *APILogRepository) GetError(ctx context.Context, id int64) (domain.APIErrorLog, error) {
	var p apiErrorLogPO
	err := dbx.FromContext(ctx, r.db).Where("id = ?", id).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrErrorLogNotFound
	}
	return p.entry(), err
}

func (r *APILogRepository) ProcessError(ctx context.Context, id int64, status int, userID int64, at time.Time) error {
	result := dbx.FromContext(ctx, r.db).Model(&apiErrorLogPO{}).Where("id = ? AND process_status = ?", id, domain.ErrorUnhandled).
		Updates(map[string]any{"process_status": status, "process_user_id": userID, "process_time": at})
	if result.Error != nil || result.RowsAffected != 0 {
		return result.Error
	}
	if _, err := r.GetError(ctx, id); err != nil {
		return err
	}
	return domain.ErrErrorLogHandled
}

func (r *APILogRepository) PageAccess(ctx context.Context, filter app.AccessLogFilter) (pagination.PageResult[domain.APIAccessLog], error) {
	query := requestLogFilter(dbx.FromContext(ctx, r.db).Model(&apiAccessLogPO{}), filter.RequestLogFilter)
	query = timeBounds(query, "begin_time", filter.From, filter.To)
	if filter.Duration != nil {
		query = query.Where("duration >= ?", *filter.Duration)
	}
	if filter.ResultCode != nil {
		query = query.Where("result_code = ?", *filter.ResultCode)
	}
	return infraPage(query, filter.PageQuery, filter.Export, apiAccessLogPO.entry)
}

func (r *APILogRepository) PageError(ctx context.Context, filter app.ErrorLogFilter) (pagination.PageResult[domain.APIErrorLog], error) {
	query := requestLogFilter(dbx.FromContext(ctx, r.db).Model(&apiErrorLogPO{}), filter.RequestLogFilter)
	query = timeBounds(query, "exception_time", filter.From, filter.To)
	if filter.ProcessStatus != nil {
		query = query.Where("process_status = ?", *filter.ProcessStatus)
	}
	return infraPage(query, filter.PageQuery, filter.Export, apiErrorLogPO.entry)
}

func requestLogFilter(query *gorm.DB, filter app.RequestLogFilter) *gorm.DB {
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.UserType != nil {
		query = query.Where("user_type = ?", *filter.UserType)
	}
	if filter.ApplicationName != "" {
		query = query.Where("application_name = ?", filter.ApplicationName)
	}
	if filter.RequestURL != "" {
		query = query.Where("request_url LIKE ? ESCAPE '!'", containsPattern(filter.RequestURL))
	}
	return query
}

func (r *APILogRepository) CleanAccess(ctx context.Context, before time.Time) (int64, error) {
	return purgeLogs(dbx.FromContext(ctx, r.db), &apiAccessLogPO{}, before)
}

func (r *APILogRepository) CleanErrors(ctx context.Context, before time.Time) (int64, error) {
	return purgeLogs(dbx.FromContext(ctx, r.db), &apiErrorLogPO{}, before)
}

func purgeLogs(db *gorm.DB, model any, before time.Time) (int64, error) {
	var total int64
	for {
		const batchSize = 500
		var ids []int64
		err := db.Model(model).Unscoped().Where("create_time < ?", before).Order("create_time, id").Limit(batchSize).Pluck("id", &ids).Error
		if err != nil || len(ids) == 0 {
			return total, err
		}
		result := db.Unscoped().Where("id IN ?", ids).Delete(model)
		total += result.RowsAffected
		if result.Error != nil || len(ids) < batchSize {
			return total, result.Error
		}
	}
}

func MigrateOperations(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(&jobPO{}, &jobLogPO{}, &apiAccessLogPO{}, &apiErrorLogPO{})
}

func CheckOperationsSchema(db *gorm.DB) error {
	for _, table := range []string{"infra_job", "infra_job_log", "infra_api_access_log", "infra_api_error_log"} {
		if !db.Migrator().HasTable(table) {
			return errors.New(table + " table is missing; apply migrations/mysql/005_infra_operations.sql")
		}
	}
	return nil
}
