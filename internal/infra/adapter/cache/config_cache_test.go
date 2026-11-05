package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

func TestConfigCacheVersionAndRedisFailure(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, err := NewConfigCache(client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	ctx := context.Background()
	epoch := outbox.VersionKey(app.ConfigRefreshTopic, 0)
	if err := mini.Set(epoch, "1"); err != nil {
		t.Fatal(err)
	}
	durable := "first"
	load := func(context.Context) (string, error) { return durable, nil }
	read := func(want string) {
		t.Helper()
		got, err := cache.Load(ctx, "sample", load)
		if err != nil || got != want {
			t.Fatalf("read = %q, %v; want %q", got, err, want)
		}
	}
	read("first")
	cache.local.Wait()
	durable = "updated"
	read("first")
	if err := mini.Set(epoch, "2"); err != nil {
		t.Fatal(err)
	}
	read("updated")
	cache.local.Wait()
	durable = "latest"
	mini.SetError("Redis temporarily unavailable")
	read("latest")
}

func TestConfigCacheAfterRedisVersionKeyLoss(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache, err := NewConfigCache(client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cache.Close)
	ctx := context.Background()
	broadcaster := NewConfigBroadcaster(client, outbox.NewRedisBroadcaster(client))
	event := outbox.Event{Topic: app.ConfigRefreshTopic}
	if err := broadcaster.Broadcast(ctx, event); err != nil {
		t.Fatal(err)
	}
	value := "before"
	load := func(context.Context) (string, error) { return value, nil }
	if _, err := cache.Load(ctx, "sample", load); err != nil {
		t.Fatal(err)
	}
	cache.local.Wait()
	mini.Del(outbox.VersionKey(app.ConfigRefreshTopic, 0))
	value = "after"
	if err := broadcaster.Broadcast(ctx, event); err != nil {
		t.Fatal(err)
	}
	if got, err := cache.Load(ctx, "sample", load); err != nil || got != "after" {
		t.Fatalf("reused a cache entry from before Redis key loss: %q, %v", got, err)
	}
}
