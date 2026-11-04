# DOC-01：yudao-framework 框架扩展底座模块（Go `pkg/` 实现规范）

## 一、模块概述与架构 Review 评估

- **模块定位**：对标 `ruoyi-vue-pro` 的 `yudao-framework`，是全平台的技术组件与业务中间件底座。
- **架构 Review 结论**：
  1. **单向依赖原则**：`pkg/` 绝对禁止 import `internal/modules/*` 的具体业务实现；当 `pkg/security` 或 `pkg/web` 需要校验 Token、检查权限或写入数据库日志时，仅依赖 `internal/api/system` 或 `internal/api/infra` 抽象接口，由 `wire` 在启动时注入实现。
  2. **内聚组件边界**：原项目自 2.0.1~2.5.1 版本起将 `file`、`captcha`、`flowable`、`pay` 四个 Starter 下沉内聚到唯一使用它们的业务模块 `framework/` 目录中，Go 实现严格遵循该内聚设计，保持顶层 `pkg/` 纯粹轻量。

## 二、18 个框架组件完整映射清单

| 编号 | 原 Maven 组件名 | 组件属性 | Go 对应包路径 | 核心职责与 Go 选型说明 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `yudao-common` | 基础公共 | `pkg/common` | 泛型 `CommonResult[T]`、`PageParam`、`PageResult[T]`、`ErrorCode`、`ServiceException`、`samber/lo` 集合工具 |
| 02 | `starter-web` | 技术组件 | `pkg/web` | Gin 引擎封装、`/admin-api` 与 `/app-api` 双路由组、全局异常恢复、Jackson 兼容 JSON 序列化、XSS 过滤、AccessLog |
| 03 | `starter-security` | 技术组件 | `pkg/security` | Bearer Token 鉴权中间件、`LoginUser` 上下文、`RequirePermission` 路由权限守卫、`OperateLog` 操作审计切面 |
| 04 | `starter-mybatis` | 技术组件 | `pkg/database` | GORM + GORM Gen 封装、`BaseDO` / `TenantBaseDO`、0/1 软删除、泛型分页 `SelectPage[T]`、`dbresolver` 多数据源 |
| 05 | `starter-redis` | 技术组件 | `pkg/cache` | `go-redis/v9` 封装、泛型强类型基类 `RedisDAO[T]`、`ristretto` 本地二级缓存 |
| 06 | `starter-mq` | 技术组件 | `pkg/mq` | 基于 `ThreeDotsLabs/watermill` 的统一 Pub/Sub 接口（支持内存、Redis Stream 集群消费、Redis Pub/Sub 广播、RocketMQ、RabbitMQ、Kafka） |
| 07 | `starter-job` | 技术组件 | `pkg/job` | 基于 `robfig/cron/v3` + `Redsync` 分布式锁的集群定时任务调度器，支持在线启停、参数传递与重试 |
| 08 | `starter-websocket` | 技术组件 | `pkg/websocket` | `gorilla/websocket` 会话池 `SessionManager`、握手 Token 鉴权、单点/广播推送与跨实例 MQ 路由 |
| 09 | `starter-protection` | 技术组件 | `pkg/protection` | 高并发保障：Redsync 分布式锁、`Idempotent` 防重复提交中间件、`RateLimiter` Redis Lua 令牌桶限流、`gobreaker` 熔断 |
| 10 | `starter-excel` | 技术组件 | `pkg/excel` | `xuri/excelize/v2` 流式导入导出、结构体标签 `excel:"name:状态;dict:common_status"` 自动字典翻译 |
| 11 | `starter-monitor` | 技术组件 | `pkg/monitor` | OpenTelemetry 链路追踪（注入 `trace_id`）、Prometheus `/metrics` 指标暴露、`uber-go/zap` 日志 |
| 12 | `starter-test` | 技术组件 | `pkg/testutil` | `stretchr/testify` + `alicebob/miniredis/v2` + `glebarez/sqlite` 内存单元测试容器 |
| 13 | `starter-biz-tenant` | 业务组件 | `pkg/biz/tenant` | SaaS 多租户 Context 传递、GORM Callback 自动拼接 `WHERE tenant_id = ?`、`WithIgnoreTenant(ctx)` 豁免控制 |
| 14 | `starter-biz-data-permission` | 业务组件 | `pkg/biz/dataperm` | 基于当前管理员角色的 5 级 `DataScope` 动态改写 GORM 查询子句（`dept_id IN (?)` 或 `creator = ?`） |
| 15 | `starter-biz-ip` | 业务组件 | `pkg/biz/ip` | `embed.FS` 内嵌省市区三级树 `area.csv` 与 `ip2region.xdb` 内存向量索引离线 IP 归属地查询 |
| 16 | `starter-file` | 内聚组件 | `modules/infra/framework/file` | 统一 `FileClient` 接口，适配 S3（MinIO/OSS/COS/OBS/七牛）、本地磁盘、FTP、SFTP、数据库 BLOB |
| 17 | `starter-captcha` | 内聚组件 | `modules/system/framework/captcha` | 基于 `wenlng/go-captcha` 实现滑动拼图与文字点选验证码生成及坐标校验 |
| 18 | `starter-pay` | 内聚组件 | `modules/pay/framework/pay` | 统一 `PayClient` 接口，基于 `go-pay/gopay` 适配微信支付 V2/V3、支付宝、会员电子钱包与模拟支付 |

## 三、核心组件 Go 源码实现

### 3.1 `pkg/common`：统一响应结构与分页契约

```go
package common

import "fmt"

type ErrorCode struct {
	Code int
	Msg  string
}

var (
	Success            = ErrorCode{Code: 0, Msg: ""}
	ErrBadRequest      = ErrorCode{Code: 400, Msg: "请求参数不正确"}
	ErrUnauthorized    = ErrorCode{Code: 401, Msg: "账号未登录"}
	ErrForbidden       = ErrorCode{Code: 403, Msg: "没有该操作权限"}
	ErrNotFound        = ErrorCode{Code: 404, Msg: "请求未找到"}
	ErrLocked          = ErrorCode{Code: 423, Msg: "请求失败，请稍后重试"}
	ErrTooManyRequests = ErrorCode{Code: 429, Msg: "请求过于频繁，请稍后重试"}
	ErrInternalServer  = ErrorCode{Code: 500, Msg: "系统异常"}
)

type ServiceException struct {
	Code int
	Msg  string
}

func (e *ServiceException) Error() string { return e.Msg }

func NewServiceException(ec ErrorCode, args ...interface{}) error {
	msg := ec.Msg
	if len(args) > 0 {
		msg = fmt.Sprintf(ec.Msg, args...)
	}
	return &ServiceException{Code: ec.Code, Msg: msg}
}

// CommonResult 严格对齐前端 axios 响应拦截器契约 {code, data, msg}
type CommonResult[T any] struct {
	Code int    `json:"code"`
	Data T      `json:"data"`
	Msg  string `json:"msg"`
}

type PageParam struct {
	PageNo   int `form:"pageNo,default=1" json:"pageNo" binding:"min=1"`
	PageSize int `form:"pageSize,default=10" json:"pageSize" binding:"min=1,max=100"`
}

type PageResult[T any] struct {
	List  []T   `json:"list"`
	Total int64 `json:"total"`
}

func NewEmptyPageResult[T any]() *PageResult[T] {
	return &PageResult[T]{List: make([]T, 0), Total: 0}
}
```

### 3.2 `pkg/database` & `pkg/biz/tenant`：GORM 基础实体与多租户透明隔离插件

```go
package database

import (
	"time"
	"oneweekonething.com/owot-pro/pkg/common"
	"gorm.io/gorm"
	"gorm.io/plugin/soft_delete"
)

// BaseDO 基础实体：自动维护审计字段与 0/1 软删除标志
type BaseDO struct {
	ID         int64                 `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Creator    string                `gorm:"column:creator;size:64;default:''" json:"creator"`
	CreateTime time.Time             `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	Updater    string                `gorm:"column:updater;size:64;default:''" json:"updater"`
	UpdateTime time.Time             `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted    soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag;default:0" json:"-"`
}

// TenantBaseDO 多租户基础实体：凡嵌入该结构体的表均自动受 GORM 多租户插件隔离保护
type TenantBaseDO struct {
	BaseDO
	TenantID int64 `gorm:"column:tenant_id;not null;default:0;index" json:"tenantId"`
}

// SelectPage 通用泛型分页查询器
func SelectPage[T any](db *gorm.DB, param common.PageParam) (*common.PageResult[T], error) {
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	if total == 0 {
		return common.NewEmptyPageResult[T](), nil
	}
	list := make([]T, 0, param.PageSize)
	offset := (param.PageNo - 1) * param.PageSize
	if err := db.Offset(offset).Limit(param.PageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	return &common.PageResult[T]{List: list, Total: total}, nil
}
```

### 3.3 `pkg/security`：双端 Token 认证与 RBAC 按钮级权限中间件

```go
package security

import (
	"strings"
	"github.com/gin-gonic/gin"
	"oneweekonething.com/owot-pro/internal/api/system"
	"oneweekonething.com/owot-pro/pkg/common"
	"oneweekonething.com/owot-pro/pkg/web"
)

const (
	UserTypeAdmin  = 1 // 管理后台用户 (/admin-api/**)
	UserTypeMember = 2 // C 端会员用户 (/app-api/**)
)

type LoginUser struct {
	ID       int64             `json:"id"`
	UserType int               `json:"userType"`
	TenantID int64             `json:"tenantId"`
	DeptID   int64             `json:"deptId"`
	Scopes   []string          `json:"scopes"`
	Info     map[string]string `json:"info"`
}

// AuthMiddleware 解析 Authorization: Bearer <token> 并验证用户类型与租户一致性
func AuthMiddleware(tokenApi system.OAuth2TokenApi, permitAllPaths map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c.GetHeader("Authorization"))
		if token != "" {
			loginUser, err := tokenApi.CheckAccessToken(c.Request.Context(), token)
			if err != nil {
				web.WriteError(c, common.NewServiceException(common.ErrUnauthorized))
				c.Abort()
				return
			}
			// 校验请求路由前缀与 Token 的 UserType 是否匹配，严防 C 端会员 Token 越权访问 /admin-api
			expectedType := UserTypeAdmin
			if strings.HasPrefix(c.Request.URL.Path, "/app-api/") {
				expectedType = UserTypeMember
			}
			if loginUser.UserType != expectedType {
				web.WriteError(c, common.NewServiceException(common.ErrForbidden))
				c.Abort()
				return
			}
			c.Request = c.Request.WithContext(WithLoginUser(c.Request.Context(), loginUser))
			c.Next()
			return
		}
		if permitAllPaths[c.FullPath()] {
			c.Next()
			return
		}
		web.WriteError(c, common.NewServiceException(common.ErrUnauthorized))
		c.Abort()
	}
}

// RequirePermission 路由级 RBAC 按钮权限校验中间件（对标 @PreAuthorize("@ss.hasPermission('xxx')")）
func RequirePermission(permApi system.PermissionApi, permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetLoginUser(c.Request.Context())
		if user == nil {
			web.WriteError(c, common.NewServiceException(common.ErrUnauthorized))
			c.Abort()
			return
		}
		ok, err := permApi.HasAnyPermissions(c.Request.Context(), user.ID, permissions...)
		if err != nil || !ok {
			web.WriteError(c, common.NewServiceException(common.ErrForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}
```

## 四、框架层 Review 验收与单测 Checklist

- [x] **空切片 JSON 序列化检查**：`NewEmptyPageResult[T]()` 必须初始化 `make([]T, 0)`，确保 JSON 序列化输出 `"list": []` 而非 `"list": null`。
- [x] **跨端越权防护检查**：`AuthMiddleware` 强制比对 `LoginUser.UserType` 与 `/admin-api`、`/app-api` 路由前缀，杜绝 C 端会员 Token 调用管理后台接口。
- [x] **异步协程 Context 租户透传检查**：凡开启后台 Goroutine 执行异步逻辑，必须使用 `tenant.CloneContext(ctx)` 拷贝 `TenantID` 与 `TraceID`，避免受 HTTP 请求结束 Cancel 影响。
