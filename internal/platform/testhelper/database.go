// Package testhelper manages isolated database dependencies for adapter tests.
package testhelper

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

func SQLite(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:owot-%d?mode=memory&cache=shared&_foreign_keys=on", time.Now().UnixNano())), &gorm.Config{
		TranslateError: true, Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Serialize SQLite connections; MySQL integration tests cover real row locking.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := dbx.Install(db); err != nil {
		t.Fatal(err)
	}
	return db
}
