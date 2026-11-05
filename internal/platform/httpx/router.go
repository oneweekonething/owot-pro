// Package httpx implements HTTP contracts and fail-closed route guards.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

// ErrUnauthenticated distinguishes an invalid credential from a dependency failure.
var ErrUnauthenticated = errors.New("httpx: unauthenticated")

type TokenVerifier interface {
	Verify(context.Context, string) (ctxmeta.LoginUser, error)
}

type PermissionChecker interface {
	HasPermission(context.Context, ctxmeta.LoginUser, string) (bool, error)
}

type TenantValidator interface {
	ValidateTenant(context.Context, int64) error
}

type DataPermissionProvider interface {
	DataScope(context.Context, ctxmeta.LoginUser) (ctxmeta.DataScope, error)
}

type OperateLog struct {
	Type, SubType, Action     string
	BizID                     int64
	RequestMethod, RequestURL string
	UserIP, UserAgent         string
}

type OperateLogRecorder interface {
	Record(context.Context, OperateLog) error
}

type Dependencies struct {
	Tokens          TokenVerifier
	Permissions     PermissionChecker
	Tenants         TenantValidator
	DataPermissions DataPermissionProvider
	Operations      OperateLogRecorder
	Logger          *slog.Logger
	AllowedOrigins  []string
}

type Route struct {
	PermitAll bool
	// CredentialExchange leaves credential validation to the public auth use
	// case. A stale bearer or visit override must not block login or logout.
	CredentialExchange bool
	TenantIgnore       bool
	Permission         string
	AuditType          string
	AuditSubType       string
}

type Router struct {
	Engine *gin.Engine
	deps   Dependencies
}

func NewRouter(deps Dependencies) *Router {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	router := &Router{Engine: engine, deps: deps}
	engine.Use(router.recovery(), trace(), cors(deps.AllowedOrigins))
	engine.NoRoute(func(c *gin.Context) { Failure(c, 404, "接口不存在") })
	engine.NoMethod(func(c *gin.Context) { Failure(c, 405, "请求方法不支持") })
	engine.HandleMethodNotAllowed = true
	return router
}

func (r *Router) Handle(method, path string, route Route, handler gin.HandlerFunc) {
	r.Engine.Handle(method, path, r.guard(route), handler)
}

func Success(c *gin.Context, value any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": value, "msg": ""})
}

func SetAuditBizID(c *gin.Context, id int64) { c.Set("owot:audit_biz_id", id) }

func Failure(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": code, "data": nil, "msg": message})
}

func InternalError(c *gin.Context, logger *slog.Logger, err error) {
	logger.ErrorContext(c.Request.Context(), "request failed", "trace_id", ctxmeta.TraceID(c.Request.Context()), "error", err)
	Failure(c, 500, "系统异常")
}

func (r *Router) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if value := recover(); value != nil {
				r.deps.Logger.Error("HTTP handler panic", "trace_id", ctxmeta.TraceID(c.Request.Context()), "panic", value)
				Failure(c, 500, "系统异常")
			}
		}()
		c.Next()
	}
}

func trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("trace-id")
		decoded, err := hex.DecodeString(id)
		if err != nil || len(decoded) != 16 {
			var data [16]byte
			if _, err := rand.Read(data[:]); err != nil {
				Failure(c, 500, "系统异常")
				return
			}
			id = hex.EncodeToString(data[:])
		}
		c.Header("trace-id", id)
		c.Request = c.Request.WithContext(ctxmeta.WithTraceID(c.Request.Context(), id))
		c.Next()
	}
}

func (r *Router) guard(route Route) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if route.CredentialExchange && route.PermitAll && route.Permission == "" {
			id, err := strconv.ParseInt(c.GetHeader("tenant-id"), 10, 64)
			if err != nil || id <= 0 {
				Failure(c, 400, "缺少或无效的租户编号")
				return
			}
			c.Request = c.Request.WithContext(ctxmeta.WithTenantID(ctx, id))
			c.Next()
			return
		}
		token := c.Query("token")
		if authorization := c.GetHeader("Authorization"); authorization != "" {
			parts := strings.Fields(authorization)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				Failure(c, 401, "账号未登录")
				return
			}
			token = parts[1]
		}
		var user ctxmeta.LoginUser
		if token != "" {
			if r.deps.Tokens == nil {
				Failure(c, 401, "账号未登录")
				return
			}
			var err error
			user, err = r.deps.Tokens.Verify(ctx, token)
			if err != nil && !errors.Is(err, ErrUnauthenticated) {
				InternalError(c, r.deps.Logger, err)
				return
			}
			wantType := 1
			if strings.HasPrefix(c.FullPath(), "/app-api/") {
				wantType = 2
			}
			if err != nil || user.ID <= 0 || user.UserType != wantType {
				Failure(c, 401, "账号未登录")
				return
			}
			ctx = ctxmeta.WithLoginUser(ctx, user)
		} else if !route.PermitAll {
			Failure(c, 401, "账号未登录")
			return
		}
		if route.TenantIgnore {
			ctx = ctxmeta.WithIgnoreTenant(ctx)
		} else {
			id, err := strconv.ParseInt(c.GetHeader("tenant-id"), 10, 64)
			if err != nil || id <= 0 {
				Failure(c, 400, "缺少或无效的租户编号")
				return
			}
			if user.ID > 0 && user.TenantID != id {
				Failure(c, 403, "无权访问该租户")
				return
			}
			if visit := c.GetHeader("visit-tenant-id"); visit != "" {
				if !user.SuperAdmin {
					Failure(c, 403, "无权切换租户")
					return
				}
				id, err = strconv.ParseInt(visit, 10, 64)
				if err != nil || id <= 0 {
					Failure(c, 400, "无效的访问租户编号")
					return
				}
			}
			if r.deps.Tenants == nil {
				Failure(c, 403, "租户不可用")
				return
			}
			if err := r.deps.Tenants.ValidateTenant(ctx, id); err != nil {
				Failure(c, 403, "租户不可用")
				return
			}
			ctx = ctxmeta.WithTenantID(ctx, id)
		}
		if user.ID > 0 && !route.TenantIgnore && r.deps.DataPermissions != nil {
			scope, err := r.deps.DataPermissions.DataScope(ctx, user)
			if err != nil {
				InternalError(c, r.deps.Logger, err)
				return
			}
			ctx = ctxmeta.WithDataScope(ctx, scope)
		}
		c.Request = c.Request.WithContext(ctx)
		if route.Permission != "" && !user.SuperAdmin {
			if r.deps.Permissions == nil {
				Failure(c, 403, "没有该操作权限")
				return
			}
			allowed, err := r.deps.Permissions.HasPermission(ctx, user, route.Permission)
			if err != nil {
				InternalError(c, r.deps.Logger, err)
				return
			}
			if !allowed {
				Failure(c, 403, "没有该操作权限")
				return
			}
		}
		c.Next()
		if route.AuditType != "" && r.deps.Operations != nil && !c.IsAborted() {
			// Bounded synchronous recording avoids untracked request goroutines.
			logCtx, cancel := context.WithTimeout(ctxmeta.CloneContext(ctx), 2*time.Second)
			defer cancel()
			err := r.deps.Operations.Record(logCtx, OperateLog{Type: route.AuditType, SubType: route.AuditSubType,
				BizID: c.GetInt64("owot:audit_biz_id"), Action: route.AuditSubType, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.Path,
				UserIP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
			if err != nil {
				r.deps.Logger.Error("record operation audit", "error", err)
			}
		}
	}
}
