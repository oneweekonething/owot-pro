// Package main 是 owot-pro 服务的启动入口（Composition Root）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	var (
		addr      = flag.String("addr", ":48080", "HTTP listen address")
		checkOnly = flag.Bool("check", false, "validate startup wiring and exit")
		migrate   = flag.Bool("migrate", false, "initialize backend tables (development only)")
		seed      = flag.Bool("seed", false, "initialize missing IAM, System and Infra defaults")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *addr, *checkOnly, *migrate, *seed); err != nil {
		fmt.Fprintf(os.Stderr, "owot-server exited with error: %v\n", err)
		os.Exit(1)
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /actuator/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"UP"},"msg":""}`))
	})
	return mux
}

func run(ctx context.Context, addr string, checkOnly, migrate, seed bool) error {
	application, err := newApplication(ctx, migrate, seed)
	if err != nil {
		return err
	}
	defer func() { _ = application.close() }()
	srv := &http.Server{
		Addr:              addr,
		Handler:           application.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if checkOnly {
		return nil
	}
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	logCtx, cancelLogs := context.WithCancel(context.WithoutCancel(ctx))
	var workers sync.WaitGroup
	if application.relay != nil {
		workers.Go(func() { application.relay.Run(workerCtx) })
	}
	if application.permissions != nil {
		workers.Go(func() { application.permissions.Run(workerCtx) })
	}
	if application.operations.runner != nil {
		workers.Go(func() { application.operations.runner.Run(workerCtx) })
		workers.Go(func() { application.operations.requestLogs.Run(logCtx) })
	}
	defer func() {
		cancelWorkers()
		cancelLogs()
		workers.Wait()
	}()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}
