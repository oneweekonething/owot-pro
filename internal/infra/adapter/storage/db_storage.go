package storage

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

type fileContentPO struct {
	dbx.BasePO
	ConfigID int64  `gorm:"not null;uniqueIndex:uk_infra_file_content_path"`
	Path     string `gorm:"size:320;not null;uniqueIndex:uk_infra_file_content_path"`
	Content  []byte `gorm:"type:LONGBLOB;not null"`
}

func (fileContentPO) TableName() string { return "infra_file_content" }

func MigrateContent(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(&fileContentPO{})
}

type dbEngine struct {
	db       *gorm.DB
	configID int64
}

func (e *dbEngine) Put(ctx context.Context, path string, data []byte, _ string) error {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	return dbx.FromContext(ctx, e.db).Create(&fileContentPO{ConfigID: e.configID, Path: path, Content: data}).Error
}

func (e *dbEngine) Get(ctx context.Context, path string) ([]byte, error) {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return nil, err
	}
	var record fileContentPO
	err := dbx.FromContext(ctx, e.db).Select("content").Where("config_id = ? AND path = ?", e.configID, path).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrFileNotFound
	}
	return record.Content, err
}

func (e *dbEngine) Delete(ctx context.Context, path string) error {
	if _, err := domain.SanitizeFilePath(path); err != nil {
		return err
	}
	return dbx.FromContext(ctx, e.db).Model(&fileContentPO{}).Where("config_id = ? AND path = ?", e.configID, path).
		Updates(map[string]any{"deleted": 1, "content": []byte{}}).Error
}
