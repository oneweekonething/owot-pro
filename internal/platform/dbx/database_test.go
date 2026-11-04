package dbx_test

import (
	"context"
	"errors"
	"testing"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/testhelper"
)

type tenantRecord struct {
	dbx.TenantBasePO
	Name string
}

func TestTenantIsolationAndAuditFields(t *testing.T) {
	db := testhelper.SQLite(t)
	if err := db.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}
	a := ctxmeta.WithLoginUser(ctxmeta.WithTenantID(context.Background(), 1), ctxmeta.LoginUser{ID: 7, TenantID: 1})
	b := ctxmeta.WithTenantID(context.Background(), 2)
	row := tenantRecord{Name: "tenant-a"}
	if err := db.WithContext(a).Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.TenantID != 1 || row.Creator != "7" || row.Updater != "7" || row.CreateTime.IsZero() {
		t.Fatalf("missing tenant/audit fields: %+v", row)
	}
	if err := db.WithContext(a).Create(&tenantRecord{TenantBasePO: dbx.TenantBasePO{TenantID: 2}}).Error; !errors.Is(err, dbx.ErrTenantMismatch) {
		t.Fatalf("cross-tenant create = %v", err)
	}
	batch := []tenantRecord{{Name: "a"}, {TenantBasePO: dbx.TenantBasePO{TenantID: 2}, Name: "b"}}
	if err := db.WithContext(a).Create(&batch).Error; !errors.Is(err, dbx.ErrTenantMismatch) {
		t.Fatalf("cross-tenant batch = %v", err)
	}
	for _, ctx := range []context.Context{context.Background(), b} {
		var rows []tenantRecord
		err := db.WithContext(ctx).Find(&rows).Error
		if ctxmeta.TenantID(ctx) == 0 && !errors.Is(err, dbx.ErrTenantRequired) {
			t.Fatalf("missing tenant query = %v", err)
		}
		if len(rows) != 0 {
			t.Fatal("tenant records leaked")
		}
	}
	if result := db.WithContext(b).Model(&tenantRecord{}).Where("id = ?", row.ID).Update("name", "hijacked"); result.Error != nil || result.RowsAffected != 0 {
		t.Fatalf("cross-tenant update = %+v", result)
	}
	if err := db.WithContext(a).Model(&tenantRecord{}).Where("id = ?", row.ID).Updates(map[string]any{"tenant_id": 2, "name": "updated"}).Error; err != nil {
		t.Fatal(err)
	}
	var found tenantRecord
	if err := db.WithContext(a).First(&found, row.ID).Error; err != nil || found.TenantID != 1 || found.Name != "updated" {
		t.Fatalf("tenant ID mutation or update lost: %+v, %v", found, err)
	}
	if err := db.WithContext(context.Background()).Delete(&tenantRecord{}, row.ID).Error; !errors.Is(err, dbx.ErrTenantRequired) {
		t.Fatalf("missing tenant delete = %v", err)
	}
	if err := db.WithContext(a).Delete(&found).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.WithContext(a).Model(&tenantRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("soft deletion failed: count=%d, err=%v", count, err)
	}
	if err := db.WithContext(ctxmeta.WithIgnoreTenant(context.Background())).Unscoped().Model(&tenantRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("trusted maintenance read failed: %d, %v", count, err)
	}
}

func TestTransactionPropagationAndCommitHooks(t *testing.T) {
	db := testhelper.SQLite(t)
	if err := db.AutoMigrate(&tenantRecord{}); err != nil {
		t.Fatal(err)
	}
	ctx := ctxmeta.WithTenantID(context.Background(), 1)
	tx := dbx.NewTxManager(db)
	want := errors.New("abort")
	called := 0
	fn := func(ctx context.Context) error {
		dbx.AfterCommit(ctx, func(ctx context.Context) {
			if dbx.InTransaction(ctx) || ctxmeta.TenantID(ctx) != 1 {
				t.Error("commit hook retained a transaction or lost metadata")
			}
			called++
		})
		return tx.WithinTx(ctx, func(ctx context.Context) error {
			if !dbx.InTransaction(ctx) {
				t.Fatal("nested transaction context lost")
			}
			return dbx.FromContext(ctx, db).Create(&tenantRecord{Name: "row"}).Error
		})
	}
	if err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return want
	}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&tenantRecord{}).Count(&count).Error; err != nil || count != 0 || called != 0 {
		t.Fatalf("rollback failed: count=%d hooks=%d err=%v", count, called, err)
	}
	if err := tx.WithinTx(ctx, fn); err != nil || called != 1 {
		t.Fatalf("commit hook: called=%d err=%v", called, err)
	}
}
