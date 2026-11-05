package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type jobPO struct {
	dbx.BasePO
	Name           string     `gorm:"size:64;not null"`
	HandlerName    string     `gorm:"size:64;not null"`
	ActiveHandler  *string    `gorm:"type:VARCHAR(64) GENERATED ALWAYS AS (CASE WHEN deleted = 0 THEN handler_name ELSE NULL END) VIRTUAL;->;uniqueIndex:uk_infra_job_active_handler"`
	HandlerParam   string     `gorm:"size:255;not null"`
	CronExpression string     `gorm:"size:128;not null"`
	Status         int        `gorm:"not null;index:idx_infra_job_due,priority:1"`
	RetryCount     int        `gorm:"not null"`
	RetryInterval  int        `gorm:"not null"`
	MonitorTimeout int        `gorm:"not null"`
	NextRun        *time.Time `gorm:"index:idx_infra_job_due,priority:2"`
	ManualPending  bool       `gorm:"not null;index:idx_infra_job_manual"`
	Revision       int64      `gorm:"not null"`
}

func (jobPO) TableName() string { return "infra_job" }

func (p jobPO) job() domain.Job {
	return domain.Job{ID: p.ID, Name: p.Name, HandlerName: p.HandlerName, HandlerParam: p.HandlerParam,
		CronExpression: p.CronExpression, Status: p.Status, RetryCount: p.RetryCount, RetryInterval: p.RetryInterval,
		MonitorTimeout: p.MonitorTimeout, NextRun: p.NextRun, ManualPending: p.ManualPending, Revision: p.Revision, CreateTime: p.CreateTime}
}

type jobLogPO struct {
	dbx.BasePO
	JobID          int64     `gorm:"not null;index"`
	HandlerName    string    `gorm:"size:64;not null"`
	HandlerParam   string    `gorm:"size:255;not null"`
	CronExpression string    `gorm:"size:128;not null"`
	ExecuteIndex   int       `gorm:"not null"`
	Status         int       `gorm:"not null"`
	BeginTime      time.Time `gorm:"not null;index"`
	EndTime        *time.Time
	Duration       int64  `gorm:"not null"`
	Result         string `gorm:"type:text;not null"`
}

func (jobLogPO) TableName() string { return "infra_job_log" }

func (p jobLogPO) entry() domain.JobLog {
	return domain.JobLog{ID: p.ID, JobID: p.JobID, HandlerName: p.HandlerName, HandlerParam: p.HandlerParam,
		CronExpression: p.CronExpression, ExecuteIndex: p.ExecuteIndex, Status: p.Status, BeginTime: p.BeginTime,
		EndTime: p.EndTime, CreateTime: p.CreateTime, Duration: p.Duration, Result: p.Result}
}

type JobRepository struct{ db *gorm.DB }

func NewJobRepository(db *gorm.DB) *JobRepository { return &JobRepository{db: db} }

func (r *JobRepository) Get(ctx context.Context, id int64) (domain.Job, error) {
	var record jobPO
	query := dbx.FromContext(ctx, r.db)
	if dbx.InTransaction(ctx) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrJobNotFound
	}
	return record.job(), err
}

func (r *JobRepository) Save(ctx context.Context, job *domain.Job) error {
	db := dbx.FromContext(ctx, r.db)
	if job.ID == 0 {
		p := jobPO{Name: job.Name, HandlerName: job.HandlerName, HandlerParam: job.HandlerParam, CronExpression: job.CronExpression,
			Status: job.Status, RetryCount: job.RetryCount, RetryInterval: job.RetryInterval, MonitorTimeout: job.MonitorTimeout, NextRun: job.NextRun}
		err := db.Create(&p).Error
		if err == nil {
			*job = p.job()
		}
		return jobSaveError(err)
	}
	result := db.Model(&jobPO{}).Where("id = ? AND revision = ?", job.ID, job.Revision).Updates(map[string]any{
		"name": job.Name, "handler_name": job.HandlerName, "handler_param": job.HandlerParam, "cron_expression": job.CronExpression,
		"status": job.Status, "retry_count": job.RetryCount, "retry_interval": job.RetryInterval, "monitor_timeout": job.MonitorTimeout,
		"next_run": job.NextRun, "manual_pending": job.ManualPending, "revision": gorm.Expr("revision + 1"),
	})
	if result.Error == nil && result.RowsAffected == 0 {
		return domain.ErrJobChanged
	}
	return jobSaveError(result.Error)
}

func jobSaveError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrJobHandlerUsed
	}
	return err
}

func (r *JobRepository) Delete(ctx context.Context, id int64) error {
	result := dbx.FromContext(ctx, r.db).Where("id = ?", id).Delete(&jobPO{})
	if result.Error == nil && result.RowsAffected == 0 {
		return domain.ErrJobNotFound
	}
	return result.Error
}

func (r *JobRepository) Page(ctx context.Context, filter app.JobFilter) (pagination.PageResult[domain.Job], error) {
	query := dbx.FromContext(ctx, r.db).Model(&jobPO{})
	if filter.Name != "" {
		query = query.Where("name LIKE ? ESCAPE '!'", containsPattern(filter.Name))
	}
	if filter.HandlerName != "" {
		query = query.Where("handler_name LIKE ? ESCAPE '!'", containsPattern(filter.HandlerName))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	return infraPage(query, filter.PageQuery, false, jobPO.job)
}

func (r *JobRepository) Due(ctx context.Context, now time.Time, limit int) ([]domain.Job, error) {
	var records []jobPO
	err := dbx.FromContext(ctx, r.db).Where("(status = ? AND next_run <= ?) OR manual_pending = ?", domain.JobEnabled, now, true).
		Order("manual_pending DESC, next_run, id").Limit(limit).Find(&records).Error
	jobs := make([]domain.Job, 0, len(records))
	for _, record := range records {
		jobs = append(jobs, record.job())
	}
	return jobs, err
}

func (r *JobRepository) Claim(ctx context.Context, job domain.Job, next *time.Time, entry *domain.JobLog) (bool, error) {
	var claimed bool
	err := dbx.NewTxManager(r.db).WithinTx(ctx, func(ctx context.Context) error {
		result := dbx.FromContext(ctx, r.db).Model(&jobPO{}).Where("id = ? AND revision = ?", job.ID, job.Revision).
			UpdateColumns(map[string]any{"next_run": next, "manual_pending": false, "revision": gorm.Expr("revision + 1")})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		claimed = true
		return r.StartLog(ctx, entry)
	})
	return claimed && err == nil, err
}

func (r *JobRepository) StartLog(ctx context.Context, entry *domain.JobLog) error {
	p := jobLogPO{JobID: entry.JobID, HandlerName: entry.HandlerName, HandlerParam: entry.HandlerParam,
		CronExpression: entry.CronExpression, ExecuteIndex: entry.ExecuteIndex, Status: entry.Status, BeginTime: entry.BeginTime}
	if err := dbx.FromContext(ctx, r.db).Create(&p).Error; err != nil {
		return err
	}
	entry.ID, entry.CreateTime = p.ID, p.CreateTime
	return nil
}

func (r *JobRepository) FinishLog(ctx context.Context, entry domain.JobLog) error {
	return dbx.FromContext(ctx, r.db).Model(&jobLogPO{}).Where("id = ?", entry.ID).Updates(map[string]any{
		"status": entry.Status, "result": entry.Result, "end_time": entry.EndTime, "duration": entry.Duration,
	}).Error
}

func (r *JobRepository) GetLog(ctx context.Context, id int64) (domain.JobLog, error) {
	var record jobLogPO
	err := dbx.FromContext(ctx, r.db).Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = domain.ErrJobLogNotFound
	}
	return record.entry(), err
}

func (r *JobRepository) PageLogs(ctx context.Context, filter app.JobLogFilter) (pagination.PageResult[domain.JobLog], error) {
	query := dbx.FromContext(ctx, r.db).Model(&jobLogPO{})
	if filter.JobID != 0 {
		query = query.Where("job_id = ?", filter.JobID)
	}
	if filter.HandlerName != "" {
		query = query.Where("handler_name LIKE ? ESCAPE '!'", containsPattern(filter.HandlerName))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	query = timeBounds(query, "begin_time", filter.BeginFrom, filter.BeginTo)
	query = timeBounds(query, "end_time", filter.EndFrom, filter.EndTo)
	return infraPage(query, filter.PageQuery, filter.Export, jobLogPO.entry)
}

func (r *JobRepository) CleanLogs(ctx context.Context, before time.Time) (int64, error) {
	return purgeLogs(dbx.FromContext(ctx, r.db), &jobLogPO{}, before)
}

func infraPage[PO, T any](query *gorm.DB, page pagination.PageQuery, export bool, convert func(PO) T) (pagination.PageResult[T], error) {
	var result pagination.PageResult[T]
	if export {
		query = query.Limit(domain.MaxLogExportRows + 1)
	} else if page.PageSize != -1 {
		if err := query.Count(&result.Total).Error; err != nil {
			return result, err
		}
		query = query.Offset(page.Offset()).Limit(page.PageSize)
	}
	var records []PO
	if err := query.Order("id DESC").Find(&records).Error; err != nil {
		return result, err
	}
	if export && len(records) > domain.MaxLogExportRows {
		return result, domain.ErrLogExportLimit
	}
	if export || page.PageSize == -1 {
		result.Total = int64(len(records))
	}
	result.List = make([]T, 0, len(records))
	for _, p := range records {
		result.List = append(result.List, convert(p))
	}
	return result, nil
}

func timeBounds(query *gorm.DB, column string, from, to *time.Time) *gorm.DB {
	if from != nil {
		query = query.Where(column+" >= ?", *from)
	}
	if to != nil {
		query = query.Where(column+" <= ?", *to)
	}
	return query
}
