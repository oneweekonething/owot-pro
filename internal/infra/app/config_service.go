package app

import (
	"context"
	"fmt"
	"slices"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

const ConfigRefreshTopic = "infra.config.refresh"

type ConfigService struct {
	repo   ConfigRepository
	tx     TxRunner
	outbox OutboxPublisher
	cache  ConfigValueCache
}

func NewConfigService(repo ConfigRepository, tx TxRunner, outbox OutboxPublisher, cache ConfigValueCache) *ConfigService {
	return &ConfigService{repo: repo, tx: tx, outbox: outbox, cache: cache}
}

func (s *ConfigService) Get(ctx context.Context, id int64) (domain.Config, error) {
	if id <= 0 {
		return domain.Config{}, domain.ErrInvalidArgument
	}
	config, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Config{}, fmt.Errorf("get config: %w", err)
	}
	return config, nil
}

func (s *ConfigService) Page(ctx context.Context, filter ConfigFilter) (pagination.PageResult[domain.Config], error) {
	if filter.Type != nil && *filter.Type != domain.ConfigBuiltin && *filter.Type != domain.ConfigCustom ||
		filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return pagination.PageResult[domain.Config]{}, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.repo.Page(ctx, filter)
	if err != nil {
		return result, fmt.Errorf("page configs: %w", err)
	}
	return result, nil
}

func (s *ConfigService) Create(ctx context.Context, config domain.Config) (int64, error) {
	if err := config.Validate(); err != nil {
		return 0, err
	}
	config.ID, config.Type = 0, domain.ConfigCustom
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		return s.save(ctx, &config)
	})
	if err != nil {
		return 0, fmt.Errorf("create config: %w", err)
	}
	return config.ID, nil
}

func (s *ConfigService) Update(ctx context.Context, config domain.Config) error {
	if config.ID <= 0 {
		return domain.ErrInvalidArgument
	}
	if err := config.Validate(); err != nil {
		return err
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		current, err := s.repo.Get(ctx, config.ID)
		if err != nil {
			return err
		}
		config.Type = current.Type
		return s.save(ctx, &config)
	})
	if err != nil {
		return fmt.Errorf("update config: %w", err)
	}
	return nil
}

func (s *ConfigService) Delete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 || len(ids) > 1000 {
		return domain.ErrInvalidArgument
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	if ids[0] <= 0 {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		for _, id := range slices.Compact(ids) {
			current, err := s.repo.Get(ctx, id)
			if err != nil {
				return fmt.Errorf("get config %d for deletion: %w", id, err)
			}
			if err := current.CanDelete(); err != nil {
				return err
			}
			if err := s.repo.Delete(ctx, id); err != nil {
				return fmt.Errorf("delete config %d: %w", id, err)
			}
		}
		return s.refresh(ctx)
	})
}

// Value is for trusted in-process consumers. It may read private configuration.
func (s *ConfigService) Value(ctx context.Context, key string) (string, error) {
	if err := domain.ValidateConfigKey(key); err != nil {
		return "", err
	}
	load := func(ctx context.Context) (string, error) {
		config, err := s.repo.FindByKey(ctx, key)
		return config.Value, err
	}
	var value string
	var err error
	if s.cache == nil {
		value, err = load(ctx)
	} else {
		value, err = s.cache.Load(ctx, key, load)
	}
	if err != nil {
		return "", fmt.Errorf("read config value: %w", err)
	}
	return value, nil
}

// VisibleValue reads the value and visibility together from SQL so a delayed
// invalidation cannot expose a configuration after it has been made private.
func (s *ConfigService) VisibleValue(ctx context.Context, key string) (string, error) {
	if err := domain.ValidateConfigKey(key); err != nil {
		return "", err
	}
	config, err := s.repo.FindByKey(ctx, key)
	if err != nil {
		return "", fmt.Errorf("read visible config: %w", err)
	}
	if !config.Visible {
		return "", domain.ErrConfigHidden
	}
	return config.Value, nil
}

func (s *ConfigService) save(ctx context.Context, config *domain.Config) error {
	if err := s.repo.Save(ctx, config); err != nil {
		return err
	}
	return s.refresh(ctx)
}

func (s *ConfigService) refresh(ctx context.Context) error {
	if err := s.outbox.Publish(ctxmeta.WithTenantID(ctx, 0), ConfigRefreshTopic, []byte("{}")); err != nil {
		return fmt.Errorf("publish config refresh: %w", err)
	}
	return nil
}
