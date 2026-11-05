package cache

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

type ConfigCache struct {
	client redis.UniversalClient
	local  *ristretto.Cache[string, string]
}

func NewConfigCache(client redis.UniversalClient) (*ConfigCache, error) {
	local, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 10000, MaxCost: 1024, BufferItems: 64, IgnoreInternalCost: true,
	})
	if err != nil {
		return nil, err
	}
	return &ConfigCache{client: client, local: local}, nil
}

func (c *ConfigCache) Close() { c.local.Close() }

func (c *ConfigCache) Load(ctx context.Context, key string, load func(context.Context) (string, error)) (string, error) {
	if dbx.InTransaction(ctx) {
		return load(ctx)
	}
	version, err := c.client.Get(ctx, outbox.VersionKey(app.ConfigRefreshTopic, 0)).Result()
	if err != nil {
		return load(ctx)
	}
	cacheKey := version + ":" + key
	if value, ok := c.local.Get(cacheKey); ok {
		return value, nil
	}
	value, err := load(ctx)
	if err == nil {
		c.local.SetWithTTL(cacheKey, value, 1, 30*time.Second)
	}
	return value, err
}
