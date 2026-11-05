package cache

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

func TestPermissionEpochsAndBoundedTTLWithoutSubscriber(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer func() { _ = client.Close() }()
	cache, err := NewPermissions(client)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	ctx := ctxmeta.WithTenantID(context.Background(), 42)
	loads := 0
	loader := func(context.Context) (app.PermissionSnapshot, error) {
		loads++
		return app.PermissionSnapshot{User: domain.AdminUser{ID: int64(loads)}}, nil
	}
	value, err := cache.Load(ctx, 7, loader)
	if err != nil || value.User.ID != 1 {
		t.Fatal(value, err)
	}
	cache.local.Wait()
	value, err = cache.Load(ctx, 7, loader)
	if err != nil || value.User.ID != 1 || loads != 1 {
		t.Fatal("cache miss", loads, err)
	}
	if ttl, ok := cache.local.GetTTL("42:7"); !ok || ttl <= 0 || ttl > 30*time.Second {
		t.Fatal("unbounded TTL", ttl, ok)
	}
	b := NewBroadcaster(client)
	if err := b.Broadcast(ctx, outbox.Event{Topic: topic, TenantID: 42, Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	value, err = cache.Load(ctx, 7, loader)
	if err != nil || value.User.ID != 2 {
		t.Fatal("missed tenant broadcast retained permissions", value, err)
	}
	cache.local.Wait()
	if err := b.Broadcast(ctx, outbox.Event{Topic: topic, TenantID: 0, Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	value, err = cache.Load(ctx, 7, loader)
	if err != nil || value.User.ID != 3 {
		t.Fatal("global menu broadcast ignored", value, err)
	}
	cache.local.Wait()
	mini.SetError("unavailable")
	if _, err := cache.Load(ctx, 7, loader); err == nil {
		t.Fatal("stale authorization served during Redis failure")
	}
}
func TestTokenTTLAndNoFallbackAfterRevocation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer func() { _ = client.Close() }()
	tokens := NewTokens(client, slog.Default())
	ctx := context.Background()
	value := domain.AccessToken{AccessToken: "token", UserID: 1, TenantID: 2, UserType: 1, ExpiresTime: time.Now().Add(time.Minute)}
	if err := tokens.Set(ctx, value); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Get(ctx, "token"); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Delete(ctx, []string{"token"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Get(ctx, "token"); !errors.Is(err, domain.ErrTokenInvalid) {
		t.Fatal(err)
	}
	if err := tokens.Set(ctx, value); err != nil {
		t.Fatal(err)
	}
	mini.FastForward(2 * time.Minute)
	if _, err := tokens.Get(ctx, "token"); !errors.Is(err, domain.ErrTokenInvalid) {
		t.Fatal(err)
	}
}
