package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
)

type cachedEngine struct {
	settings domain.FileSettings
	engine   *s3Engine
}

type Factory struct {
	db        *gorm.DB
	redis     redis.UniversalClient
	localRoot string
	mu        sync.Mutex
	version   string
	s3        map[int64]cachedEngine
}

func NewFactory(db *gorm.DB, client redis.UniversalClient, root string) *Factory {
	return &Factory{db: db, redis: client, localRoot: root, s3: make(map[int64]cachedEngine)}
}

func (f *Factory) Engine(ctx context.Context, config domain.FileConfig) (app.FileStorageEngine, error) {
	switch config.Storage {
	case domain.StorageDB:
		return &dbEngine{db: f.db, configID: config.ID}, nil
	case domain.StorageLocal:
		base := config.Settings.BasePath
		if filepath.IsAbs(base) {
			var err error
			base, err = filepath.Rel(f.localRoot, base)
			if err != nil {
				return nil, domain.ErrInvalidArgument
			}
		}
		if base != "" && (!filepath.IsLocal(base) || filepath.Clean(base) != base) {
			return nil, domain.ErrInvalidArgument
		}
		return &localEngine{root: f.localRoot, directory: filepath.Join(base, strconv.FormatInt(config.ID, 10))}, nil
	case domain.StorageS3:
		if config.ID == 0 || f.redis == nil {
			return newS3(config.Settings)
		}
		version, err := f.redis.Get(ctx, outbox.VersionKey(app.FileConfigRefreshTopic, 0)).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return newS3(config.Settings)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if version != f.version {
			clear(f.s3)
			f.version = version
		}
		// SQL supplies the current settings even when outbox delivery is delayed.
		if entry, ok := f.s3[config.ID]; ok && entry.settings == config.Settings {
			return entry.engine, nil
		}
		engine, err := newS3(config.Settings)
		if err != nil {
			return nil, err
		}
		if len(f.s3) >= 64 {
			clear(f.s3)
		}
		f.s3[config.ID] = cachedEngine{settings: config.Settings, engine: engine}
		return engine, nil
	default:
		return nil, domain.ErrInvalidArgument
	}
}
