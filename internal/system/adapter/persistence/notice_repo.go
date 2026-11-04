package persistence

import (
	"context"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type NoticeRepository struct{ db *gorm.DB }

func NewNoticeRepository(db *gorm.DB) *NoticeRepository { return &NoticeRepository{db: db} }

func (r *NoticeRepository) FindNotice(ctx context.Context, id int64) (domain.Notice, error) {
	var row noticePO
	err := dbx.FromContext(ctx, r.db).First(&row, id).Error
	return noticeEntity(row), notFound(err, domain.ErrNoticeNotFound)
}

func (r *NoticeRepository) SaveNotice(ctx context.Context, value *domain.Notice) error {
	db := dbx.FromContext(ctx, r.db)
	if value.ID == 0 {
		row := noticePO{Title: value.Title, Type: value.Type, Content: value.Content, Status: value.Status}
		row.TenantID = value.TenantID
		if err := db.Create(&row).Error; err != nil {
			return err
		}
		*value = noticeEntity(row)
		return nil
	}
	result := db.Model(&noticePO{}).Where("id = ?", value.ID).Updates(map[string]any{
		"title": value.Title, "type": value.Type, "content": value.Content, "status": value.Status,
	})
	return updateResult(result, domain.ErrNoticeNotFound)
}

func (r *NoticeRepository) DeleteNotice(ctx context.Context, id int64) error {
	result := dbx.FromContext(ctx, r.db).Where("id = ?", id).Delete(&noticePO{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNoticeNotFound
	}
	return nil
}

func (r *NoticeRepository) PageNotices(ctx context.Context, f app.NoticeFilter) (pagination.PageResult[domain.Notice], error) {
	query := dbx.FromContext(ctx, r.db).Model(&noticePO{})
	if f.Title != "" {
		query = query.Where("title LIKE ?", "%"+f.Title+"%")
	}
	if f.Status != nil {
		query = query.Where("status = ?", *f.Status)
	}
	if f.Type != nil {
		query = query.Where("type = ?", *f.Type)
	}
	var result pagination.PageResult[domain.Notice]
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []noticePO
	if err := page(query.Order("id DESC"), f.PageQuery).Find(&rows).Error; err != nil {
		return result, err
	}
	result.List = make([]domain.Notice, 0, len(rows))
	for _, row := range rows {
		result.List = append(result.List, noticeEntity(row))
	}
	return result, nil
}

func noticeEntity(row noticePO) domain.Notice {
	return domain.Notice{ID: row.ID, TenantID: row.TenantID, Title: row.Title, Type: row.Type, Content: row.Content,
		Status: row.Status, Creator: row.Creator, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime}
}
