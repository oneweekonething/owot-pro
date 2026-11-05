package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	infracache "oneweekonething.com/owot-pro/internal/infra/adapter/cache"
	infrahttp "oneweekonething.com/owot-pro/internal/infra/adapter/http"
	infrapersistence "oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	infraapp "oneweekonething.com/owot-pro/internal/infra/app"
	infradomain "oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/jobx"
	"oneweekonething.com/owot-pro/internal/platform/lockx"
)

type infraOperations struct {
	routes      *infrahttp.OperationsHandler
	requestLogs *httpx.RequestLogs
	runner      *infraapp.JobRunner
	seed        func(context.Context) error
}

func newInfraOperations(db *gorm.DB, client redis.UniversalClient, logger *slog.Logger) (infraOperations, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return infraOperations{}, err
	}
	schedule := jobx.NewCron(location)
	jobRepo := infrapersistence.NewJobRepository(db)
	logRepo := infrapersistence.NewAPILogRepository(db)

	defaults := []struct {
		handler, name, cron string
		clean               func(context.Context, time.Time) (int64, error)
	}{
		{"accessLogCleanJob", "清理 API 访问日志", "0 0 2 * * ?", logRepo.CleanAccess},
		{"errorLogCleanJob", "清理 API 错误日志", "0 10 2 * * ?", logRepo.CleanErrors},
		{"jobLogCleanJob", "清理任务执行日志", "0 20 2 * * ?", jobRepo.CleanLogs},
	}
	handlers := make(map[string]infraapp.JobHandler, len(defaults))
	for _, item := range defaults {
		handlers[item.handler] = func(ctx context.Context, _ string) (string, error) {
			count, err := item.clean(ctx, time.Now().UTC().AddDate(0, 0, -14))
			return fmt.Sprintf("清理 %d 条日志", count), err
		}
	}
	jobs := infraapp.NewJobService(jobRepo, jobRepo, dbx.NewTxManager(db), schedule, handlers)
	logs := infraapp.NewAPILogService(logRepo, logRepo)
	monitor := infraapp.NewRedisMonitorService(infracache.NewRedisInspector(client))
	writer := infrahttp.NewAPILogWriter(logs)
	return infraOperations{
		routes:      infrahttp.NewOperationsHandler(jobs, logs, monitor, logger),
		requestLogs: httpx.NewRequestLogs(writer, writer, logger),
		runner:      infraapp.NewJobRunner(jobRepo, jobRepo, schedule, lockx.NewLocker(client), handlers, logger),
		seed: func(ctx context.Context) error {
			for _, item := range defaults {
				_, err := jobs.Save(ctx, infradomain.Job{Name: item.name, HandlerName: item.handler, CronExpression: item.cron})
				if err != nil && !errors.Is(err, infradomain.ErrJobHandlerUsed) {
					return err
				}
			}
			return nil
		},
	}, nil
}
