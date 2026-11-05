package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type configPO struct {
	dbx.BasePO
	Category  string  `gorm:"size:50;not null"`
	Name      string  `gorm:"size:100;not null"`
	ConfigKey string  `gorm:"size:100;not null"`
	ActiveKey *string `gorm:"type:VARCHAR(100) GENERATED ALWAYS AS (CASE WHEN deleted = 0 THEN config_key ELSE NULL END) VIRTUAL;->;uniqueIndex:uk_infra_config_active_key"`
	Value     string  `gorm:"size:500;not null"`
	Type      int     `gorm:"not null"`
	Visible   bool    `gorm:"not null;default:false"`
	Remark    string  `gorm:"size:500;not null;default:''"`
}

func (configPO) TableName() string { return "infra_config" }

func (p configPO) toDomain() domain.Config {
	return domain.Config{ID: p.ID, Category: p.Category, Name: p.Name, Key: p.ConfigKey, Value: p.Value,
		Type: p.Type, Visible: p.Visible, Remark: p.Remark, CreateTime: p.CreateTime, UpdateTime: p.UpdateTime}
}

type ConfigRepository struct{ db *gorm.DB }

func NewConfigRepository(db *gorm.DB) *ConfigRepository { return &ConfigRepository{db: db} }

func Migrate(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(&configPO{})
}

func CheckSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable(&configPO{}) {
		return errors.New("infra_config table is missing; apply migrations/mysql/003_infra_config.sql")
	}
	return nil
}

func (r *ConfigRepository) Get(ctx context.Context, id int64) (domain.Config, error) {
	var record configPO
	query := dbx.FromContext(ctx, r.db)
	if dbx.InTransaction(ctx) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := query.First(&record, id).Error
	return record.toDomain(), configError(err)
}

func (r *ConfigRepository) FindByKey(ctx context.Context, key string) (domain.Config, error) {
	var record configPO
	err := dbx.FromContext(ctx, r.db).Where("active_key = ?", key).First(&record).Error
	return record.toDomain(), configError(err)
}

func (r *ConfigRepository) Save(ctx context.Context, config *domain.Config) error {
	db := dbx.FromContext(ctx, r.db)
	if config.ID == 0 {
		record := configPO{Category: config.Category, Name: config.Name, ConfigKey: config.Key,
			Value: config.Value, Type: config.Type, Visible: config.Visible, Remark: config.Remark}
		if err := db.Create(&record).Error; err != nil {
			return configError(err)
		}
		*config = record.toDomain()
		return nil
	}
	return configError(db.Model(&configPO{}).Where("id = ?", config.ID).Updates(map[string]any{
		"category": config.Category, "name": config.Name, "config_key": config.Key,
		"value": config.Value, "visible": config.Visible, "remark": config.Remark,
	}).Error)
}

func (r *ConfigRepository) Delete(ctx context.Context, id int64) error {
	return dbx.FromContext(ctx, r.db).Delete(&configPO{}, id).Error
}

func (r *ConfigRepository) Page(ctx context.Context, filter app.ConfigFilter) (pagination.PageResult[domain.Config], error) {
	query := dbx.FromContext(ctx, r.db).Model(&configPO{})
	if filter.Name != "" {
		query = query.Where("name LIKE ? ESCAPE '!'", containsPattern(filter.Name))
	}
	if filter.Key != "" {
		query = query.Where("config_key LIKE ? ESCAPE '!'", containsPattern(filter.Key))
	}
	if filter.Type != nil {
		query = query.Where("type = ?", *filter.Type)
	}
	if filter.From != nil {
		query = query.Where("create_time >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("create_time <= ?", *filter.To)
	}
	var result pagination.PageResult[domain.Config]
	if filter.PageSize != -1 {
		if err := query.Count(&result.Total).Error; err != nil {
			return result, fmt.Errorf("count configs: %w", err)
		}
		query = query.Offset(filter.Offset()).Limit(filter.PageSize)
	}
	var records []configPO
	if err := query.Order("id DESC").Find(&records).Error; err != nil {
		return result, err
	}
	if filter.PageSize == -1 {
		result.Total = int64(len(records))
	}
	result.List = make([]domain.Config, 0, len(records))
	for _, record := range records {
		result.List = append(result.List, record.toDomain())
	}
	return result, nil
}

func containsPattern(value string) string {
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value) + "%"
}

func configError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrConfigNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrConfigKeyExists
	default:
		return err
	}
}
