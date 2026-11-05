package app

import (
	"context"
	"slices"
	"time"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

const FileConfigRefreshTopic = "infra.file-config.refresh"

type FileRepositories struct {
	Configs      FileConfigReader
	ConfigWrites FileConfigWriter
	Files        FileMetadataReader
	FileWrites   FileMetadataWriter
}

type FileService struct {
	repo       FileRepositories
	tx         TxRunner
	outbox     OutboxPublisher
	storage    FileStorageFactory
	publicBase string
}

func NewFileService(repo FileRepositories, tx TxRunner, outbox OutboxPublisher, storage FileStorageFactory, publicBase string) *FileService {
	return &FileService{repo: repo, tx: tx, outbox: outbox, storage: storage, publicBase: publicBase}
}

func (s *FileService) GetConfig(ctx context.Context, id int64) (domain.FileConfig, error) {
	if id <= 0 {
		return domain.FileConfig{}, domain.ErrInvalidArgument
	}
	return s.repo.Configs.Config(ctx, id)
}

func (s *FileService) PageConfigs(ctx context.Context, filter FileFilter) (pagination.PageResult[domain.FileConfig], error) {
	if err := normalizeFileFilter(&filter); err != nil {
		return pagination.PageResult[domain.FileConfig]{}, err
	}
	if filter.Storage != 0 && filter.Storage != domain.StorageDB && filter.Storage != domain.StorageLocal && filter.Storage != domain.StorageS3 {
		return pagination.PageResult[domain.FileConfig]{}, domain.ErrInvalidArgument
	}
	return s.repo.Configs.PageConfigs(ctx, filter)
}

func (s *FileService) CreateConfig(ctx context.Context, c domain.FileConfig) (int64, error) {
	c.ID, c.Master = 0, false
	if err := c.Validate(); err != nil {
		return 0, err
	}
	if _, err := s.storage.Engine(ctx, c); err != nil {
		return 0, err
	}
	err := s.repo.ConfigWrites.SaveConfig(ctx, &c)
	return c.ID, err
}

func (s *FileService) UpdateConfig(ctx context.Context, c domain.FileConfig) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := s.GetConfig(ctx, c.ID)
		if err != nil {
			return err
		}
		c.Storage = current.Storage
		if c.Settings.AccessSecret == "" {
			c.Settings.AccessSecret = current.Settings.AccessSecret
		}
		if err := c.Validate(); err != nil {
			return err
		}
		if !current.Settings.SameLocation(c.Settings) {
			inUse, err := s.repo.Files.ConfigInUse(ctx, c.ID, time.Now().UTC())
			if err != nil {
				return err
			}
			if inUse {
				return domain.ErrFileConfigInUse
			}
		}
		if _, err := s.storage.Engine(ctx, c); err != nil {
			return err
		}
		if err := s.repo.ConfigWrites.SaveConfig(ctx, &c); err != nil {
			return err
		}
		return s.refreshFileConfigs(ctx)
	})
}

func (s *FileService) UpdateMaster(ctx context.Context, id int64) error {
	if id <= 0 {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.ConfigWrites.SwitchMaster(ctx, id); err != nil {
			return err
		}
		return s.refreshFileConfigs(ctx)
	})
}

func (s *FileService) DeleteConfigs(ctx context.Context, ids []int64) error {
	ids, err := fileIDs(ids)
	if err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		now := time.Now().UTC()
		for _, id := range ids {
			c, err := s.repo.Configs.Config(ctx, id)
			if err != nil {
				return err
			}
			if c.Master {
				return domain.ErrFileMasterDelete
			}
			inUse, err := s.repo.Files.ConfigInUse(ctx, id, now)
			if err != nil {
				return err
			}
			if inUse {
				return domain.ErrFileConfigInUse
			}
			if err := s.repo.FileWrites.PrunePending(ctx, id, now); err != nil {
				return err
			}
			if err := s.repo.ConfigWrites.DeleteConfig(ctx, id); err != nil {
				return err
			}
		}
		return s.refreshFileConfigs(ctx)
	})
}

func (s *FileService) refreshFileConfigs(ctx context.Context) error {
	return s.outbox.Publish(ctxmeta.WithTenantID(ctx, 0), FileConfigRefreshTopic, []byte("{}"))
}

func normalizeFileFilter(filter *FileFilter) error {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	return nil
}

func fileIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, domain.ErrInvalidArgument
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	if ids[0] <= 0 {
		return nil, domain.ErrInvalidArgument
	}
	return slices.Compact(ids), nil
}
