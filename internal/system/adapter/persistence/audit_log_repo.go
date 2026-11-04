package persistence

import (
	"context"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type AuditLogRepository struct{ db *gorm.DB }

func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository { return &AuditLogRepository{db: db} }

func (r *AuditLogRepository) AppendOperateLog(ctx context.Context, value *domain.OperateLog) error {
	row := operateLogPO{TraceID: value.TraceID, UserID: value.UserID, UserType: value.UserType, Type: value.Type,
		SubType: value.SubType, BizID: value.BizID, Action: value.Action, Extra: value.Extra,
		RequestMethod: value.RequestMethod, RequestURL: value.RequestURL, UserIP: value.UserIP, UserAgent: value.UserAgent}
	row.TenantID = value.TenantID
	if err := dbx.FromContext(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}
	value.ID, value.CreateTime = row.ID, row.CreateTime
	return nil
}

func (r *AuditLogRepository) AppendLoginLog(ctx context.Context, value *domain.LoginLog) error {
	row := loginLogPO{LogType: value.LogType, TraceID: value.TraceID, UserID: value.UserID, UserType: value.UserType,
		Username: value.Username, Result: value.Result, UserIP: value.UserIP, UserAgent: value.UserAgent}
	row.TenantID = value.TenantID
	if err := dbx.FromContext(ctx, r.db).Create(&row).Error; err != nil {
		return err
	}
	value.ID, value.CreateTime = row.ID, row.CreateTime
	return nil
}

func (r *AuditLogRepository) PageOperateLogs(ctx context.Context, f app.OperateLogFilter) (pagination.PageResult[domain.OperateLog], error) {
	query := dbx.FromContext(ctx, r.db).Model(&operateLogPO{})
	if f.UserID != nil {
		query = query.Where("user_id = ?", *f.UserID)
	}
	if f.BizID != nil {
		query = query.Where("biz_id = ?", *f.BizID)
	}
	if f.Action != "" {
		query = query.Where("action LIKE ?", "%"+f.Action+"%")
	}
	if len(f.UserIDs) > 0 {
		query = query.Where("user_id IN ?", f.UserIDs)
	}
	if f.Type != "" {
		query = query.Where("type LIKE ?", "%"+f.Type+"%")
	}
	if f.SubType != "" {
		query = query.Where("sub_type LIKE ?", "%"+f.SubType+"%")
	}
	query = timeRange(query, f.From, f.To)
	var result pagination.PageResult[domain.OperateLog]
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []operateLogPO
	if err := page(query.Order("id DESC"), f.PageQuery).Find(&rows).Error; err != nil {
		return result, err
	}
	result.List = make([]domain.OperateLog, 0, len(rows))
	for _, row := range rows {
		result.List = append(result.List, domain.OperateLog{ID: row.ID, TenantID: row.TenantID, TraceID: row.TraceID,
			UserID: row.UserID, UserType: row.UserType, Type: row.Type, SubType: row.SubType, BizID: row.BizID,
			Action: row.Action, Extra: row.Extra, RequestMethod: row.RequestMethod, RequestURL: row.RequestURL,
			UserIP: row.UserIP, UserAgent: row.UserAgent, CreateTime: row.CreateTime})
	}
	return result, nil
}

func (r *AuditLogRepository) PageLoginLogs(ctx context.Context, f app.LoginLogFilter) (pagination.PageResult[domain.LoginLog], error) {
	query := dbx.FromContext(ctx, r.db).Model(&loginLogPO{})
	if f.Username != "" {
		query = query.Where("username LIKE ?", "%"+f.Username+"%")
	}
	if f.UserIP != "" {
		query = query.Where("user_ip LIKE ?", "%"+f.UserIP+"%")
	}
	if f.Status != nil {
		if *f.Status {
			query = query.Where("result = 0")
		} else {
			query = query.Where("result <> 0")
		}
	}
	query = timeRange(query, f.From, f.To)
	var result pagination.PageResult[domain.LoginLog]
	if err := query.Count(&result.Total).Error; err != nil {
		return result, err
	}
	var rows []loginLogPO
	if err := page(query.Order("id DESC"), f.PageQuery).Find(&rows).Error; err != nil {
		return result, err
	}
	result.List = make([]domain.LoginLog, 0, len(rows))
	for _, row := range rows {
		result.List = append(result.List, domain.LoginLog{ID: row.ID, TenantID: row.TenantID, LogType: row.LogType,
			TraceID: row.TraceID, UserID: row.UserID, UserType: row.UserType, Username: row.Username,
			Result: row.Result, UserIP: row.UserIP, UserAgent: row.UserAgent, CreateTime: row.CreateTime})
	}
	return result, nil
}
