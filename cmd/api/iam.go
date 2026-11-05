package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/iam/adapter/avatar"
	iamcache "oneweekonething.com/owot-pro/internal/iam/adapter/cache"
	iamhttp "oneweekonething.com/owot-pro/internal/iam/adapter/http"
	iampersistence "oneweekonething.com/owot-pro/internal/iam/adapter/persistence"
	"oneweekonething.com/owot-pro/internal/iam/adapter/security"
	iamapp "oneweekonething.com/owot-pro/internal/iam/app"
	iamdomain "oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/outbox"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	systemhttp "oneweekonething.com/owot-pro/internal/system/adapter/http"
	systempersistence "oneweekonething.com/owot-pro/internal/system/adapter/persistence"
	systemapp "oneweekonething.com/owot-pro/internal/system/app"
	systemdomain "oneweekonething.com/owot-pro/internal/system/domain"
)

func iamRepositories(db *gorm.DB) iamapp.Repositories {
	return iamapp.Repositories{Users: iampersistence.NewUserRepository(db), Depts: iampersistence.NewDeptRepository(db), Posts: iampersistence.NewPostRepository(db), Roles: iampersistence.NewRoleRepository(db), Menus: iampersistence.NewMenuRepository(db), Tenants: iampersistence.NewTenantRepository(db), Packages: iampersistence.NewPackageRepository(db), Clients: iampersistence.NewClientRepository(db), Links: iampersistence.NewAssociations(db), Tokens: iampersistence.NewTokenRepository(db), Locks: iampersistence.NewLocker(db)}
}

func newBackendHandler(db *gorm.DB, store *outbox.Store, client redis.UniversalClient, deps httpx.Dependencies, avatarDirectory string) (http.Handler, *iamapp.Service, *iamcache.Permissions, error) {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	repos := iamRepositories(db)
	tx := dbx.NewTxManager(db)
	dictRepo := systempersistence.NewDictRepository(db)
	dicts := systemapp.NewDictService(dictRepo, dictRepo, tx, store)
	notices := systemapp.NewNoticeService(systempersistence.NewNoticeRepository(db))
	logs := systempersistence.NewAuditLogRepository(db)
	audit := systemapp.NewAuditLogService(logs, logs, &iamNicknameReader{users: repos.Users})
	permissions, err := iamcache.NewPermissions(client)
	if err != nil {
		return nil, nil, nil, err
	}
	if avatarDirectory == "" {
		avatarDirectory = filepath.Join("data", "iam-avatars")
	}
	service := iamapp.NewService(iamapp.Dependencies{Repositories: repos, Tx: tx, Outbox: store, Hasher: security.NewPasswordHasher(), TokenCache: iamcache.NewTokens(client, deps.Logger), Permissions: permissions, LoginLogs: &iamLoginRecorder{audit: audit}, Avatars: &avatar.Local{Directory: avatarDirectory}})
	iamHandler := iamhttp.NewHandler(service, deps.Logger, systemhttp.NewDictionaryTranslator(dicts))
	deps.Tokens, deps.Permissions, deps.Tenants, deps.DataPermissions = iamHandler, service, service, service
	deps.Operations = systemhttp.NewOperationRecorder(audit)
	router := httpx.NewRouter(deps)
	router.Handle(http.MethodGet, "/actuator/health", httpx.Route{PermitAll: true, TenantIgnore: true}, health)
	systemhttp.NewHandler(dicts, notices, audit, deps.Logger).Register(router)
	iamHandler.Register(router)
	avatarName := regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpeg|gif)$`)
	router.Handle(http.MethodGet, "/admin-api/system/user/profile/avatar/:name", httpx.Route{PermitAll: true, TenantIgnore: true}, func(c *gin.Context) {
		name := c.Param("name")
		if !avatarName.MatchString(name) {
			httpx.Failure(c, 404, "头像不存在")
			return
		}
		path := filepath.Join(avatarDirectory, name)
		if _, err := os.Stat(path); err != nil {
			httpx.Failure(c, 404, "头像不存在")
			return
		}
		c.Header("X-Content-Type-Options", "nosniff")
		c.File(path)
	})
	return router.Engine, service, permissions, nil
}

// Bridges live at the composition root: neither application imports another domain.
type iamLoginRecorder struct{ audit *systemapp.AuditLogService }

func (r *iamLoginRecorder) RecordLogin(ctx context.Context, v iamapp.LoginLog) error {
	agent := []rune(v.UserAgent)
	if len(agent) > systemdomain.MaxUserAgentLength {
		agent = agent[:systemdomain.MaxUserAgentLength]
	}
	return r.audit.RecordLoginLog(ctx, systemdomain.LoginLog{UserID: v.UserID, UserType: systemdomain.UserTypeAdmin, Username: v.Username, UserIP: v.UserIP, UserAgent: string(agent), LogType: v.LogType, Result: v.Result})
}

type iamNicknameReader struct {
	users iamapp.Repository[iamdomain.AdminUser]
}

func (r *iamNicknameReader) Nicknames(ctx context.Context, ids []int64) (map[int64]string, error) {
	page, err := r.users.List(ctxmeta.WithoutDataScope(ctx), iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}, IDs: ids})
	result := map[int64]string{}
	for _, u := range page.List {
		result[u.ID] = u.Nickname
	}
	return result, err
}
func (r *iamNicknameReader) FindUserIDsByNickname(ctx context.Context, name string) ([]int64, error) {
	page, err := r.users.List(ctxmeta.WithoutDataScope(ctx), iamapp.Query{PageQuery: pagination.PageQuery{PageSize: -1}, Name: name})
	result := []int64{}
	for _, u := range page.List {
		result = append(result, u.ID)
	}
	return result, err
}
