package persistence

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type repository[T, P any] struct {
	db                  *gorm.DB
	tenantScoped        bool
	order               string
	missing             error
	toPO                func(T) P
	toDomain            func(P) T
	base                func(*P) *dbx.BasePO
	active, clearActive func(*P)
	filter              func(*gorm.DB, app.Query) *gorm.DB
	afterSave           func(context.Context, *P) error
}

func (r *repository[T, P]) current(ctx context.Context, db *gorm.DB) *gorm.DB {
	// Current reads after the tenant lock include changes committed while a
	// global package transaction was waiting for that tenant. Global catalog
	// reads stay non-locking to preserve the global -> tenant lock order.
	if r.tenantScoped && dbx.InTransaction(ctx) {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return db
}

func active(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func translate(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return domain.ErrDuplicate
	}
	return err
}

func (r *repository[T, P]) Get(ctx context.Context, id int64) (T, error) {
	var p P
	var zero T
	if id <= 0 {
		return zero, domain.ErrInvalidArgument
	}
	err := r.current(ctx, dbx.FromContext(ctx, r.db)).First(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return zero, r.missing
	}
	if err != nil {
		return zero, fmt.Errorf("find IAM record: %w", err)
	}
	return r.toDomain(p), nil
}
func (r *repository[T, P]) List(ctx context.Context, q app.Query) (pagination.PageResult[T], error) {
	result := pagination.PageResult[T]{List: []T{}}
	var model P
	db := dbx.FromContext(ctx, r.db).Model(&model)
	db = r.filter(db, q)
	if q.Status != nil {
		db = db.Where("status = ?", *q.Status)
	}
	if q.IDs != nil {
		db = db.Where("id IN ?", q.IDs)
	}
	if q.From != nil {
		db = db.Where("create_time >= ?", *q.From)
	}
	if q.To != nil {
		db = db.Where("create_time <= ?", *q.To)
	}
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	page := q.Normalize()
	if page.PageSize != -1 {
		db = db.Offset((page.PageNo - 1) * page.PageSize).Limit(page.PageSize)
	}
	var rows []P
	order := r.order
	if order == "" {
		order = "id ASC"
	}
	if err := r.current(ctx, db).Order(order).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, p := range rows {
		result.List = append(result.List, r.toDomain(p))
	}
	if page.PageSize == -1 {
		result.Total = int64(len(result.List))
	}
	return result, nil
}
func (r *repository[T, P]) Save(ctx context.Context, value *T) error {
	p := r.toPO(*value)
	r.active(&p)
	db := dbx.FromContext(ctx, r.db)
	if r.base(&p).ID == 0 {
		if err := translate(db.Create(&p).Error); err != nil {
			return fmt.Errorf("create IAM record: %w", err)
		}
	} else {
		var old P
		err := r.current(ctx, db).First(&old, r.base(&p).ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.missing
		}
		if err != nil {
			return err
		}
		r.base(&p).CreateTime, r.base(&p).Creator = r.base(&old).CreateTime, r.base(&old).Creator
		if err := translate(db.Model(&old).Select("*").Omit("id", "tenant_id", "creator", "create_time", "deleted").Updates(&p).Error); err != nil {
			return fmt.Errorf("update IAM record: %w", err)
		}
		// GORM omits tenant_id on updates; return the stored tenant identity as well.
		if err := db.First(&p, r.base(&p).ID).Error; err != nil {
			return err
		}
	}
	if r.afterSave != nil {
		if err := r.afterSave(ctx, &p); err != nil {
			return err
		}
	}
	*value = r.toDomain(p)
	return nil
}

// Create also accepts an explicit ID for the documented bootstrap identities.
func (r *repository[T, P]) Create(ctx context.Context, value *T) error {
	p := r.toPO(*value)
	r.active(&p)
	if err := translate(dbx.FromContext(ctx, r.db).Create(&p).Error); err != nil {
		return fmt.Errorf("create IAM record: %w", err)
	}
	if r.afterSave != nil {
		if err := r.afterSave(ctx, &p); err != nil {
			return err
		}
	}
	*value = r.toDomain(p)
	return nil
}
func (r *repository[T, P]) Delete(ctx context.Context, id int64) error {
	var row P
	db := dbx.FromContext(ctx, r.db)
	if err := r.current(ctx, db).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.missing
		}
		return err
	}
	r.clearActive(&row)
	if err := db.Model(&row).Select("*").Omit("id", "tenant_id", "creator", "create_time", "deleted").Updates(&row).Error; err != nil {
		return err
	}
	return db.Delete(&row).Error
}

type Locker struct{ db *gorm.DB }

func NewLocker(db *gorm.DB) *Locker                    { return &Locker{db: db} }
func (l *Locker) LockGlobal(ctx context.Context) error { return l.LockTenant(ctx, 1) }
func (l *Locker) LockTenant(ctx context.Context, id int64) error {
	if !dbx.InTransaction(ctx) {
		return errors.New("iam: mutation lock requires transaction")
	}
	var t tenantPO
	err := dbx.FromContext(ctx, l.db).Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrTenantNotFound
	}
	return err
}

type userRolePO struct {
	dbx.TenantBasePO
	UserID int64 `gorm:"not null;index:idx_iam_user_role_user"`
	RoleID int64 `gorm:"not null;index:idx_iam_user_role_role"`
}

func (userRolePO) TableName() string { return "system_user_role" }

type roleMenuPO struct {
	dbx.TenantBasePO
	RoleID int64 `gorm:"not null;index:idx_iam_role_menu,priority:1"`
	MenuID int64 `gorm:"not null;index:idx_iam_role_menu,priority:2"`
}

func (roleMenuPO) TableName() string { return "system_role_menu" }

type userPostPO struct {
	dbx.TenantBasePO
	UserID int64 `gorm:"not null;index:idx_iam_user_post,priority:1"`
	PostID int64 `gorm:"not null;index:idx_iam_user_post,priority:2"`
}

func (userPostPO) TableName() string { return "system_user_post" }

type Associations struct{ db *gorm.DB }

func NewAssociations(db *gorm.DB) *Associations { return &Associations{db: db} }
func (a *Associations) UserRoles(ctx context.Context, id int64) ([]int64, error) {
	ids := []int64{}
	db := dbx.FromContext(ctx, a.db)
	if dbx.InTransaction(ctx) {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := db.Model(&userRolePO{}).Where("user_id = ?", id).Order("role_id").Pluck("role_id", &ids).Error
	return ids, err
}
func (a *Associations) RoleMenus(ctx context.Context, id int64) ([]int64, error) {
	ids := []int64{}
	db := dbx.FromContext(ctx, a.db)
	if dbx.InTransaction(ctx) {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := db.Model(&roleMenuPO{}).Where("role_id = ?", id).Order("menu_id").Pluck("menu_id", &ids).Error
	return ids, err
}
func (a *Associations) AssignUserRoles(ctx context.Context, id int64, ids []int64) error {
	if !dbx.InTransaction(ctx) {
		return errors.New("iam: association write requires transaction")
	}
	db := dbx.FromContext(ctx, a.db)
	if err := db.Where("user_id = ?", id).Delete(&userRolePO{}).Error; err != nil {
		return err
	}
	for _, role := range domain.UniqueIDs(ids) {
		if err := db.Create(&userRolePO{UserID: id, RoleID: role}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (a *Associations) AssignRoleMenus(ctx context.Context, id int64, ids []int64) error {
	if !dbx.InTransaction(ctx) {
		return errors.New("iam: association write requires transaction")
	}
	current, err := a.RoleMenus(ctx, id)
	if err != nil {
		return err
	}
	added, removed := domain.MenuDiff(current, ids)
	db := dbx.FromContext(ctx, a.db)
	if len(removed) > 0 {
		if err := db.Where("role_id = ? AND menu_id IN ?", id, removed).Delete(&roleMenuPO{}).Error; err != nil {
			return err
		}
	}
	for _, menu := range added {
		if err := db.Create(&roleMenuPO{RoleID: id, MenuID: menu}).Error; err != nil {
			return err
		}
	}
	return nil
}
func syncUserPosts(ctx context.Context, db *gorm.DB, id int64, ids []int64) error {
	db = dbx.FromContext(ctx, db)
	if err := db.Where("user_id = ?", id).Delete(&userPostPO{}).Error; err != nil {
		return err
	}
	for _, post := range domain.UniqueIDs(ids) {
		if err := db.Create(&userPostPO{UserID: id, PostID: post}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (a *Associations) RemoveReferences(ctx context.Context, kind string, id int64) error {
	db := dbx.FromContext(ctx, a.db)
	switch kind {
	case "user":
		return errors.Join(db.Where("user_id = ?", id).Delete(&userRolePO{}).Error, db.Where("user_id = ?", id).Delete(&userPostPO{}).Error)
	case "role":
		return errors.Join(db.Where("role_id = ?", id).Delete(&userRolePO{}).Error, db.Where("role_id = ?", id).Delete(&roleMenuPO{}).Error)
	case "menu":
		return db.Where("menu_id = ?", id).Delete(&roleMenuPO{}).Error
	case "tenant":
		if ctxmeta.TenantID(ctx) != id {
			return domain.ErrForbidden
		}
		return errors.Join(db.Where("tenant_id = ?", id).Delete(&userRolePO{}).Error, db.Where("tenant_id = ?", id).Delete(&roleMenuPO{}).Error, db.Where("tenant_id = ?", id).Delete(&userPostPO{}).Error)
	default:
		return domain.ErrInvalidArgument
	}
}
