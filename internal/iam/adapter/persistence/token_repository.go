package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type accessTokenPO struct {
	dbx.TenantBasePO
	UserID       int64 `gorm:"not null;index"`
	UserType     int
	ClientID     string    `gorm:"size:255;not null"`
	AccessToken  string    `gorm:"size:255;not null;uniqueIndex:uk_iam_access_token"`
	RefreshToken string    `gorm:"size:255;not null;index"`
	ExpiresTime  time.Time `gorm:"not null;index"`
}

func (accessTokenPO) TableName() string { return "system_oauth2_access_token" }

type refreshTokenPO struct {
	dbx.TenantBasePO
	UserID       int64 `gorm:"not null;index"`
	UserType     int
	ClientID     string    `gorm:"size:255;not null"`
	RefreshToken string    `gorm:"size:255;not null;uniqueIndex:uk_iam_refresh_token"`
	ExpiresTime  time.Time `gorm:"not null;index"`
}

func (refreshTokenPO) TableName() string { return "system_oauth2_refresh_token" }
func accessDomain(p accessTokenPO) domain.AccessToken {
	return domain.AccessToken{ID: p.ID, TenantID: p.TenantID, UserID: p.UserID, UserType: p.UserType, ClientID: p.ClientID, AccessToken: p.AccessToken, RefreshToken: p.RefreshToken, CreateTime: p.CreateTime, ExpiresTime: p.ExpiresTime}
}
func refreshDomain(p refreshTokenPO) domain.RefreshToken {
	return domain.RefreshToken{ID: p.ID, TenantID: p.TenantID, UserID: p.UserID, UserType: p.UserType, ClientID: p.ClientID, RefreshToken: p.RefreshToken, CreateTime: p.CreateTime, ExpiresTime: p.ExpiresTime}
}

type TokenRepository struct{ db *gorm.DB }

func NewTokenRepository(db *gorm.DB) *TokenRepository { return &TokenRepository{db: db} }
func (r *TokenRepository) FindAccess(ctx context.Context, selector app.AccessSelector) (domain.AccessToken, error) {
	var p accessTokenPO
	query := dbx.FromContext(ctx, r.db)
	if selector.ID > 0 {
		query = query.Where("id = ?", selector.ID)
	} else if selector.Token != "" {
		query = query.Where("access_token = ?", selector.Token)
	} else {
		return domain.AccessToken{}, domain.ErrTokenInvalid
	}
	err := query.First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.AccessToken{}, domain.ErrTokenInvalid
	}
	return accessDomain(p), err
}
func (r *TokenRepository) FindRefresh(ctx context.Context, token string) (domain.RefreshToken, error) {
	var p refreshTokenPO
	db := dbx.FromContext(ctx, r.db)
	if dbx.InTransaction(ctx) {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := db.Where("refresh_token = ?", token).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.RefreshToken{}, domain.ErrTokenInvalid
	}
	return refreshDomain(p), err
}
func (r *TokenRepository) SaveTokens(ctx context.Context, a *domain.AccessToken, b *domain.RefreshToken) error {
	if !dbx.InTransaction(ctx) {
		return errors.New("iam: token write requires transaction")
	}
	p := accessTokenPO{UserID: a.UserID, UserType: a.UserType, ClientID: a.ClientID, AccessToken: a.AccessToken, RefreshToken: a.RefreshToken, ExpiresTime: a.ExpiresTime}
	q := refreshTokenPO{UserID: b.UserID, UserType: b.UserType, ClientID: b.ClientID, RefreshToken: b.RefreshToken, ExpiresTime: b.ExpiresTime}
	db := dbx.FromContext(ctx, r.db)
	if err := db.Create(&p).Error; err != nil {
		return err
	}
	if err := db.Create(&q).Error; err != nil {
		return err
	}
	*a, *b = accessDomain(p), refreshDomain(q)
	return nil
}
func (r *TokenRepository) Revoke(ctx context.Context, s app.TokenSelection) ([]domain.AccessToken, error) {
	if !dbx.InTransaction(ctx) {
		return nil, errors.New("iam: revocation requires transaction")
	}
	db := dbx.FromContext(ctx, r.db)
	query := db.Model(&accessTokenPO{})
	if s.UserID > 0 {
		query = query.Where("user_id = ?", s.UserID)
	}
	if s.AccessToken != "" {
		query = query.Where("access_token = ?", s.AccessToken)
	}
	if s.RefreshToken != "" {
		query = query.Where("refresh_token = ?", s.RefreshToken)
	}
	var rows []accessTokenPO
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := []domain.AccessToken{}
	ids := []int64{}
	refresh := []string{}
	for _, p := range rows {
		result = append(result, accessDomain(p))
		ids = append(ids, p.ID)
		refresh = append(refresh, p.RefreshToken)
	}
	if len(ids) > 0 {
		if err := db.Where("id IN ?", ids).Delete(&accessTokenPO{}).Error; err != nil {
			return nil, err
		}
	}
	refreshQuery := db.Model(&refreshTokenPO{})
	switch {
	case s.RefreshToken != "":
		refreshQuery = refreshQuery.Where("refresh_token = ?", s.RefreshToken)
	case s.AccessToken != "":
		if len(refresh) == 0 {
			return result, nil
		}
		refreshQuery = refreshQuery.Where("refresh_token IN ?", refresh)
	case s.UserID > 0:
		refreshQuery = refreshQuery.Where("user_id = ?", s.UserID)
	default:
		refreshQuery = refreshQuery.Where("1 = 1")
	}
	return result, refreshQuery.Delete(&refreshTokenPO{}).Error
}
func (r *TokenRepository) PageTokens(ctx context.Context, q app.Query) (pagination.PageResult[domain.AccessToken], error) {
	result := pagination.PageResult[domain.AccessToken]{List: []domain.AccessToken{}}
	db := dbx.FromContext(ctx, r.db).Model(&accessTokenPO{}).Where("expires_time > ?", time.Now())
	if q.ClientID != "" {
		db = db.Where("client_id = ?", q.ClientID)
	}
	if q.UserID != nil {
		db = db.Where("user_id = ?", *q.UserID)
	}
	if q.UserType != nil {
		db = db.Where("user_type = ?", *q.UserType)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	p := q.Normalize()
	if p.PageSize != -1 {
		db = db.Offset((p.PageNo - 1) * p.PageSize).Limit(p.PageSize)
	}
	var rows []accessTokenPO
	if err := db.Order("id DESC").Find(&rows).Error; err != nil {
		return result, err
	}
	for _, p := range rows {
		result.List = append(result.List, accessDomain(p))
	}
	return result, nil
}
