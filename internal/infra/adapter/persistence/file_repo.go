package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type fileConfigPO struct {
	dbx.BasePO
	Name       string `gorm:"size:100;not null"`
	Storage    int    `gorm:"not null"`
	Remark     string `gorm:"size:500;not null;default:''"`
	Master     bool   `gorm:"not null;default:false"`
	MasterSlot *int   `gorm:"type:INT GENERATED ALWAYS AS (CASE WHEN deleted = 0 AND master = 1 THEN 1 ELSE NULL END) VIRTUAL;->;uniqueIndex:uk_infra_file_master"`
	Config     string `gorm:"type:JSON;not null"`
}

func (fileConfigPO) TableName() string { return "infra_file_config" }

func (p fileConfigPO) domain() (domain.FileConfig, error) {
	c := domain.FileConfig{ID: p.ID, Name: p.Name, Storage: p.Storage, Remark: p.Remark, Master: p.Master, CreateTime: p.CreateTime}
	err := json.Unmarshal([]byte(p.Config), &c.Settings)
	return c, err
}

type filePO struct {
	dbx.BasePO
	ConfigID        int64      `gorm:"not null;uniqueIndex:uk_infra_file_path"`
	OwnerTenantID   int64      `gorm:"not null;index:idx_infra_file_owner"`
	OwnerUserID     int64      `gorm:"not null"`
	Name            string     `gorm:"size:180;not null"`
	Path            string     `gorm:"size:320;not null;uniqueIndex:uk_infra_file_path"`
	URL             string     `gorm:"size:1500;not null"`
	Type            string     `gorm:"size:128;not null;default:''"`
	Size            int64      `gorm:"not null;default:0"`
	UploadExpiresAt *time.Time `gorm:"index:idx_infra_file_pending"`
	Avatar          bool       `gorm:"not null;default:false"`
	CleanupPending  bool       `gorm:"not null;default:false"`
}

func (filePO) TableName() string { return "infra_file" }

func (p filePO) domain() domain.FileObject {
	return domain.FileObject{ID: p.ID, ConfigID: p.ConfigID, OwnerTenantID: p.OwnerTenantID, OwnerUserID: p.OwnerUserID,
		Name: p.Name, Path: p.Path, URL: p.URL, Type: p.Type, Size: p.Size, CreateTime: p.CreateTime, UploadExpiresAt: p.UploadExpiresAt, Avatar: p.Avatar, CleanupPending: p.CleanupPending}
}

type FileRepository struct{ db *gorm.DB }

func NewFileRepository(db *gorm.DB) *FileRepository { return &FileRepository{db: db} }

func MigrateFiles(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	if err := db.AutoMigrate(&fileConfigPO{}, &filePO{}); err != nil {
		return err
	}
	return db.Exec(`INSERT INTO infra_file_config (id, name, storage, master, config, create_time, update_time)
		SELECT 1, '数据库存储', 1, 1, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		WHERE NOT EXISTS (SELECT 1 FROM infra_file_config)`).Error
}

func CheckFileSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable(&fileConfigPO{}) || !db.Migrator().HasTable(&filePO{}) ||
		!db.Migrator().HasTable("infra_file_content") {
		return errors.New("infra file tables are missing; apply migrations/mysql/004_infra_file.sql")
	}
	return nil
}

func (r *FileRepository) locked(ctx context.Context) *gorm.DB {
	db := dbx.FromContext(ctx, r.db)
	if dbx.InTransaction(ctx) {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return db
}

func (r *FileRepository) config(query *gorm.DB) (domain.FileConfig, error) {
	var record fileConfigPO
	if err := query.First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.FileConfig{}, domain.ErrFileConfigNotFound
		}
		return domain.FileConfig{}, err
	}
	return record.domain()
}

func (r *FileRepository) Config(ctx context.Context, id int64) (domain.FileConfig, error) {
	return r.config(r.locked(ctx).Where("id = ?", id))
}

func (r *FileRepository) Master(ctx context.Context) (domain.FileConfig, error) {
	db := dbx.FromContext(ctx, r.db)
	if dbx.InTransaction(ctx) {
		if err := r.lockMasterSlot(ctx, clause.LockingStrengthShare); err != nil {
			return domain.FileConfig{}, err
		}
		db = db.Clauses(clause.Locking{Strength: clause.LockingStrengthShare})
	}
	return r.config(db.Where("master_slot = 1"))
}

func (r *FileRepository) SaveConfig(ctx context.Context, c *domain.FileConfig) error {
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return err
	}
	db := dbx.FromContext(ctx, r.db)
	if c.ID != 0 {
		return db.Model(&fileConfigPO{}).Where("id = ?", c.ID).Updates(map[string]any{
			"name": c.Name, "remark": c.Remark, "config": string(settings),
		}).Error
	}
	record := fileConfigPO{Name: c.Name, Remark: c.Remark, Storage: c.Storage, Config: string(settings)}
	if err := db.Create(&record).Error; err != nil {
		return err
	}
	c.ID = record.ID
	return nil
}

func (r *FileRepository) SwitchMaster(ctx context.Context, id int64) error {
	db := dbx.FromContext(ctx, r.db)
	if err := r.lockMasterSlot(ctx, clause.LockingStrengthUpdate); err != nil {
		return err
	}
	if err := db.Model(&fileConfigPO{}).Where("master_slot = 1").Update("master", false).Error; err != nil {
		return err
	}
	result := db.Model(&fileConfigPO{}).Where("id = ?", id).Update("master", true)
	if result.Error == nil && result.RowsAffected == 0 {
		return domain.ErrFileConfigNotFound
	}
	return result.Error
}

// The seeded row remains available as a lock even after soft deletion.
// Shared locks let uploads run concurrently and exclude a master switch.
func (r *FileRepository) lockMasterSlot(ctx context.Context, strength string) error {
	var anchor fileConfigPO
	return dbx.FromContext(ctx, r.db).Unscoped().Select("id").
		Clauses(clause.Locking{Strength: strength}).First(&anchor, 1).Error
}

func (r *FileRepository) DeleteConfig(ctx context.Context, id int64) error {
	return dbx.FromContext(ctx, r.db).Delete(&fileConfigPO{}, id).Error
}

func (r *FileRepository) ConfigInUse(ctx context.Context, id int64, now time.Time) (bool, error) {
	var record filePO
	err := r.locked(ctx).Unscoped().Select("id").Where("config_id = ? AND (deleted = 0 OR cleanup_pending = TRUE) AND (upload_expires_at IS NULL OR upload_expires_at > ?)", id, now).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (r *FileRepository) CreateFile(ctx context.Context, f *domain.FileObject) error {
	p := filePO{ConfigID: f.ConfigID, OwnerTenantID: f.OwnerTenantID, OwnerUserID: f.OwnerUserID, Name: f.Name,
		Path: f.Path, URL: f.URL, Type: f.Type, Size: f.Size, UploadExpiresAt: f.UploadExpiresAt, Avatar: f.Avatar}
	if err := dbx.FromContext(ctx, r.db).Create(&p).Error; err != nil {
		return err
	}
	f.ID, f.CreateTime = p.ID, p.CreateTime
	return nil
}

func (r *FileRepository) CompleteFile(ctx context.Context, f domain.FileObject) error {
	return dbx.FromContext(ctx, r.db).Model(&filePO{}).Where("id = ?", f.ID).Updates(map[string]any{
		"type": f.Type, "size": f.Size, "upload_expires_at": nil,
	}).Error
}

func (r *FileRepository) FindFile(ctx context.Context, configID int64, path string) (domain.FileObject, error) {
	var p filePO
	err := r.locked(ctx).Where("config_id = ? AND path = ?", configID, path).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.FileObject{}, domain.ErrFileNotFound
	}
	return p.domain(), err
}

func (r *FileRepository) FilesByID(ctx context.Context, ids []int64) ([]domain.FileObject, error) {
	var records []filePO
	err := r.locked(ctx).Unscoped().Where("owner_tenant_id = ? AND id IN ? AND upload_expires_at IS NULL AND (deleted = 0 OR cleanup_pending = TRUE)",
		ctxmeta.TenantID(ctx), ids).Order("id").Find(&records).Error
	files := make([]domain.FileObject, 0, len(records))
	for _, record := range records {
		files = append(files, record.domain())
	}
	return files, err
}

func (r *FileRepository) MarkFileForDeletion(ctx context.Context, id int64) error {
	return dbx.FromContext(ctx, r.db).Model(&filePO{}).
		Where("id = ? AND owner_tenant_id = ?", id, ctxmeta.TenantID(ctx)).
		Updates(map[string]any{"deleted": 1, "cleanup_pending": true}).Error
}

func (r *FileRepository) FinishFileDeletion(ctx context.Context, id int64) error {
	return dbx.FromContext(ctx, r.db).Unscoped().Model(&filePO{}).
		Where("id = ? AND owner_tenant_id = ? AND deleted = 1", id, ctxmeta.TenantID(ctx)).
		Update("cleanup_pending", false).Error
}

func (r *FileRepository) PrunePending(ctx context.Context, configID int64, now time.Time) error {
	return dbx.FromContext(ctx, r.db).Unscoped().Where("config_id = ? AND upload_expires_at <= ?", configID, now).Delete(&filePO{}).Error
}

func (r *FileRepository) PageConfigs(ctx context.Context, filter app.FileFilter) (pagination.PageResult[domain.FileConfig], error) {
	q := dbx.FromContext(ctx, r.db).Model(&fileConfigPO{})
	if filter.Storage != 0 {
		q = q.Where("storage = ?", filter.Storage)
	}
	records, total, err := filePage[fileConfigPO](q, filter)
	result := pagination.PageResult[domain.FileConfig]{List: make([]domain.FileConfig, 0, len(records)), Total: total}
	if err != nil {
		return result, err
	}
	for _, record := range records {
		config, err := record.domain()
		if err != nil {
			return result, err
		}
		result.List = append(result.List, config)
	}
	return result, nil
}

func (r *FileRepository) PageFiles(ctx context.Context, filter app.FileFilter) (pagination.PageResult[domain.FileObject], error) {
	q := dbx.FromContext(ctx, r.db).Unscoped().Model(&filePO{}).Where("owner_tenant_id = ? AND upload_expires_at IS NULL AND (deleted = 0 OR cleanup_pending = TRUE)", ctxmeta.TenantID(ctx))
	if filter.Path != "" {
		q = q.Where("path LIKE ? ESCAPE '!'", containsPattern(filter.Path))
	}
	if filter.Type != "" {
		q = q.Where("type LIKE ? ESCAPE '!'", containsPattern(filter.Type))
	}
	records, total, err := filePage[filePO](q, filter)
	result := pagination.PageResult[domain.FileObject]{List: make([]domain.FileObject, 0, len(records)), Total: total}
	for _, record := range records {
		result.List = append(result.List, record.domain())
	}
	return result, err
}

func filePage[T any](q *gorm.DB, filter app.FileFilter) ([]T, int64, error) {
	if filter.Name != "" {
		q = q.Where("name LIKE ? ESCAPE '!'", containsPattern(filter.Name))
	}
	if filter.From != nil {
		q = q.Where("create_time >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("create_time <= ?", *filter.To)
	}
	var total int64
	if filter.PageSize != -1 {
		if err := q.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		q = q.Offset(filter.Offset()).Limit(filter.PageSize)
	}
	var records []T
	err := q.Order("id DESC").Find(&records).Error
	if filter.PageSize == -1 {
		total = int64(len(records))
	}
	return records, total, err
}
