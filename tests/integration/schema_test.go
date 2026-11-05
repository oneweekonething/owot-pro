package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

func realSchema(t *testing.T, prefix string, migrations ...string) (*gorm.DB, *redis.Client) {
	t.Helper()
	base, client := realSystemDependencies(t)
	// An isolated schema proves the checked-in DDL itself, independently of
	// AutoMigrate or earlier runs. The test DSN needs CREATE/DROP DATABASE rights.
	name := fmt.Sprintf("owot_%s_%d", prefix, time.Now().UnixNano())
	if err := base.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
			t.Error(err)
		}
	})
	config, err := mysqldriver.ParseDSN(os.Getenv("OWOT_TEST_MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid test DSN")
	}
	config.DBName, config.ParseTime, config.Loc = name, true, time.UTC
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() }})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := dbx.Install(db); err != nil {
		t.Fatal(err)
	}
	for _, filename := range migrations {
		ddl, err := os.ReadFile("../../migrations/mysql/" + filename)
		if err != nil {
			t.Fatal(err)
		}
		lines := []string{}
		for _, line := range strings.Split(string(ddl), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				lines = append(lines, line)
			}
		}
		for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
			if strings.TrimSpace(statement) != "" {
				if err := db.Exec(statement).Error; err != nil {
					t.Fatalf("apply %s: %v", filename, err)
				}
			}
		}
	}
	return db, client
}
