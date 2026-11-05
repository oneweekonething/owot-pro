package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infracache "oneweekonething.com/owot-pro/internal/infra/adapter/cache"
	"oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/jobx"
	"oneweekonething.com/owot-pro/internal/platform/lockx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type transientJobLogFailure struct {
	*persistence.JobRepository
	jobID  int64
	failed atomic.Bool
}

func (r *transientJobLogFailure) FinishLog(ctx context.Context, entry domain.JobLog) error {
	if entry.JobID == r.jobID && r.failed.CompareAndSwap(false, true) {
		return errors.New("test: transient job log update failure")
	}
	return r.JobRepository.FinishLog(ctx, entry)
}

func TestInfraJobsAcrossMySQLAndRedisWorkers(t *testing.T) {
	db, client := realSchema(t, "jobs", "005_infra_operations.sql", "005_infra_operations.sql")
	if err := persistence.CheckOperationsSchema(db); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	repo := persistence.NewJobRepository(db)
	schedule := jobx.NewCron(time.UTC)
	var manualCalls, autoCalls atomic.Int64
	manualName := fmt.Sprintf("testManual%d", time.Now().UnixNano())
	autoName := fmt.Sprintf("testAuto%d", time.Now().UnixNano())
	handlers := map[string]app.JobHandler{
		manualName: func(ctx context.Context, param string) (string, error) {
			if !ctxmeta.IsIgnoreTenant(ctx) || param != "manual" {
				t.Errorf("manual handler received wrong context or params: %s", param)
			}
			if manualCalls.Add(1) == 1 {
				return "", errors.New("retry this failure")
			}
			return "recovered", nil
		},
		autoName: func(ctx context.Context, _ string) (string, error) {
			if !ctxmeta.IsIgnoreTenant(ctx) {
				t.Error("scheduled handler did not ignore tenant")
			}
			autoCalls.Add(1)
			return "scheduled", nil
		},
	}
	service := app.NewJobService(repo, repo, dbx.NewTxManager(db), schedule, handlers)
	manual, err := service.Save(ctx, domain.Job{Name: "Paused and retried", HandlerName: manualName,
		HandlerParam: "manual", CronExpression: "0 0 2 * * ?", RetryCount: 1, RetryInterval: 30})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetStatus(ctx, manual, domain.JobPaused); err != nil {
		t.Fatal(err)
	}
	if err := service.Trigger(ctx, manual); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Add(3 * time.Second)
	expression := fmt.Sprintf("%d %d %d %d %d ? %d", when.Second(), when.Minute(), when.Hour(), when.Day(), when.Month(), when.Year())
	auto, err := service.Save(ctx, domain.Job{Name: "One scheduled run", HandlerName: autoName, CronExpression: expression})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	recorder := &transientJobLogFailure{JobRepository: repo, jobID: manual}
	for range 2 {
		runner := app.NewJobRunner(repo, recorder, schedule, lockx.NewLocker(client), handlers, log)
		workers.Go(func() { runner.Run(ctx) })
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		page, err := service.LogPage(ctx, app.JobLogFilter{PageQuery: pagination.PageQuery{PageSize: -1}})
		if err != nil {
			t.Fatal(err)
		}
		var succeeded int
		for _, entry := range page.List {
			if entry.Status == domain.JobSucceeded {
				succeeded++
			}
		}
		if succeeded >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("jobs did not finish: %+v", page.List)
		case <-ticker.C:
		}
	}

	if manualCalls.Load() != 2 || autoCalls.Load() != 1 {
		t.Fatalf("dispatch duplicated or skipped a run: manual=%d scheduled=%d", manualCalls.Load(), autoCalls.Load())
	}
	page, err := service.LogPage(ctx, app.JobLogFilter{JobID: manual})
	if err != nil || len(page.List) != 2 || page.List[0].ExecuteIndex != 2 || page.List[0].Result != "recovered" ||
		page.List[1].ExecuteIndex != 1 || page.List[1].Status != domain.JobFailed || page.List[1].EndTime == nil {
		t.Fatalf("manual retry log: %+v, %v", page, err)
	}
	job, err := service.Get(ctx, auto)
	if err != nil || job.NextRun != nil {
		t.Fatalf("one-time cron did not expire: %+v, %v", job, err)
	}
	job, err = service.Get(ctx, manual)
	if err != nil || job.Status != domain.JobPaused || job.ManualPending || job.NextRun != nil {
		t.Fatalf("manual run modified the paused schedule: %+v, %v", job, err)
	}
}

func TestInfraTenantLogTransitionsAndRedisMonitor(t *testing.T) {
	db, client := realSchema(t, "api_logs", "005_infra_operations.sql")
	repo := persistence.NewAPILogRepository(db)
	service := app.NewAPILogService(repo, repo)
	ctx := t.Context()
	tenantA := ctxmeta.WithTenantID(ctx, 10)
	tenantB := ctxmeta.WithTenantID(ctx, 11)
	at := time.Now().UTC()
	for _, tenant := range []context.Context{tenantA, tenantB} {
		request := domain.RequestLog{TraceID: "0123456789abcdef0123456789abcdef", UserID: 1, UserType: 1,
			RequestMethod: "GET", RequestURL: "/admin-api/infra/job/page", ApplicationName: "owot-pro", ResultCode: 500}
		if err := service.RecordError(tenant, domain.APIErrorLog{RequestLog: request, ExceptionTime: at}); err != nil {
			t.Fatal(err)
		}
		if err := service.RecordAccess(tenant, domain.APIAccessLog{RequestLog: request, BeginTime: at, EndTime: at}); err != nil {
			t.Fatal(err)
		}
	}
	access, err := service.AccessPage(tenantA, app.AccessLogFilter{})
	if err != nil || access.Total != 1 || access.List[0].ResultCode != 500 {
		t.Fatalf("access logs are not tenant scoped: %+v, %v", access, err)
	}
	page, err := service.ErrorPage(tenantA, app.ErrorLogFilter{})
	if err != nil || page.Total != 1 {
		t.Fatalf("error logs are not tenant scoped: %+v, %v", page, err)
	}
	id := page.List[0].ID
	if err := service.ProcessError(ctxmeta.WithLoginUser(tenantB, ctxmeta.LoginUser{ID: 20}), id, domain.ErrorIgnored); !errors.Is(err, domain.ErrErrorLogNotFound) {
		t.Fatalf("cross-tenant processing returned %v", err)
	}
	var changed atomic.Int64
	var wg sync.WaitGroup
	for _, state := range []int{domain.ErrorProcessed, domain.ErrorIgnored} {
		wg.Go(func() {
			actor := ctxmeta.WithLoginUser(tenantA, ctxmeta.LoginUser{ID: int64(state + 30)})
			err := service.ProcessError(actor, id, state)
			if err == nil {
				changed.Add(1)
			} else if !errors.Is(err, domain.ErrErrorLogHandled) {
				t.Errorf("unexpected concurrent process error: %v", err)
			}
		})
	}
	wg.Wait()
	handled, err := service.Error(tenantA, id)
	if changed.Load() != 1 || err != nil || handled.ProcessTime == nil || handled.ProcessUserID != int64(handled.ProcessStatus+30) {
		t.Fatalf("processing did not record one winner: %d %+v %v", changed.Load(), handled, err)
	}
	if _, err := service.ErrorPage(ctxmeta.WithIgnoreTenant(tenantA), app.ErrorLogFilter{}); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("unscoped log read returned %v", err)
	}
	if err := db.Exec("UPDATE infra_api_error_log SET create_time = ? WHERE tenant_id = ?", at.AddDate(0, 0, -16), 11).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := repo.CleanErrors(ctxmeta.WithIgnoreTenant(ctx), at.AddDate(0, 0, -14)); err != nil || count != 1 {
		t.Fatalf("global retention removed %d rows, err=%v", count, err)
	}
	if remaining, err := service.ErrorPage(tenantB, app.ErrorLogFilter{}); err != nil || remaining.Total != 0 {
		t.Fatalf("tenant B still has expired rows: %+v, %v", remaining, err)
	}
	if _, err := service.Error(tenantA, id); err != nil {
		t.Fatalf("retention removed a current row: %v", err)
	}
	key := fmt.Sprintf("owot:test:monitor:%d", time.Now().UnixNano())
	if err := client.Set(ctx, key, "1", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	monitor, err := app.NewRedisMonitorService(infracache.NewRedisInspector(client)).Monitor(ctx)
	if err != nil || monitor.Info["redis_version"] == "" || monitor.DBSize < 1 ||
		!slices.ContainsFunc(monitor.CommandStats, func(stat app.RedisCommandStat) bool { return stat.Command == "set" && stat.Calls > 0 }) {
		t.Fatalf("Redis monitor did not report server info, key count and SET stats: %+v, %v", monitor, err)
	}
}
