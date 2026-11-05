package app

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type JobHandler func(context.Context, string) (string, error)

type JobService struct {
	repo     JobRepository
	logs     JobLogReader
	tx       TxRunner
	schedule JobSchedule
	handlers map[string]JobHandler
}

func NewJobService(repo JobRepository, logs JobLogReader, tx TxRunner, schedule JobSchedule, handlers map[string]JobHandler) *JobService {
	return &JobService{repo: repo, logs: logs, tx: tx, schedule: schedule, handlers: maps.Clone(handlers)}
}

func (s *JobService) validate(job domain.Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if s.handlers[job.HandlerName] == nil {
		return domain.ErrJobHandlerMissing
	}
	return nil
}

func (s *JobService) Save(ctx context.Context, job domain.Job) (int64, error) {
	job.Name = strings.TrimSpace(job.Name)
	job.HandlerName = strings.TrimSpace(job.HandlerName)
	job.CronExpression = strings.TrimSpace(job.CronExpression)
	if job.ID < 0 {
		return 0, domain.ErrInvalidArgument
	}
	if err := s.validate(job); err != nil {
		return 0, err
	}
	if job.ID != 0 {
		err := s.mutate(ctx, job.ID, func(current *domain.Job) error {
			if current.HandlerName != job.HandlerName {
				return domain.ErrInvalidArgument
			}
			job.Status, job.Revision, job.ManualPending = current.Status, current.Revision, current.ManualPending
			job.CreateTime, job.NextRun = current.CreateTime, current.NextRun
			if job.CronExpression != current.CronExpression {
				if err := s.setNextRun(&job, time.Now()); err != nil {
					return err
				}
			}
			*current = job
			return nil
		})
		return job.ID, err
	}
	job.Status, job.ManualPending = domain.JobEnabled, false
	if err := s.setNextRun(&job, time.Now()); err != nil {
		return 0, err
	}
	if err := s.repo.Save(ctx, &job); err != nil {
		return 0, fmt.Errorf("save job: %w", err)
	}
	return job.ID, nil
}

func (s *JobService) mutate(ctx context.Context, id int64, change func(*domain.Job) error) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		job, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		updated := job
		if err := change(&updated); err != nil {
			return err
		}
		if updated == job {
			return nil
		}
		if err := s.repo.Save(ctx, &updated); err != nil {
			return fmt.Errorf("update job %d: %w", id, err)
		}
		return nil
	})
}

func (s *JobService) setNextRun(job *domain.Job, now time.Time) error {
	next, err := s.schedule.Next(job.CronExpression, now)
	if err != nil || next.IsZero() {
		return domain.ErrJobCron
	}
	job.NextRun = nil
	if job.Status == domain.JobEnabled {
		job.NextRun = &next
	}
	return nil
}

func (s *JobService) SetStatus(ctx context.Context, id int64, status int) error {
	if status != domain.JobEnabled && status != domain.JobPaused {
		return domain.ErrInvalidArgument
	}
	return s.mutate(ctx, id, func(job *domain.Job) error {
		if job.Status == status {
			return nil
		}
		job.Status, job.NextRun = status, nil
		if status == domain.JobEnabled {
			if err := s.validate(*job); err != nil {
				return err
			}
			return s.setNextRun(job, time.Now())
		}
		return nil
	})
}

func (s *JobService) Trigger(ctx context.Context, id int64) error {
	return s.mutate(ctx, id, func(job *domain.Job) error {
		if s.handlers[job.HandlerName] == nil {
			return domain.ErrJobHandlerMissing
		}
		job.ManualPending = true
		return nil
	})
}

func (s *JobService) Delete(ctx context.Context, ids []int64) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, id := range ids {
			if err := s.repo.Delete(ctx, id); err != nil {
				return fmt.Errorf("delete job %d: %w", id, err)
			}
		}
		return nil
	})
}

func (s *JobService) Get(ctx context.Context, id int64) (domain.Job, error) {
	result, err := s.repo.Get(ctx, id)
	if err != nil {
		return result, fmt.Errorf("get job %d: %w", id, err)
	}
	return result, nil
}

func (s *JobService) Page(ctx context.Context, filter JobFilter) (pagination.PageResult[domain.Job], error) {
	filter.PageQuery = filter.Normalize()
	result, err := s.repo.Page(ctx, filter)
	if err != nil {
		return result, fmt.Errorf("page jobs: %w", err)
	}
	return result, nil
}

func (s *JobService) NextTimes(ctx context.Context, id int64, count int) ([]int64, error) {
	job, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if count < 1 || count > 100 {
		return nil, domain.ErrInvalidArgument
	}
	times := make([]int64, 0, count)
	next := time.Now()
	for range count {
		next, err = s.schedule.Next(job.CronExpression, next)
		if err != nil {
			return nil, domain.ErrJobCron
		}
		if next.IsZero() {
			break
		}
		times = append(times, next.UnixMilli())
	}
	return times, nil
}

// Sync validates the persisted schedules. Dispatchers read the same SQL state
// directly, so there is no secondary scheduler state to rebuild.
func (s *JobService) Sync(ctx context.Context) error {
	page, err := s.Page(ctx, JobFilter{PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		return err
	}
	for _, job := range page.List {
		if err := s.validate(job); err != nil {
			return err
		}
		if _, err := s.schedule.Next(job.CronExpression, time.Now()); err != nil {
			return domain.ErrJobCron
		}
	}
	return nil
}

func (s *JobService) Log(ctx context.Context, id int64) (domain.JobLog, error) {
	result, err := s.logs.GetLog(ctx, id)
	if err != nil {
		return result, fmt.Errorf("get job log %d: %w", id, err)
	}
	return result, nil
}

func (s *JobService) LogPage(ctx context.Context, filter JobLogFilter) (pagination.PageResult[domain.JobLog], error) {
	filter.PageQuery = filter.Normalize()
	result, err := s.logs.PageLogs(ctx, filter)
	if err != nil {
		return result, fmt.Errorf("page job logs: %w", err)
	}
	return result, nil
}
