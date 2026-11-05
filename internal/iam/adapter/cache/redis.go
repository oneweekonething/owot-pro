// Package cache implements authoritative Redis sessions and versioned permission snapshots.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

const topic = "iam.perm.invalidate"

func tokenKey(token string) string   { return "oauth2_access_token:" + token }
func versionKey(tenant int64) string { return fmt.Sprintf("iam:perm_ver:%d", tenant) }

type Tokens struct {
	client redis.UniversalClient
	logger *slog.Logger
}

func NewTokens(client redis.UniversalClient, logger *slog.Logger) *Tokens {
	return &Tokens{client: client, logger: logger}
}
func (t *Tokens) Get(ctx context.Context, token string) (domain.AccessToken, error) {
	if token == "" || len(token) > 255 {
		return domain.AccessToken{}, domain.ErrTokenInvalid
	}
	data, err := t.client.Get(ctx, tokenKey(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return domain.AccessToken{}, domain.ErrTokenInvalid
	}
	if err != nil {
		return domain.AccessToken{}, err
	}
	var value domain.AccessToken
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	if value.AccessToken != token {
		return value, domain.ErrTokenInvalid
	}
	return value, nil
}
func (t *Tokens) Set(ctx context.Context, value domain.AccessToken) error {
	ttl := time.Until(value.ExpiresTime)
	if ttl <= 0 {
		return domain.ErrTokenInvalid
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return t.client.Set(ctx, tokenKey(value.AccessToken), data, ttl).Err()
}
func (t *Tokens) Delete(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	keys := make([]string, len(tokens))
	for i, token := range tokens {
		keys[i] = tokenKey(token)
	}
	return t.client.Del(ctx, keys...).Err()
}

func (t *Tokens) DeleteAfterCommit(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	if !dbx.InTransaction(ctx) {
		return errors.New("iam: token eviction requires transaction")
	}
	dbx.AfterCommit(ctx, func(ctx context.Context) {
		cleanup, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		// SQL is already revoked and Verify checks it, even if Redis retains
		// the expired authorization until its TTL after an unavailable DEL.
		if err := t.Delete(cleanup, tokens); err != nil {
			t.logger.WarnContext(ctx, "evict revoked Redis sessions", "error", err)
		}
	})
	return nil
}

type permissionEntry struct {
	Versions [2]string
	Value    app.PermissionSnapshot
}
type Permissions struct {
	client redis.UniversalClient
	local  *ristretto.Cache[string, permissionEntry]
}

func NewPermissions(client redis.UniversalClient) (*Permissions, error) {
	local, err := ristretto.NewCache(&ristretto.Config[string, permissionEntry]{NumCounters: 100000, MaxCost: 32 << 20, BufferItems: 64})
	if err != nil {
		return nil, err
	}
	return &Permissions{client: client, local: local}, nil
}
func (p *Permissions) Close() { p.local.Close() }
func (p *Permissions) versions(ctx context.Context) ([2]string, error) {
	var result [2]string
	values, err := p.client.MGet(ctx, versionKey(0), versionKey(ctxmeta.TenantID(ctx))).Result()
	if err != nil {
		return result, err
	}
	for i, value := range values {
		if value != nil {
			result[i] = fmt.Sprint(value)
		}
	}
	return result, nil
}
func (p *Permissions) Load(ctx context.Context, userID int64, load func(context.Context) (app.PermissionSnapshot, error)) (app.PermissionSnapshot, error) {
	key := strconv.FormatInt(ctxmeta.TenantID(ctx), 10) + ":" + strconv.FormatInt(userID, 10)
	for range 3 {
		versions, err := p.versions(ctx)
		if err != nil {
			return app.PermissionSnapshot{}, err
		}
		if entry, ok := p.local.Get(key); ok && entry.Versions == versions {
			return entry.Value, nil
		}
		value, err := load(ctx)
		if err != nil {
			return value, err
		}
		after, err := p.versions(ctx)
		if err != nil {
			return app.PermissionSnapshot{}, err
		}
		if after != versions {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			return app.PermissionSnapshot{}, err
		}
		p.local.SetWithTTL(key, permissionEntry{Versions: versions, Value: value}, int64(len(data)), 30*time.Second)
		return value, nil
	}
	return app.PermissionSnapshot{}, errors.New("iam: permissions changed repeatedly during lookup")
}

// Run is owned by cmd/api. Version checks remain authoritative if Pub/Sub reconnects.
func (p *Permissions) Run(ctx context.Context) {
	sub := p.client.Subscribe(ctx, topic)
	defer func() { _ = sub.Close() }()
	channel := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-channel:
			if !ok {
				return
			}
			p.local.Clear()
		}
	}
}

type Broadcaster struct {
	client   redis.UniversalClient
	fallback *outbox.RedisBroadcaster
}

func NewBroadcaster(client redis.UniversalClient) *Broadcaster {
	return &Broadcaster{client: client, fallback: outbox.NewRedisBroadcaster(client)}
}
func (b *Broadcaster) Broadcast(ctx context.Context, event outbox.Event) error {
	if event.Topic != topic {
		return b.fallback.Broadcast(ctx, event)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	// One Redis command makes the epoch change and its notification indivisible.
	return b.client.Eval(ctx, `redis.call('INCR', KEYS[1]); redis.call('PUBLISH', ARGV[1], ARGV[2]); return 1`, []string{versionKey(event.TenantID)}, topic, string(data)).Err()
}
