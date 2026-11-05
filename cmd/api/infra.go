package main

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	iamcache "oneweekonething.com/owot-pro/internal/iam/adapter/cache"
	infracache "oneweekonething.com/owot-pro/internal/infra/adapter/cache"
	infrapersistence "oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	infraapp "oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

func newConfigService(db *gorm.DB, store *outbox.Store, client redis.UniversalClient) (*infraapp.ConfigService, *infracache.ConfigCache, error) {
	cache, err := infracache.NewConfigCache(client)
	if err != nil {
		return nil, nil, err
	}
	return infraapp.NewConfigService(infrapersistence.NewConfigRepository(db), dbx.NewTxManager(db), store, cache), cache, nil
}

func newBackendBroadcaster(client redis.UniversalClient) outbox.Broadcaster {
	return infracache.NewConfigBroadcaster(client, iamcache.NewBroadcaster(client))
}
