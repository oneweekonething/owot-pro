package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/iam/adapter/avatar"
	iamcache "oneweekonething.com/owot-pro/internal/iam/adapter/cache"
	infracache "oneweekonething.com/owot-pro/internal/infra/adapter/cache"
	infrapersistence "oneweekonething.com/owot-pro/internal/infra/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/infra/adapter/storage"
	infraapp "oneweekonething.com/owot-pro/internal/infra/app"
	infradomain "oneweekonething.com/owot-pro/internal/infra/domain"
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

func migrateFileStorage(db *gorm.DB) error {
	if err := infrapersistence.MigrateFiles(db); err != nil {
		return err
	}
	return storage.MigrateContent(db)
}

func newFileService(db *gorm.DB, store *outbox.Store, client redis.UniversalClient, directory, publicBase string) (*infraapp.FileService, error) {
	if directory == "" {
		directory = filepath.Join("data", "infra-files")
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	publicBase, err = infradomain.NormalizeFileDomain(publicBase)
	if err != nil {
		return nil, fmt.Errorf("invalid OWOT_FILE_PUBLIC_URL: %w", err)
	}
	repo := infrapersistence.NewFileRepository(db)
	repos := infraapp.FileRepositories{Configs: repo, ConfigWrites: repo, Files: repo, FileWrites: repo}
	return infraapp.NewFileService(repos, dbx.NewTxManager(db), store, storage.NewFactory(db, client, root), publicBase), nil
}

type infraAvatars struct{ files *infraapp.FileService }

func (a *infraAvatars) Upload(ctx context.Context, _ string, data []byte) (string, error) {
	format, err := avatar.Validate(data)
	if err != nil {
		return "", err
	}
	return a.files.UploadAvatar(ctx, "avatar."+format, data)
}
