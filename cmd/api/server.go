package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	iamcache "oneweekonething.com/owot-pro/internal/iam/adapter/cache"
	iampersistence "oneweekonething.com/owot-pro/internal/iam/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	systemhttp "oneweekonething.com/owot-pro/internal/system/adapter/http"
	"oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/system/app"
)

type application struct {
	handler     http.Handler
	relay       *outbox.Store
	permissions *iamcache.Permissions
	close       func() error
}

func newApplication(ctx context.Context, migrate, seed bool) (*application, error) {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	dsn, redisAddr := os.Getenv("OWOT_MYSQL_DSN"), os.Getenv("OWOT_REDIS_ADDR")
	if dsn == "" && redisAddr == "" {
		if migrate || seed {
			return nil, errors.New("-migrate/-seed require OWOT_MYSQL_DSN and OWOT_REDIS_ADDR")
		}
		log.Info("IAM and System are disabled; set OWOT_MYSQL_DSN and OWOT_REDIS_ADDR to enable them")
		return &application{handler: newHandler(), close: func() error { return nil }}, nil
	}
	if dsn == "" || redisAddr == "" {
		return nil, errors.New("OWOT_MYSQL_DSN and OWOT_REDIS_ADDR must be configured together")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil || config.DBName == "" {
		return nil, errors.New("invalid OWOT_MYSQL_DSN configuration")
	}
	config.ParseTime, config.Loc = true, time.UTC
	config.Timeout, config.ReadTimeout, config.WriteTimeout = 3*time.Second, 5*time.Second, 5*time.Second
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: config.FormatDSN(), SkipInitializeWithVersion: true}), &gorm.Config{
		TranslateError: true, Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open MySQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	client := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("OWOT_REDIS_PASSWORD"),
		DialTimeout: 3 * time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, ContextTimeoutEnabled: true})
	closeDependencies := func() error { return errors.Join(client.Close(), sqlDB.Close()) }
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(readyCtx); err != nil {
		_ = closeDependencies()
		return nil, fmt.Errorf("ping MySQL: %w", err)
	}
	if err := client.Ping(readyCtx).Err(); err != nil {
		_ = closeDependencies()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	if err := dbx.Install(db); err != nil {
		_ = closeDependencies()
		return nil, err
	}
	if migrate {
		if err := errors.Join(persistence.Migrate(db.WithContext(ctx)), iampersistence.Migrate(db.WithContext(ctx)), outbox.Migrate(db.WithContext(ctx))); err != nil {
			_ = closeDependencies()
			return nil, fmt.Errorf("migrate backend schema: %w", err)
		}
	}
	if err := errors.Join(persistence.CheckSchema(db.WithContext(ctx)), iampersistence.CheckSchema(db.WithContext(ctx))); err != nil {
		_ = closeDependencies()
		return nil, err
	}
	store := outbox.NewStore(db, iamcache.NewBroadcaster(client), log)
	deps := httpx.Dependencies{Logger: log, AllowedOrigins: strings.Split(os.Getenv("OWOT_CORS_ORIGINS"), ",")}
	handler, iam, permissions, err := newBackendHandler(db, store, client, deps, os.Getenv("OWOT_AVATAR_DIR"))
	if err != nil {
		_ = closeDependencies()
		return nil, err
	}
	closeBackend := func() error { permissions.Close(); return closeDependencies() }
	if seed {
		repo := persistence.NewDictRepository(db)
		dicts := app.NewDictService(repo, repo, dbx.NewTxManager(db), store)
		if err := dicts.SeedDefaults(ctx); err != nil {
			_ = closeBackend()
			return nil, fmt.Errorf("seed System dictionaries: %w", err)
		}
		if err := iam.SeedDefaults(ctx); err != nil {
			_ = closeBackend()
			return nil, fmt.Errorf("seed IAM defaults: %w", err)
		}
	}
	return &application{handler: handler, relay: store, permissions: permissions, close: closeBackend}, nil
}

func newSystemHandler(db *gorm.DB, store *outbox.Store, deps httpx.Dependencies, users app.UserNicknameReader) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	dicts := persistence.NewDictRepository(db)
	notices := persistence.NewNoticeRepository(db)
	logs := persistence.NewAuditLogRepository(db)
	tx := dbx.NewTxManager(db)
	dictService := app.NewDictService(dicts, dicts, tx, store)
	noticeService := app.NewNoticeService(notices)
	auditService := app.NewAuditLogService(logs, logs, users)
	deps.Operations = systemhttp.NewOperationRecorder(auditService)
	router := httpx.NewRouter(deps)
	router.Handle(http.MethodGet, "/actuator/health", httpx.Route{PermitAll: true, TenantIgnore: true}, health)
	systemhttp.NewHandler(dictService, noticeService, auditService, deps.Logger).Register(router)
	return router.Engine
}

func health(c *gin.Context) { httpx.Success(c, gin.H{"status": "UP"}) }
