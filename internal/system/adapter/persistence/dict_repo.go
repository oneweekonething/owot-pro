package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type DictRepository struct{ db *gorm.DB }

func NewDictRepository(db *gorm.DB) *DictRepository { return &DictRepository{db: db} }

func (r *DictRepository) FindType(ctx context.Context, id int64) (domain.DictType, error) {
	var row dictTypePO
	err := dbx.FromContext(ctx, r.db).First(&row, id).Error
	return typeEntity(row), notFound(err, domain.ErrDictTypeNotFound)
}

func (r *DictRepository) LockType(ctx context.Context, typ string) (domain.DictType, error) {
	var row dictTypePO
	err := dbx.FromContext(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ?", typ).First(&row).Error
	return typeEntity(row), notFound(err, domain.ErrDictTypeNotFound)
}

func (r *DictRepository) SaveType(ctx context.Context, value *domain.DictType) error {
	db := dbx.FromContext(ctx, r.db)
	if value.ID == 0 {
		row := dictTypePO{Name: value.Name, Type: value.Type, ActiveType: &value.Type, Status: value.Status, Remark: value.Remark}
		if err := db.Create(&row).Error; err != nil {
			return duplicate(err, domain.ErrDictTypeExists)
		}
		*value = typeEntity(row)
		return nil
	}
	query := db.Model(&dictTypePO{}).Where("id = ?", value.ID)
	result := query.Updates(map[string]any{"name": value.Name, "status": value.Status, "remark": value.Remark})
	return updateResult(result, domain.ErrDictTypeNotFound)
}

func (r *DictRepository) DeleteType(ctx context.Context, id int64) error {
	result := dbx.FromContext(ctx, r.db).Model(&dictTypePO{}).Where("id = ?", id).
		Updates(map[string]any{"deleted": 1, "active_type": nil})
	return updateResult(result, domain.ErrDictTypeNotFound)
}

func (r *DictRepository) PageTypes(ctx context.Context, f app.DictTypeFilter) (pagination.PageResult[domain.DictType], error) {
	query := dbx.FromContext(ctx, r.db).Model(&dictTypePO{})
	if f.Name != "" {
		query = query.Where("name LIKE ?", "%"+f.Name+"%")
	}
	if f.Type != "" {
		query = query.Where("type LIKE ?", "%"+f.Type+"%")
	}
	if f.Status != nil {
		query = query.Where("status = ?", *f.Status)
	}
	query = timeRange(query, f.From, f.To)
	var result pagination.PageResult[domain.DictType]
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []dictTypePO
	if err := page(query.Order("id DESC"), f.PageQuery).Find(&rows).Error; err != nil {
		return result, err
	}
	result.List = make([]domain.DictType, 0, len(rows))
	for _, row := range rows {
		result.List = append(result.List, typeEntity(row))
	}
	return result, nil
}

func (r *DictRepository) FindData(ctx context.Context, id int64) (domain.DictData, error) {
	var row dictDataPO
	err := dbx.FromContext(ctx, r.db).First(&row, id).Error
	return dataEntity(row), notFound(err, domain.ErrDictDataNotFound)
}

func (r *DictRepository) HasData(ctx context.Context, typ string) (bool, error) {
	// A locking read sees the latest committed items under MySQL REPEATABLE READ,
	// including items committed after this transaction's first non-locking read.
	var rows []dictDataPO
	result := dbx.FromContext(ctx, r.db).Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").Where("dict_type = ?", typ).Limit(1).Find(&rows)
	return result.RowsAffected > 0, result.Error
}

func (r *DictRepository) SaveData(ctx context.Context, value *domain.DictData) error {
	db := dbx.FromContext(ctx, r.db)
	if value.ID == 0 {
		row := dictDataPO{Sort: value.Sort, Label: value.Label, Value: value.Value, ActiveValue: &value.Value,
			DictType: value.DictType, Status: value.Status, ColorType: value.ColorType, CSSClass: value.CSSClass, Remark: value.Remark}
		if err := db.Create(&row).Error; err != nil {
			return duplicate(err, domain.ErrDictValueExists)
		}
		*value = dataEntity(row)
		return nil
	}
	result := db.Model(&dictDataPO{}).Where("id = ?", value.ID).Updates(map[string]any{
		"sort": value.Sort, "label": value.Label, "value": value.Value, "active_value": value.Value,
		"dict_type": value.DictType, "status": value.Status, "color_type": value.ColorType,
		"css_class": value.CSSClass, "remark": value.Remark,
	})
	if err := duplicate(result.Error, domain.ErrDictValueExists); err != nil {
		return err
	}
	return updateResult(result, domain.ErrDictDataNotFound)
}

func (r *DictRepository) DeleteData(ctx context.Context, id int64) error {
	result := dbx.FromContext(ctx, r.db).Model(&dictDataPO{}).Where("id = ?", id).
		Updates(map[string]any{"deleted": 1, "active_value": nil})
	return updateResult(result, domain.ErrDictDataNotFound)
}

func (r *DictRepository) PageData(ctx context.Context, f app.DictDataFilter) (pagination.PageResult[domain.DictData], error) {
	query := dbx.FromContext(ctx, r.db).Model(&dictDataPO{})
	if f.DictType != "" {
		query = query.Where("dict_type = ?", f.DictType)
	}
	if f.Label != "" {
		query = query.Where("label LIKE ?", "%"+f.Label+"%")
	}
	if f.Status != nil {
		query = query.Where("status = ?", *f.Status)
	}
	var result pagination.PageResult[domain.DictData]
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []dictDataPO
	if err := page(query.Order("sort ASC, id ASC"), f.PageQuery).Find(&rows).Error; err != nil {
		return result, err
	}
	result.List = make([]domain.DictData, 0, len(rows))
	for _, row := range rows {
		result.List = append(result.List, dataEntity(row))
	}
	return result, nil
}

func typeEntity(row dictTypePO) domain.DictType {
	return domain.DictType{ID: row.ID, Name: row.Name, Type: row.Type, Status: row.Status, Remark: row.Remark,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
}

func dataEntity(row dictDataPO) domain.DictData {
	return domain.DictData{ID: row.ID, Sort: row.Sort, Label: row.Label, Value: row.Value, DictType: row.DictType,
		Status: row.Status, ColorType: row.ColorType, CSSClass: row.CSSClass, Remark: row.Remark,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
}

func notFound(err, target error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return target
	}
	return err
}

func duplicate(err, target error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return target
	}
	return err
}
