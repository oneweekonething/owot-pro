package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type DictService struct {
	types  DictTypeRepository
	data   DictDataRepository
	tx     TxRunner
	outbox OutboxPublisher
}

func NewDictService(types DictTypeRepository, data DictDataRepository, tx TxRunner, outbox OutboxPublisher) *DictService {
	return &DictService{types: types, data: data, tx: tx, outbox: outbox}
}

func (s *DictService) CreateType(ctx context.Context, value domain.DictType) (int64, error) {
	if err := value.Validate(); err != nil {
		return 0, err
	}
	value.ID = 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.types.SaveType(ctx, &value); err != nil {
			return fmt.Errorf("create dictionary type: %w", err)
		}
		return s.invalidate(ctx, value.Type)
	})
	return value.ID, err
}

func (s *DictService) UpdateType(ctx context.Context, value domain.DictType) error {
	if value.ID <= 0 {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		old, err := s.types.FindType(ctx, value.ID)
		if err != nil {
			return fmt.Errorf("find dictionary type: %w", err)
		}
		if value.Type == "" {
			value.Type = old.Type // Frontend update DTOs may omit the immutable field.
		}
		if old.Type != value.Type {
			return domain.ErrDictTypeImmutable
		}
		if err := value.Validate(); err != nil {
			return err
		}
		if _, err := s.types.LockType(ctx, old.Type); err != nil {
			return fmt.Errorf("lock dictionary type: %w", err)
		}
		if err := s.types.SaveType(ctx, &value); err != nil {
			return fmt.Errorf("update dictionary type: %w", err)
		}
		return s.invalidate(ctx, value.Type)
	})
}

func (s *DictService) DeleteTypes(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		values := make([]domain.DictType, 0, len(ids))
		types := make([]string, 0, len(ids))
		for _, id := range uniqueIDs(ids) {
			value, err := s.types.FindType(ctx, id)
			if err != nil {
				return fmt.Errorf("find dictionary type: %w", err)
			}
			values = append(values, value)
			types = append(types, value.Type)
		}
		if err := s.lockTypes(ctx, types); err != nil {
			return err
		}
		for _, value := range values {
			hasData, err := s.data.HasData(ctx, value.Type)
			if err != nil {
				return fmt.Errorf("check dictionary items: %w", err)
			}
			if hasData {
				return domain.ErrDictTypeHasData
			}
			if err := s.types.DeleteType(ctx, value.ID); err != nil {
				return fmt.Errorf("delete dictionary type: %w", err)
			}
			if err := s.invalidate(ctx, value.Type); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *DictService) GetType(ctx context.Context, id int64) (domain.DictType, error) {
	if id <= 0 {
		return domain.DictType{}, domain.ErrInvalidArgument
	}
	value, err := s.types.FindType(ctx, id)
	return value, wrap("get dictionary type", err)
}

func (s *DictService) PageTypes(ctx context.Context, filter DictTypeFilter) (pagination.PageResult[domain.DictType], error) {
	if !validStatusFilter(filter.Status) || !validRange(filter.From, filter.To) {
		return pagination.PageResult[domain.DictType]{}, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.types.PageTypes(ctx, filter)
	return result, wrap("page dictionary types", err)
}

func (s *DictService) CreateData(ctx context.Context, value domain.DictData) (int64, error) {
	if err := value.Validate(); err != nil {
		return 0, err
	}
	value.ID = 0
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.types.LockType(ctx, value.DictType); err != nil {
			return fmt.Errorf("lock dictionary type: %w", err)
		}
		if err := s.data.SaveData(ctx, &value); err != nil {
			return fmt.Errorf("create dictionary item: %w", err)
		}
		return s.invalidate(ctx, value.DictType)
	})
	return value.ID, err
}

func (s *DictService) UpdateData(ctx context.Context, value domain.DictData) error {
	if err := value.Validate(); err != nil || value.ID <= 0 {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		old, err := s.data.FindData(ctx, value.ID)
		if err != nil {
			return fmt.Errorf("find dictionary item: %w", err)
		}
		if err := s.lockTypes(ctx, []string{old.DictType, value.DictType}); err != nil {
			return err
		}
		if err := s.data.SaveData(ctx, &value); err != nil {
			return fmt.Errorf("update dictionary item: %w", err)
		}
		if err := s.invalidate(ctx, old.DictType); err != nil {
			return err
		}
		if old.DictType != value.DictType {
			return s.invalidate(ctx, value.DictType)
		}
		return nil
	})
}

func (s *DictService) DeleteData(ctx context.Context, ids []int64) error {
	if !validIDs(ids) {
		return domain.ErrInvalidArgument
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		values := make([]domain.DictData, 0, len(ids))
		types := make([]string, 0, len(ids))
		for _, id := range uniqueIDs(ids) {
			value, err := s.data.FindData(ctx, id)
			if err != nil {
				return fmt.Errorf("find dictionary item: %w", err)
			}
			values = append(values, value)
			types = append(types, value.DictType)
		}
		if err := s.lockTypes(ctx, types); err != nil {
			return err
		}
		for _, value := range values {
			if err := s.data.DeleteData(ctx, value.ID); err != nil {
				return fmt.Errorf("delete dictionary item: %w", err)
			}
			if err := s.invalidate(ctx, value.DictType); err != nil {
				return err
			}
		}
		return nil
	})
}

// All multi-type mutations acquire every parent in the same order before writing children.
func (s *DictService) lockTypes(ctx context.Context, types []string) error {
	slices.Sort(types)
	for _, typ := range slices.Compact(types) {
		if _, err := s.types.LockType(ctx, typ); err != nil {
			return fmt.Errorf("lock dictionary type: %w", err)
		}
	}
	return nil
}

func (s *DictService) GetData(ctx context.Context, id int64) (domain.DictData, error) {
	if id <= 0 {
		return domain.DictData{}, domain.ErrInvalidArgument
	}
	value, err := s.data.FindData(ctx, id)
	return value, wrap("get dictionary item", err)
}

func (s *DictService) PageData(ctx context.Context, filter DictDataFilter) (pagination.PageResult[domain.DictData], error) {
	if !validStatusFilter(filter.Status) {
		return pagination.PageResult[domain.DictData]{}, domain.ErrInvalidArgument
	}
	filter.PageQuery = filter.Normalize()
	result, err := s.data.PageData(ctx, filter)
	return result, wrap("page dictionary items", err)
}

// EnabledData checks both type and item status. No process cache can serve stale data.
func (s *DictService) EnabledData(ctx context.Context, typ string) ([]domain.DictData, error) {
	if typ == "" {
		return nil, domain.ErrInvalidArgument
	}
	status := domain.StatusEnabled
	types, err := s.types.PageTypes(ctx, DictTypeFilter{Type: typ, Status: &status, PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		return nil, wrap("find enabled dictionary type", err)
	}
	for _, t := range types.List {
		if t.Type == typ {
			result, err := s.PageData(ctx, DictDataFilter{DictType: typ, Status: &status, PageQuery: pagination.PageQuery{PageSize: -1}})
			if result.List == nil {
				result.List = []domain.DictData{}
			}
			return result.List, err
		}
	}
	return []domain.DictData{}, nil
}

func (s *DictService) SimpleData(ctx context.Context) ([]domain.DictData, error) {
	status := domain.StatusEnabled
	types, err := s.PageTypes(ctx, DictTypeFilter{Status: &status, PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		return nil, err
	}
	enabled := make(map[string]bool, len(types.List))
	for _, typ := range types.List {
		enabled[typ.Type] = true
	}
	data, err := s.PageData(ctx, DictDataFilter{Status: &status, PageQuery: pagination.PageQuery{PageSize: -1}})
	if err != nil {
		return nil, err
	}
	result := make([]domain.DictData, 0, len(data.List))
	for _, item := range data.List {
		if enabled[item.DictType] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *DictService) GetLabel(ctx context.Context, typ, value string) (string, error) {
	items, err := s.EnabledData(ctx, typ)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Value == value {
			return item.Label, nil
		}
	}
	return "", domain.ErrDictLabelNotFound
}

func (s *DictService) GetValue(ctx context.Context, typ, label string) (string, error) {
	items, err := s.EnabledData(ctx, typ)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.Label == label {
			return item.Value, nil
		}
	}
	return "", domain.ErrDictLabelNotFound
}

func (s *DictService) invalidate(ctx context.Context, typ string) error {
	payload, err := json.Marshal(struct {
		DictType string `json:"dictType"`
	}{typ})
	if err != nil {
		return err
	}
	return wrap("append dictionary invalidation", s.outbox.Publish(ctxmeta.WithTenantID(ctx, 0), "system.dict.invalidate", payload))
}
