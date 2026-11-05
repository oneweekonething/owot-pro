package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RedisInspector struct{ client redis.UniversalClient }

func NewRedisInspector(client redis.UniversalClient) *RedisInspector {
	return &RedisInspector{client: client}
}

func (r *RedisInspector) Snapshot(ctx context.Context) (string, string, int64, error) {
	pipe := r.client.Pipeline()
	info := pipe.Info(ctx)
	stats := pipe.Info(ctx, "commandstats")
	size := pipe.DBSize(ctx)
	_, err := pipe.Exec(ctx)
	return info.Val(), stats.Val(), size.Val(), err
}
