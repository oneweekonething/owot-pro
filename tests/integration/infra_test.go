package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type configBroadcaster struct {
	offline atomic.Bool
	next    outbox.Broadcaster
}

func (b *configBroadcaster) Broadcast(ctx context.Context, event outbox.Event) error {
	if b.offline.Load() {
		return errors.New("test: config broadcast interrupted")
	}
	return b.next.Broadcast(ctx, event)
}

type failedConfigOutbox struct{ err error }

func (f failedConfigOutbox) Publish(context.Context, string, []byte) error { return f.err }

func TestInfraConfigMySQLAndRedis(t *testing.T) {
	db, client := realSchema(t, "infra", "001_system.sql", "003_infra_config.sql")
	ctx, cancel := context.WithTimeout(ctxmeta.WithTenantID(context.Background(), 2), 10*time.Second)
	defer cancel()
	if err := persistence.CheckSchema(db); err != nil {
		t.Fatal(err)
	}
	repo := persistence.NewConfigRepository(db)
	tx := dbx.NewTxManager(db)
	broadcaster := &configBroadcaster{next: infracache.NewConfigBroadcaster(client, outbox.NewRedisBroadcaster(client))}
	store := outbox.NewStore(db, broadcaster, slog.New(slog.NewTextHandler(io.Discard, nil)))
	writer := app.NewConfigService(repo, tx, store, nil)
	cache, err := infracache.NewConfigCache(client)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	reader := app.NewConfigService(repo, tx, store, cache)
	builtinTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		key       string
		createdAt time.Time
	}{
		{"integration_builtin", builtinTime},
		{"integrationXbuiltin", builtinTime},
		{"integration_builtin_old", builtinTime.Add(-time.Hour)},
	} {
		if err := db.Exec("INSERT INTO infra_config (category, name, config_key, value, type, visible, create_time) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"test", "SQL 内置项", row.key, "built-in", domain.ConfigBuiltin, true, row.createdAt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if value, err := reader.VisibleValue(ctx, "integration_builtin"); err != nil || value != "built-in" {
		t.Fatalf("SQL-provisioned config was not visible: %q, %v", value, err)
	}
	builtinType := domain.ConfigBuiltin
	page, err := writer.Page(ctx, app.ConfigFilter{
		PageQuery: pagination.PageQuery{PageNo: 1, PageSize: 1},
		Key:       "integration_builtin", Type: &builtinType, From: &builtinTime, To: &builtinTime,
	})
	if err != nil || page.Total != 1 || len(page.List) != 1 || page.List[0].Key != "integration_builtin" {
		t.Fatalf("literal wildcard or inclusive time boundary failed: %+v, %v", page, err)
	}
	duplicate := domain.Config{Category: "test", Name: "重复配置", Key: "integration_builtin", Value: "duplicate"}
	if _, err := writer.Create(ctx, duplicate); !errors.Is(err, domain.ErrConfigKeyExists) {
		t.Fatalf("SQL-provisioned key was not unique: %v", err)
	}
	config := domain.Config{Category: "test", Name: "参数", Key: "integration.config", Value: "before", Visible: true}
	var wg sync.WaitGroup
	var successful atomic.Int64
	ids := make(chan int64, 2)
	for range 2 {
		wg.Go(func() {
			id, err := writer.Create(ctx, config)
			if err == nil {
				successful.Add(1)
				ids <- id
			} else if !errors.Is(err, domain.ErrConfigKeyExists) {
				t.Errorf("unexpected create error: %v", err)
			}
		})
	}
	wg.Wait()
	if successful.Load() != 1 {
		t.Fatalf("unique key allowed %d successful creates", successful.Load())
	}
	config.ID = <-ids
	if value, err := reader.Value(ctx, config.Key); err != nil || value != "before" {
		t.Fatalf("read initial config: %q, %v", value, err)
	}

	rejected := errors.New("test: reject outbox append")
	failingWriter := app.NewConfigService(repo, tx, failedConfigOutbox{err: rejected}, nil)
	config.Value = "rolled back"
	if err := failingWriter.Update(ctx, config); !errors.Is(err, rejected) {
		t.Fatalf("failed update returned %v", err)
	}
	if value, err := writer.Value(ctx, config.Key); err != nil || value != "before" {
		t.Fatalf("outbox failure did not roll back SQL: %q, %v", value, err)
	}

	versionKey := outbox.VersionKey(app.ConfigRefreshTopic, 0)
	version, err := client.Get(ctx, versionKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	broadcaster.offline.Store(true)
	config.Value, config.Visible = "after", false
	if err := writer.Update(ctx, config); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.VisibleValue(ctx, config.Key); !errors.Is(err, domain.ErrConfigHidden) {
		t.Fatalf("public visibility depended on cache delivery: %v", err)
	}
	if current, err := client.Get(ctx, versionKey).Result(); err != nil || current != version {
		t.Fatalf("offline broadcaster unexpectedly changed version to %q, %v", current, err)
	}
	broadcaster.offline.Store(false)
	relayCtx, stopRelay := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		store.Run(relayCtx)
	}()
	defer func() {
		stopRelay()
		<-done
	}()

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
replay:
	for {
		select {
		case <-deadline.C:
			t.Fatal("outbox did not broadcast config update")
		case <-ticker.C:
			current, err := client.Get(ctx, versionKey).Result()
			if err != nil {
				t.Fatal(err)
			}
			if current != version {
				break replay
			}
		}
	}
	if value, err := reader.Value(ctx, config.Key); err != nil || value != "after" {
		t.Fatalf("reader retained old value after replay: %q, %v", value, err)
	}

	if err := writer.Delete(ctx, []int64{config.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Create(ctx, config); err != nil {
		t.Fatalf("soft deletion did not release the key: %v", err)
	}
}
