package httpx

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

type requestLogTenant struct{}

func (requestLogTenant) ValidateTenant(context.Context, int64) error { return nil }

type requestLogProbe struct {
	access  AccessLog
	fault   ErrorLog
	tenant  int64
	err     error
	started chan struct{}
}

func (p *requestLogProbe) WriteError(ctx context.Context, _ AccessLog, fault ErrorLog) error {
	p.fault, p.tenant = fault, ctxmeta.TenantID(ctx)
	return nil
}

func (p *requestLogProbe) WriteAccess(ctx context.Context, access AccessLog) error {
	close(p.started)
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		p.err = ctx.Err()
	case <-timer.C:
		p.access = access
	}
	return p.err
}

func TestRequestLogsPersistRecoveredPanicDuringShutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	probe := &requestLogProbe{started: make(chan struct{})}
	logs := NewRequestLogs(probe, probe, logger)
	router := NewRouter(Dependencies{Tenants: requestLogTenant{}, RequestLogs: logs, Logger: logger})
	router.Handle(http.MethodGet, "/admin-api/panic", Route{PermitAll: true}, func(*gin.Context) {
		panic("sensitive panic message")
	})
	req := httptest.NewRequest(http.MethodGet, "/admin-api/panic", nil)
	req.Header.Set("tenant-id", "7")
	w := httptest.NewRecorder()
	router.Engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":500`) {
		t.Fatalf("unexpected recovery response: %d %s", w.Code, w.Body.String())
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		logs.Run(ctx)
		close(done)
	}()
	select {
	case <-probe.started:
	case <-time.After(time.Second):
		t.Fatal("request log worker did not start")
	}
	cancel()
	<-done
	if probe.err != nil || probe.access.ResultCode != 500 || probe.tenant != 7 ||
		!strings.Contains(probe.fault.Stack, "TestRequestLogsPersistRecoveredPanic") ||
		strings.Contains(probe.fault.Stack, "sensitive panic message") {
		t.Fatalf("panic log was lost or retained sensitive data: %+v", probe)
	}
}
