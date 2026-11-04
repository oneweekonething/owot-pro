package persistence

import (
	"time"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func page(query *gorm.DB, p pagination.PageQuery) *gorm.DB {
	p = p.Normalize()
	if p.PageSize == -1 {
		return query
	}
	return query.Offset(p.Offset()).Limit(p.PageSize)
}

func timeRange(query *gorm.DB, from, to *time.Time) *gorm.DB {
	if from != nil {
		query = query.Where("create_time >= ?", *from)
	}
	if to != nil {
		query = query.Where("create_time <= ?", *to)
	}
	return query
}

func updateResult(result *gorm.DB, missing error) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL may report zero for a no-op update. Check existence with the same scope.
		var count int64
		if err := result.Session(&gorm.Session{}).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return missing
		}
	}
	return nil
}
