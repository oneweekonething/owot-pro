package app

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

type JobRunner struct {
	queue    JobQueue
	logs     JobLogRecorder
	schedule JobSchedule
	locker   JobLocker
	handlers map[string]JobHandler
	logger   *slog.Logger
}

func NewJobRunner(queue JobQueue, logs JobLogRecorder, schedule JobSchedule, locker JobLocker, handlers map[string]JobHandler, logger *slog.Logger) *JobRunner {
	return &JobRunner{queue: queue, logs: logs, schedule: schedule, locker: locker, handlers: maps.Clone(handlers), logger: logger}
}

func (r *JobRunner) Run(ctx context.Context) {
	ctx = ctxmeta.WithIgnoreTenant(ctx)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	slots := make(chan struct{}, 4)
	var workers sync.WaitGroup
	var active sync.Map
	defer workers.Wait()
	for {
		if ctx.Err() != nil {
			return
		}
		if len(slots) < cap(slots) {
			jobs, err := r.queue.Due(ctx, time.Now().UTC(), 32)
			if err != nil && ctx.Err() == nil {
				r.logger.ErrorContext(ctx, "find due jobs", "error", err)
			}
			for _, job := range jobs {
				if len(slots) == cap(slots) || ctx.Err() != nil {
					break
				}
				if _, exists := active.LoadOrStore(job.ID, struct{}{}); exists {
					continue
				}
				slots <- struct{}{}
				workers.Go(func() {
					defer func() {
						active.Delete(job.ID)
						<-slots
					}()
					if err := r.run(ctx, job); err != nil && ctx.Err() == nil {
						r.logger.ErrorContext(ctx, "dispatch job", "job_id", job.ID, "error", err)
					}
				})
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *JobRunner) run(ctx context.Context, job domain.Job) error {
	handler := r.handlers[job.HandlerName]
	if handler == nil {
		return domain.ErrJobHandlerMissing
	}
	if err := job.Validate(); err != nil {
		return err
	}
	interval := time.Duration(job.RetryInterval) * time.Millisecond
	ttl := time.Duration(job.RetryCount+1)*(job.Timeout()+10*time.Second) + time.Duration(job.RetryCount)*interval + time.Minute
	release, err := r.locker.Acquire(ctx, "owot:infra:job:"+job.HandlerName, ttl)
	if err != nil || release == nil {
		return err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := release(releaseCtx); err != nil {
			r.logger.ErrorContext(releaseCtx, "release job lease", "job_id", job.ID, "error", err)
		}
	}()
	begin := time.Now().UTC()
	nextRun := job.NextRun
	if !job.ManualPending {
		next, err := r.schedule.Next(job.CronExpression, begin)
		if err != nil {
			return err
		}
		nextRun = nil
		if !next.IsZero() {
			nextRun = &next
		}
	}
	entry := domain.JobLog{JobID: job.ID, HandlerName: job.HandlerName, HandlerParam: job.HandlerParam,
		CronExpression: job.CronExpression, ExecuteIndex: 1, Status: domain.JobRunning, BeginTime: begin}
	claimed, err := r.queue.Claim(ctx, job, nextRun, &entry)
	if err != nil || !claimed {
		return err
	}
	for {
		result, runErr := executeJob(ctx, handler, job)
		end := time.Now().UTC()
		entry.EndTime, entry.Duration, entry.Status = &end, end.Sub(entry.BeginTime).Milliseconds(), domain.JobSucceeded
		if runErr != nil {
			entry.Status, result = domain.JobFailed, runErr.Error()
		}
		runes := []rune(strings.ToValidUTF8(result, "�"))
		if len(runes) > 4096 {
			runes = runes[:4096]
		}
		entry.Result = string(runes)
		if err := r.finishLog(ctx, entry); err != nil {
			r.logger.ErrorContext(ctx, "finish job log", "job_id", entry.JobID, "log_id", entry.ID, "error", err)
		}
		if runErr == nil || entry.ExecuteIndex > job.RetryCount {
			return nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		entry = domain.JobLog{JobID: job.ID, HandlerName: job.HandlerName, HandlerParam: job.HandlerParam,
			CronExpression: job.CronExpression, ExecuteIndex: entry.ExecuteIndex + 1, Status: domain.JobRunning, BeginTime: time.Now().UTC()}
		if err := r.logs.StartLog(ctx, &entry); err != nil {
			return err
		}
	}
}

func (r *JobRunner) finishLog(ctx context.Context, entry domain.JobLog) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := r.logs.FinishLog(ctx, entry); err != nil {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return err
		case <-timer.C:
			return r.logs.FinishLog(ctx, entry)
		}
	}
	return nil
}

func executeJob(ctx context.Context, handler JobHandler, job domain.Job) (result string, err error) {
	ctx, cancel := context.WithTimeout(ctx, job.Timeout())
	defer cancel()
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("job panic: %v", value)
		}
	}()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	result, err = handler(ctx, job.HandlerParam)
	if err == nil {
		err = ctx.Err()
	}
	return result, err
}
