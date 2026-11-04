// Package dbx supplies transaction propagation and tenant isolation for GORM adapters.
package dbx

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/soft_delete"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

var ErrTenantRequired = errors.New("dbx: tenant context required")
var ErrTenantMismatch = errors.New("dbx: tenant does not match context")

type BasePO struct {
	ID         int64                 `gorm:"primaryKey;autoIncrement"`
	Creator    string                `gorm:"size:64;not null;default:''"`
	CreateTime time.Time             `gorm:"autoCreateTime;index"`
	Updater    string                `gorm:"size:64;not null;default:''"`
	UpdateTime time.Time             `gorm:"autoUpdateTime"`
	Deleted    soft_delete.DeletedAt `gorm:"softDelete:flag;not null;default:0"`
}

type TenantBasePO struct {
	BasePO
	TenantID int64 `gorm:"not null;index"`
}

type txKey struct{}

type transaction struct {
	db       *gorm.DB
	hooks    []func(context.Context)
	hookKeys map[any]bool
}

func FromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*transaction); ok {
		return tx.db.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

func InTransaction(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(*transaction)
	return ok
}

func AfterCommit(ctx context.Context, fn func(context.Context)) {
	if tx, ok := ctx.Value(txKey{}).(*transaction); ok {
		tx.hooks = append(tx.hooks, fn)
	}
}

// AfterCommitOnce coalesces repeated notifications within a batch transaction.
// key must be comparable, such as the notifying store pointer.
func AfterCommitOnce(ctx context.Context, key any, fn func(context.Context)) {
	if tx, ok := ctx.Value(txKey{}).(*transaction); ok {
		if tx.hookKeys == nil {
			tx.hookKeys = make(map[any]bool)
		}
		if !tx.hookKeys[key] {
			tx.hookKeys[key] = true
			tx.hooks = append(tx.hooks, fn)
		}
	}
}

type TxManager struct{ db *gorm.DB }

func NewTxManager(db *gorm.DB) *TxManager { return &TxManager{db: db} }

// WithinTx joins an existing transaction; the outermost caller owns commit and hooks.
func (m *TxManager) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if InTransaction(ctx) {
		return fn(ctx)
	}
	tx := &transaction{}
	err := m.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		tx.db = db
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
	if err == nil {
		for _, hook := range tx.hooks {
			hook(ctxmeta.CloneContext(ctx))
		}
	}
	return err
}

// Install registers fail-closed tenant checks on schema-based GORM operations.
// Raw SQL must remain inside trusted adapters and include explicit tenant predicates.
func Install(db *gorm.DB) error {
	callbacks := []struct {
		register func(string, func(*gorm.DB)) error
		fn       func(*gorm.DB)
	}{
		{db.Callback().Query().Before("gorm:query").Register, scopeTenant},
		{db.Callback().Row().Before("gorm:row").Register, scopeTenant},
		{db.Callback().Update().Before("gorm:update").Register, scopeTenantUpdate},
		{db.Callback().Delete().Before("gorm:delete").Register, scopeTenant},
		{db.Callback().Create().Before("gorm:create").Register, createTenant},
	}
	for _, cb := range callbacks {
		if err := cb.register("owot:tenant", cb.fn); err != nil {
			return err
		}
	}
	if err := db.Callback().Create().Before("gorm:create").Register("owot:audit", auditCreate); err != nil {
		return err
	}
	return db.Callback().Update().Before("gorm:update").Register("owot:audit", auditUpdate)
}

func tenantID(db *gorm.DB) (int64, bool) {
	if db.Statement.Schema == nil || db.Statement.Schema.LookUpField("TenantID") == nil || ctxmeta.IsIgnoreTenant(db.Statement.Context) {
		return 0, false
	}
	switch db.Statement.Schema.Table {
	case "system_menu", "system_tenant", "system_tenant_package", "system_oauth2_client",
		"system_dict_type", "system_dict_data", "infra_config", "infra_file_config",
		"infra_file", "infra_file_content", "infra_job", "infra_job_log", "sys_outbox_event":
		return 0, false
	}
	id := ctxmeta.TenantID(db.Statement.Context)
	if id <= 0 {
		_ = db.AddError(ErrTenantRequired)
		return 0, false
	}
	return id, true
}

func scopeTenant(db *gorm.DB) {
	if id, ok := tenantID(db); ok {
		db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{
			Column: clause.Column{Table: clause.CurrentTable, Name: "tenant_id"}, Value: id,
		}}})
	}
}

func scopeTenantUpdate(db *gorm.DB) {
	scopeTenant(db)
	// TenantID is immutable on updates, including map-based updates.
	if db.Statement.Schema != nil && db.Statement.Schema.LookUpField("TenantID") != nil {
		db.Statement.Omits = append(db.Statement.Omits, "tenant_id", "TenantID")
	}
}

func createTenant(db *gorm.DB) {
	id, ok := tenantID(db)
	if !ok {
		return
	}
	field := db.Statement.Schema.LookUpField("TenantID")
	value := db.Statement.ReflectValue
	// SetColumn handles both a single record and a batch after validating each value.
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		for i := 0; i < value.Len(); i++ {
			current, _ := field.ValueOf(db.Statement.Context, value.Index(i))
			if tid, _ := current.(int64); tid != 0 && tid != id {
				_ = db.AddError(ErrTenantMismatch)
				return
			}
		}
	} else {
		current, _ := field.ValueOf(db.Statement.Context, value)
		if tid, _ := current.(int64); tid != 0 && tid != id {
			_ = db.AddError(ErrTenantMismatch)
			return
		}
	}
	db.Statement.SetColumn("TenantID", id)
}

func auditCreate(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}
	if user, ok := ctxmeta.User(db.Statement.Context); ok {
		if db.Statement.Schema.LookUpField("Creator") != nil {
			db.Statement.SetColumn("Creator", strconv.FormatInt(user.ID, 10))
		}
		if db.Statement.Schema.LookUpField("Updater") != nil {
			db.Statement.SetColumn("Updater", strconv.FormatInt(user.ID, 10))
		}
	}
}

func auditUpdate(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}
	if user, ok := ctxmeta.User(db.Statement.Context); ok && db.Statement.Schema.LookUpField("Updater") != nil {
		db.Statement.SetColumn("Updater", strconv.FormatInt(user.ID, 10))
	}
}
