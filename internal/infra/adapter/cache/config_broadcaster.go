package cache

import (
	"context"
	"crypto/rand"
	"encoding/json"

	"github.com/redis/go-redis/v9"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

// ConfigBroadcaster assigns a new version to each delivery, even if Redis
// lost its old version key or the outbox is replaying the same event.
type ConfigBroadcaster struct {
	client redis.UniversalClient
	next   outbox.Broadcaster
}

func NewConfigBroadcaster(client redis.UniversalClient, next outbox.Broadcaster) *ConfigBroadcaster {
	return &ConfigBroadcaster{client: client, next: next}
}

func (b *ConfigBroadcaster) Broadcast(ctx context.Context, event outbox.Event) error {
	if event.Topic != app.ConfigRefreshTopic {
		return b.next.Broadcast(ctx, event)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	version := rand.Text()
	_, err = b.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, outbox.VersionKey(event.Topic, 0), version, 0)
		pipe.Publish(ctx, event.Topic, data)
		return nil
	})
	return err
}
