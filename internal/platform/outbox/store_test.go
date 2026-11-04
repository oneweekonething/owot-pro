package outbox

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/testhelper"
)

type switchBroadcaster struct {
	fail bool
	next Broadcaster
}

func (b *switchBroadcaster) Broadcast(ctx context.Context, value Event) error {
	if b.fail {
		return errors.New("network unavailable")
	}
	return b.next.Broadcast(ctx, value)
}

func TestAtomicAppendAndRelayRecovery(t *testing.T) {
	db := testhelper.SQLite(t)
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	broadcast := &switchBroadcaster{fail: true, next: NewRedisBroadcaster(client)}
	store := NewStore(db, broadcast, slog.New(slog.NewTextHandler(io.Discard, nil)))
	tx := dbx.NewTxManager(db)
	ctx := context.Background()
	if err := store.Publish(ctx, "system.dict.invalidate", []byte(`{}`)); err == nil {
		t.Fatal("out-of-transaction append succeeded")
	}
	want := errors.New("rollback")
	if err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := store.Publish(ctx, "system.dict.invalidate", []byte(`{"dictType":"status"}`)); err != nil {
			return err
		}
		return want
	}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&eventPO{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rolled-back outbox event remains: %d, %v", count, err)
	}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error {
		return store.Publish(ctx, "system.dict.invalidate", []byte(`{"dictType":"status"}`))
	}); err != nil {
		t.Fatal(err)
	}
	var row eventPO
	if err := db.First(&row).Error; err != nil || row.Status != 0 || row.RetryCount != 1 {
		t.Fatalf("failed event not pending: %+v, %v", row, err)
	}
	// Advance the durable retry deadline rather than sleeping.
	if err := db.Model(&row).Update("next_retry_time", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	sub := client.Subscribe(ctx, "system.dict.invalidate")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	broadcast.fail = false
	if err := store.Flush(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&row, row.ID).Error; err != nil || row.Status != 1 {
		t.Fatalf("event not published: %+v, %v", row, err)
	}
	if version, err := client.Get(ctx, "system.dict.invalidate:version:0").Int(); err != nil || version != 1 {
		t.Fatalf("version = %d, %v", version, err)
	}
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if message, err := sub.ReceiveMessage(readCtx); err != nil || message.Payload == "" {
		t.Fatalf("broadcast missing: %v, %v", message, err)
	}
	if err := store.Flush(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if version, _ := client.Get(ctx, "system.dict.invalidate:version:0").Int(); version != 1 {
		t.Fatal("published event was delivered again")
	}
}
