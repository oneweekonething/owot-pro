# OWOT-Pro (`oneweekonething.com/owot-pro`) 基础功能模块架构与实现手册

> **品牌理念**：*One Week, One Module; One Week, One System.*（一周打磨一个精品模块，一周交付一套企业级系统）
> **文档说明**：本手册为 **OWOT-Pro** 开源项目的基础功能模块完整架构与代码实现规范（对标 [YunaiV/ruoyi-vue-pro](https://github.com/YunaiV/ruoyi-vue-pro)），所有 Go 包导入路径已统一规范为 `oneweekonething.com/owot-pro`。


# DOC-00：ruoyi-vue-pro 基础功能模块 Review 总览与 Go 工程总体架构规范

本文档库基于开源项目 [YunaiV/ruoyi-vue-pro](https://github.com/YunaiV/ruoyi-vue-pro) 与[官方开发指南（doc.iocoder.cn）](https://doc.iocoder.cn/project-intro/)，经过全面架构与代码级 Review 后整理输出。全套文档采用**按模块独立存放的多文件目录结构**，优先完成全平台最核心的 **基础功能模块（顶层容器 + 框架底座 + 2 个必选核心模块 + 5 个通用基础支撑模块）**。

## 一、多文件目录结构与模块文档清单

```text
owot-pro/docs/
├── README.md                                  # [DOC-00] 总体架构规范、Review 总结与多文件目录总索引
├── 00-architecture/
│   └── 00-architecture-and-server.md          # [DOC-00] yudao-dependencies 依赖管理与 yudao-server 启动容器设计
├── 01-framework/
│   └── 01-yudao-framework.md                  # [DOC-01] yudao-framework 18 个技术与业务 Starter 组件 Go 实现规范
├── 02-base-modules/                           # [基础功能模块集 - 已全部通过深度 Review 并完成整理]
│   ├── 02-module-system.md                    # [DOC-02][必选核心] yudao-module-system 系统功能模块规范 (29张表)
│   ├── 03-module-infra.md                     # [DOC-03][必选核心] yudao-module-infra 基础设施模块规范 (12张表)
│   ├── 04-module-bpm.md                       # [DOC-04][通用基础] yudao-module-bpm 工作流程模块规范 (12张表)
│   ├── 05-module-pay.md                       # [DOC-05][通用基础] yudao-module-pay 支付系统模块规范 (12张表)
│   ├── 06-module-member.md                    # [DOC-06][通用基础] yudao-module-member 会员中心模块规范 (11张表)
│   ├── 07-module-report.md                    # [DOC-07][通用基础] yudao-module-report 大屏与报表模块规范
│   └── 08-module-mp.md                        # [DOC-08][通用基础] yudao-module-mp 微信公众号模块规范 (7张表)
└── 03-biz-modules/                            # [第二阶段垂类业务模块预留目录]
    └── README.md                              # 12 大垂类业务系统模块（Mall/ERP/WMS/CRM/MES/HRM/FMS/PMS/OA/AI/IoT/IM）接口边界
```

### 各基础功能模块深度 Review 核查汇总表

在本次 Review 整理中，所有基础功能模块文档均统一对齐为 **7 大标准章节**（`模块概述与 Review 评估` -> `Go 标准包目录结构` -> `全量数据库表结构清单` -> `RedisDAO / MQ / Job 设计` -> `全量 RESTful API 路由契约表` -> `跨模块 API 与核心 Go 源码实现` -> `Review 验收与单测 Checklist`）：

| 文档编号 | 模块名称 | 模块分类 | 涉及数据表 | Review 重点核查与修正内容 |
| :--- | :--- | :--- | :--- | :--- |
| **DOC-00** | `yudao-dependencies` + `yudao-server` | 顶层容器 | 无 | 确立 `go.work` + `go.mod` 依赖锁定基线；确立 `cmd/owot-server/wire.go` 编译期静态依赖注入与 `internal/api/*` 跨模块单向依赖铁律 |
| **DOC-01** | `yudao-framework` (`pkg/`) | 框架底座 | 无 | 核查 18 个 Starter 组件（含 3 个已内聚组件）；统一 Gin 6 层中间件执行顺序；补齐 GORM 多租户自动注入、5 级数据权限、泛型 `RedisDAO[T]` 与防重放幂等源码 |
| **DOC-02** | `yudao-module-system` | **必选核心** | 29 张表 | 严格区分全局表（菜单/字典/租户）与租户隔离表（用户/角色/部门）；补齐 Ristretto 本地缓存 + Redis Pub/Sub 广播刷新机制；完善租户初始化级联事务与数据权限计算源码 |
| **DOC-03** | `yudao-module-infra` | **必选核心** | 12 张表 | 补齐基于 `embed.FS` + `text/template` 的 Go + Vue3 全栈代码生成器模板矩阵（单表/树表/主子表）；完善 S3/MinIO/OSS/COS/本地/FTP/SFTP/DB 多协议文件客户端工厂与动态 Cron 调度器 |
| **DOC-04** | `yudao-module-bpm` | 通用基础 | 12 张表 | 针对 Go 无原生 Flowable 的痛点，设计 5 张轻量级引擎表 + 原生 Go **DAG 有向图与 Token 状态机**，完整支持 BPMN XML 与仿钉钉 Simple JSON 双设计器、会签/或签/依次审批/加减签/自由驳回 |
| **DOC-05** | `yudao-module-pay` | 通用基础 | 12 张表 | 核查支付单（`pay_order`）与支付渠道扩展单（`pay_order_extension`）1:N 模型；补齐 `PayClient` 多渠道接口、Redsync 锁 + CAS 乐观锁回调状态机及 8 级阶梯重试通知任务 |
| **DOC-06** | `yudao-module-member` | 通用基础 | 11 张表 | 明确 C 端会员（`user_type=2`）与后台管理员（`user_type=1`）物理表隔离但复用 OAuth2 Token 与 Social 三方绑定的架构；补齐积分 SQL 乐观锁防负值扣减与成长值触发自动升级源码 |
| **DOC-07** | `yudao-module-report` | 通用基础 | 2+ 张表 | 针对 GoView 大屏动态 SQL 执行接口的高危风险，设计正则黑名单 + SQL AST 校验 + 5 秒超时只读事务 + 租户 ID 强制过滤的四重安全沙箱 |
| **DOC-08** | `yudao-module-mp` | 通用基础 | 7 张表 | 补齐 `internal/modules/mp` 完整包结构、Redis AccessToken 分布式缓存、临时多媒体素材自动转存 S3 机制，以及关注/取关/菜单点击/关键词自动回复责任链源码 |

## 二、Go 语言工程全局架构规范（`owot-pro`）

### 2.1 Go Monorepo 顶层工程目录树

```text
owot-pro/
├── go.work                           # Go Workspace 多模块编排（对标 yudao-dependencies）
├── go.mod                            # 统一第三方库版本基线
├── configs/
│   ├── config.yaml                   # 主配置文件（Server, Database, Redis, MQ, Tenant, Security）
│   └── config.dev.yaml               # 本地开发环境覆盖配置
├── sql/                              # 数据库初始化与迁移脚本 (mysql / postgres / dm8 / kingbase)
├── cmd/
│   └── yudao-server/                 # 主服务端组装容器（对标 yudao-server）
│       ├── main.go                   # 程序入口：加载配置 -> Wire 初始化 -> 启动 Gin & Cron & MQ
│       ├── wire.go                   # 编译期依赖注入声明：按需组合各业务模块 ProviderSet
│       └── wire_gen.go               # wire 自动生成的静态装配代码
├── pkg/                              # 框架扩展底座（对标 yudao-framework，详见 DOC-01）
│   ├── common/                       # CommonResult[T], PageParam, PageResult[T], ErrorCode, ServiceException
│   ├── web/                          # Gin 双路由组 (/admin-api, /app-api), 全局 Recovery, Validator, AccessLog
│   ├── security/                     # Bearer Token 认证中间件, LoginUser Context, RequirePermission, OperateLog
│   ├── database/                     # GORM 初始化, BaseDO / TenantBaseDO, SelectPage[T], 读写分离
│   ├── cache/                        # go-redis/v9 封装, 泛型 RedisDAO[T], ristretto 进程内二级缓存
│   ├── mq/                           # Watermill 统一消息总线 (Memory / Redis Stream / PubSub / Kafka / RocketMQ)
│   ├── job/                          # robfig/cron/v3 + Redsync 集群定时任务调度器
│   ├── websocket/                    # WebSocket 连接池、鉴权与跨节点 Redis Pub/Sub 广播
│   ├── protection/                   # Redsync 分布式锁、Idempotent 防重复提交中间件、Redis Lua 限流器
│   ├── excel/                        # Excelize v2 泛型导入导出与字典标签自动翻译器
│   ├── monitor/                      # OpenTelemetry 链路追踪、Prometheus 指标、Zap 结构化日志
│   ├── testutil/                     # Testify + Miniredis + SQLite 内存库单元测试容器
│   └── biz/                          # 业务级框架组件
│       ├── tenant/                   # SaaS 多租户 Context 传递与 GORM AST 自动注入 WHERE tenant_id = ?
│       ├── dataperm/                 # 5 级部门数据权限 GORM 动态子句改写插件
│       └── ip/                       # ip2region 离线 IP 解析与省市区行政区划树内存索引
├── internal/
│   ├── api/                          # 跨模块契约层（仅含 Interface 与 DTO，打破 Go 包循环引用）
│   │   ├── system/                   # AdminUserApi, DeptApi, RoleApi, PermissionApi, DictDataApi, TenantApi, SmsSendApi...
│   │   ├── infra/                    # FileApi, ConfigApi, ApiAccessLogApi, ApiErrorLogApi
│   │   ├── bpm/                      # BpmProcessInstanceApi
│   │   ├── pay/                      # PayOrderApi, PayRefundApi, PayTransferApi, PayWalletApi
│   │   └── member/                   # MemberUserApi, MemberLevelApi, MemberPointApi, MemberAddressApi
│   └── modules/                      # 基础功能模块实现层（每个模块内部结构严格统一）
│       ├── system/                   # [DOC-02] 系统功能模块（必选）
│       ├── infra/                    # [DOC-03] 基础设施模块（必选）
│       ├── bpm/                      # [DOC-04] 工作流程模块（通用可选）
│       ├── pay/                      # [DOC-05] 支付系统模块（通用可选）
│       ├── member/                   # [DOC-06] 会员中心模块（通用可选）
│       ├── report/                   # [DOC-07] 大屏报表模块（通用可选）
│       └── mp/                       # [DOC-08] 微信公众号模块（通用可选）
```

### 2.2 服务端容器静态装配实现（`cmd/owot-server/wire.go`）

```go
//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"
	"oneweekonething.com/owot-pro/internal/modules/bpm"
	"oneweekonething.com/owot-pro/internal/modules/infra"
	"oneweekonething.com/owot-pro/internal/modules/member"
	"oneweekonething.com/owot-pro/internal/modules/mp"
	"oneweekonething.com/owot-pro/internal/modules/pay"
	"oneweekonething.com/owot-pro/internal/modules/report"
	"oneweekonething.com/owot-pro/internal/modules/system"
	"oneweekonething.com/owot-pro/pkg/bootstrap"
)

// BaseModulesProviderSet 基础功能模块装配集合
// 若需裁剪为极简版（yudao-boot-mini），仅需保留 system.ProviderSet 与 infra.ProviderSet
var BaseModulesProviderSet = wire.NewSet(
	system.ProviderSet,
	infra.ProviderSet,
	bpm.ProviderSet,
	pay.ProviderSet,
	member.ProviderSet,
	report.ProviderSet,
	mp.ProviderSet,
)

func InitServer(configPath string) (*bootstrap.Server, func(), error) {
	panic(wire.Build(
		bootstrap.FrameworkSet,
		BaseModulesProviderSet,
		bootstrap.NewServer,
	))
}
```

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

# DOC-02：yudao-module-system 系统功能模块（必选核心基础模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：全平台的身份认证中心（IAM）、RBAC 菜单与按钮权限底座、5 级部门数据权限中枢、SaaS 多租户与套餐管控中心，以及统一消息通知渠道（短信、邮件、站内信）。
- **Go 模块路径**：`internal/modules/system`（对外跨模块契约：`internal/api/system`）
- **架构 Review 核心结论**：
  1. **全局表 vs 租户隔离表严格区分**：`system` 模块共 29 张表，其中 13 张为租户隔离表（含 `tenant_id`），16 张为系统全局表（如 `system_menu`、`system_dict_data`、`system_tenant`、`system_sms_channel`）。在注册 `pkg/biz/tenant` 插件时，必须将这 16 张全局表配置进 `ignoreTables` 白名单。
  2. **高频权限与字典查询零 DB 压力**：角色权限、部门树、数据字典、敏感词在每次请求中均可能被调用，Go 实现采用 `ristretto` 进程内缓存 + `Watermill` Redis Pub/Sub 广播刷新机制，确保平均查询耗时 `<10μs`。
  3. **密码算法无缝兼容**：原项目使用 Spring Security `BCryptPasswordEncoder`（前缀 `$2a$10$...`），Go 标准扩展库 `golang.org/x/crypto/bcrypt` 原生支持 `$2a$` 格式，老数据库用户无需重置密码即可直接登录。

## 二、Go 模块标准包目录结构

```text
internal/modules/system/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/system/*)
│   │   ├── auth/                     # 登录、登出、刷新 Token、获取权限菜单树、短信/社交快捷登录
│   │   ├── captcha/                  # 滑动拼图与点选验证码获取与校验
│   │   ├── dept/                     # 部门树 CRUD 与岗位 CRUD
│   │   ├── dict/                     # 字典类型与字典数据项管理
│   │   ├── errorcode/                # 动态错误码管理
│   │   ├── ip/                       # 省市区行政区划树与 IP 归属地查询
│   │   ├── logger/                   # 操作日志与登录日志分页查询与导出
│   │   ├── mail/                     # 邮箱账号、邮件模板与发送日志
│   │   ├── notice/                   # 系统通知公告发布管理
│   │   ├── notify/                   # 站内信模板与我的站内信列表（已读/未读统计）
│   │   ├── oauth2/                   # OAuth2 客户端管理、在线用户 Token 监控与强退、SSO 授权页
│   │   ├── permission/               # 菜单管理、角色管理、赋予角色菜单权限/数据权限、赋予用户角色
│   │   ├── sensitiveword/            # 敏感词 CRUD 与文本检测验证
│   │   ├── sms/                      # 短信渠道、短信模板、短信发送日志与渠道异步回调
│   │   ├── social/                   # 三方社交客户端配置、社交用户与绑定记录
│   │   ├── tenant/                   # SaaS 租户 CRUD、租户套餐 CRUD、域名/名称解析租户 ID
│   │   └── user/                     # 后台用户 CRUD、重置密码、修改状态、个人中心资料修改、Excel 导入导出
│   └── app/                          # 移动端 App 接口 (/app-api/system/*)
│       ├── dict/                     # 移动端数据字典查询
│       ├── ip/                       # 移动端省市区树查询
│       └── tenant/                   # 移动端租户信息获取
├── service/                          # 15 个核心业务子域 Service 接口与实现
│   ├── auth/
│   ├── dept/
│   ├── dict/
│   ├── errorcode/
│   ├── logger/
│   ├── mail/
│   ├── notice/
│   ├── notify/
│   ├── oauth2/
│   ├── permission/
│   ├── sensitiveword/
│   ├── sms/
│   ├── social/
│   ├── tenant/
│   └── user/
├── dal/
│   ├── dataobject/                   # 29 张数据库表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO 与自定义 Mapper
│   └── redis/                        # OAuth2AccessTokenRedisDAO, CaptchaRedisDAO, SocialStateRedisDAO
├── api/                              # 实现 internal/api/system 中定义的 12 个跨模块接口
├── mq/                               # 短信/邮件异步发送消费者、角色菜单/字典/敏感词缓存刷新广播
├── job/                              # 过期 Token 清理任务
├── enums/                            # 模块错误码段 (1_002_000_000 起) 与业务枚举常量
├── convert/                          # DO <-> DTO <-> VO 类型转换函数
├── framework/
│   ├── captcha/                      # 内聚滑块验证码组件实现
│   └── sms/                          # 阿里云、腾讯云、华为云、调试钉钉短信客户端实现
└── module.go                         # 导出 system.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（29 张表，前缀 `system_`）

| 序号 | 表名 | 租户隔离 | 主键与核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `system_users` | **租户级 (`tenant_id`)** | `id`, `username`, `password`(BCrypt), `nickname`, `remark`, `dept_id`, `post_ids`(JSON数组), `email`, `mobile`, `sex`, `avatar`, `status`, `login_ip`, `login_date` | `uk_username_tenant` (`username`, `deleted`, `tenant_id`) |
| 02 | `system_role` | **租户级 (`tenant_id`)** | `id`, `name`, `code`(`super_admin`/`tenant_admin`等), `sort`, `data_scope`(1~5), `data_scope_dept_ids`(JSON), `status`, `type`(1内置/2自定义) | `idx_tenant_code` (`tenant_id`, `code`) |
| 03 | `system_menu` | 全局表（无 `tenant_id`） | `id`, `name`, `permission`(如 `system:user:create`), `type`(1目录/2菜单/3按钮), `sort`, `parent_id`, `path`, `icon`, `component`, `component_name`, `status`, `visible`, `keep_alive`, `always_show` | `idx_parent_id` (`parent_id`) |
| 04 | `system_user_role` | **租户级 (`tenant_id`)** | `id`, `user_id`, `role_id` | `idx_user_id`, `idx_role_id` |
| 05 | `system_role_menu` | **租户级 (`tenant_id`)** | `id`, `role_id`, `menu_id` | `idx_role_menu` (`role_id`, `menu_id`) |
| 06 | `system_dept` | **租户级 (`tenant_id`)** | `id`, `name`, `parent_id`, `sort`, `leader_user_id`, `phone`, `email`, `status` | `idx_parent_id` (`parent_id`, `tenant_id`) |
| 07 | `system_post` | **租户级 (`tenant_id`)** | `id`, `code`, `name`, `sort`, `status`, `remark` | `idx_tenant_code` (`tenant_id`, `code`) |
| 08 | `system_user_post` | **租户级 (`tenant_id`)** | `id`, `user_id`, `post_id` | `idx_user_post` (`user_id`, `post_id`) |
| 09 | `system_tenant` | 全局表 | `id`, `name`, `contact_user_id`, `contact_name`, `contact_mobile`, `status`, `websites`(绑定域名JSON数组), `package_id`(0表示系统租户), `expire_time`, `account_count`(最大账号配额) | `idx_name` (`name`) |
| 10 | `system_tenant_package` | 全局表 | `id`, `name`, `status`, `remark`, `menu_ids`(JSON数组，定义该套餐开放的菜单ID集合) | 主键索引 `id` |
| 11 | `system_oauth2_client` | 全局表 | `id`, `client_id`, `secret`, `name`, `logo`, `description`, `status`, `access_token_validity_seconds`, `refresh_token_validity_seconds`, `redirect_uris`, `authorized_grant_types`, `scopes`, `auto_approve_scopes` | `uk_client_id` (`client_id`) |
| 12 | `system_oauth2_access_token` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`(1管理员/2会员), `user_info`(JSON), `access_token`, `refresh_token`, `client_id`, `scopes`, `expires_time` | `idx_access_token`, `idx_refresh_token` |
| 13 | `system_oauth2_refresh_token` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `refresh_token`, `client_id`, `scopes`, `expires_time` | `idx_refresh_token` |
| 14 | `system_oauth2_code` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `code`, `client_id`, `scopes`, `expires_time`, `redirect_uri`, `state` | `idx_code` (`code`) |
| 15 | `system_oauth2_approve` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `client_id`, `scope`, `approved`, `expires_time` | `idx_user_client` (`user_id`, `client_id`) |
| 16 | `system_social_client` | **租户级 (`tenant_id`)** | `id`, `name`, `social_type`(10公众号/20企微/30钉钉/31小程序), `user_type`, `client_id`, `client_secret`, `agent_id`, `status` | `idx_tenant_social` (`tenant_id`, `social_type`, `user_type`) |
| 17 | `system_social_user` | **租户级 (`tenant_id`)** | `id`, `type`, `openid`, `token`, `raw_token_info`, `nickname`, `avatar`, `raw_user_info`, `code`, `state` | `idx_type_openid` (`type`, `openid`) |
| 18 | `system_social_user_bind` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `social_type`, `social_user_id` | `idx_user_social` (`user_id`, `user_type`, `social_type`) |
| 19 | `system_dict_type` | 全局表 | `id`, `name`, `type`(如 `common_status`), `status`, `remark` | `uk_type` (`type`) |
| 20 | `system_dict_data` | 全局表 | `id`, `sort`, `label`, `value`, `dict_type`, `status`, `color_type`, `css_class`, `remark` | `idx_dict_type` (`dict_type`) |
| 21 | `system_sms_channel` | 全局表 | `id`, `signature`, `code`(`ALIYUN`/`TENCENT`/`HUAWEI`), `status`, `remark`, `api_key`, `api_secret`, `callback_url` | `idx_code` (`code`) |
| 22 | `system_sms_template` | 全局表 | `id`, `type`, `status`, `code`, `name`, `content`, `params`(JSON数组), `remark`, `api_template_id`, `channel_id`, `channel_code` | `uk_code` (`code`) |
| 23 | `system_sms_log` | 全局表 | `id`, `channel_id`, `channel_code`, `template_id`, `template_code`, `template_type`, `template_content`, `template_params`, `api_template_id`, `mobile`, `user_id`, `user_type`, `send_status`, `send_time`, `api_send_code`, `api_serial_no`, `receive_status`, `receive_time` | `idx_mobile` (`mobile`), `idx_send_time` |
| 24 | `system_sms_code` | **租户级 (`tenant_id`)** | `id`, `mobile`, `code`, `create_ip`, `scene`(1登录/2修改手机/3忘记密码), `today_index`, `used`, `used_time`, `used_ip` | `idx_mobile_scene` (`mobile`, `scene`) |
| 25 | `system_mail_account` | 全局表 | `id`, `mail`, `username`, `password`, `host`, `port`, `ssl_enable`, `starttls_enable` | `idx_mail` (`mail`) |
| 26 | `system_mail_template` | 全局表 | `id`, `name`, `code`, `account_id`, `nickname`, `title`, `content`, `params`, `status`, `remark` | `uk_code` (`code`) |
| 27 | `system_mail_log` | 全局表 | `id`, `user_id`, `user_type`, `to_mail`, `account_id`, `from_mail`, `template_id`, `template_code`, `template_nickname`, `template_title`, `template_content`, `template_params`, `send_status`, `send_time`, `send_exception` | `idx_to_mail` (`to_mail`) |
| 28 | `system_notify_template` & `system_notify_message` | 全局模板 / **租户级消息** | 模板：`id`, `name`, `code`, `nickname`, `content`, `type`, `params`, `status`；消息：`id`, `user_id`, `user_type`, `template_id`, `template_code`, `template_nickname`, `template_content`, `template_type`, `template_params`, `read_status`, `read_time` | `idx_user_read` (`user_id`, `user_type`, `read_status`) |
| 29 | `system_notice` / `system_sensitive_word` / `system_error_code` / `system_operate_log` / `system_login_log` | 公告与日志为**租户级**；敏感词与错误码为全局表 | 覆盖通知公告、敏感词库（含标签）、动态错误码、操作审计日志（模块/名/类型/方法/参数/结果/耗时/biz_id）与登录审计日志 | `idx_create_time`, `idx_user_id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与消费/过期策略 |
| :--- | :--- | :--- |
| **RedisDAO** | `oauth2_access_token:%s` | 签发或校验 AccessToken 时读写，TTL 等于 Token 剩余有效期（默认 30 分钟），强退用户时立即删除 |
| **RedisDAO** | `captcha_code:%s` | 生成滑块验证码时写入 AES 坐标值，TTL 2 分钟，校验一次后立即 `DEL` 防重放 |
| **MQ 集群消费** | `system.sms.send` (`SmsSendMessage`) | 调用 `SmsSendService.SendSingleSms` 落库 `system_sms_log` 后异步投递，由消费者调用阿里云/腾讯云 SDK 发送 |
| **MQ 集群消费** | `system.mail.send` (`MailSendMessage`) | 调用 `MailSendService.SendSingleMail` 落库 `system_mail_log` 后异步投递，由消费者通过 SMTP 发送邮件 |
| **MQ 广播刷新** | `system.cache.refresh` | 当菜单、角色权限、部门树、字典、敏感词、短信/邮件模板变更时广播，各节点收到后刷新本地 `ristretto` 缓存 |

## 五、全量 RESTful API 路由契约表（核心接口）

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/login` | `@PermitAll` | 账号密码登录（支持校验滑块验证码 `captchaVerification`），返回 `{userId, accessToken, refreshToken, expiresTime}` |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/refresh-token` | `@PermitAll` | 传入 Query 参数 `refreshToken` 换取新的双 Token |
| **认证 (`auth`)** | `GET` | `/admin-api/system/auth/get-permission-info` | 已登录即可 | 返回当前登录管理员的 `{user, roles: string[], permissions: string[], menus: MenuVO[]}` |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/logout` | `@PermitAll` | 提取 Header Token 并从数据库及 Redis 中同步移除会话 |
| **用户 (`user`)** | `GET` | `/admin-api/system/user/page` | `system:user:query` | 用户分页查询（若传入 `deptId`，自动通过内存部门树展开包含其所有子孙部门的用户） |
| **用户 (`user`)** | `POST/PUT/DELETE` | `/admin-api/system/user/{create,update,delete}` | `system:user:{create,update,delete}` | 创建用户（校验租户账号配额 `account_count`）、更新用户、删除用户 |
| **用户 (`user`)** | `PUT` | `/admin-api/system/user/update-password` | `system:user:update-password` | 管理员重置指定用户的登录密码（BCrypt Hash） |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-role-menu` | `system:permission:assign-role-menu` | 给角色分配菜单权限（对比差异集执行批量新增与软删除，并触发缓存刷新广播） |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-role-data-scope` | `system:permission:assign-role-data-scope` | 给角色配置 5 种数据范围权限及自定义部门 ID 集合 |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-user-role` | `system:permission:assign-user-role` | 给管理员分配角色列表 |
| **租户 (`tenant`)** | `GET` | `/admin-api/system/tenant/{get-id-by-name,get-by-website}` | `@PermitAll` | 登录页根据租户名称或当前访问域名解析 `tenantId` |
| **租户 (`tenant`)** | `POST/PUT` | `/admin-api/system/tenant/{create,update}` | `system:tenant:{create,update}` | 创建新租户（级联创建默认部门、管理员角色与初始账号）；更新租户套餐时自动回收超出新套餐范围的角色菜单权限 |
| **短信 (`sms`)** | `POST` | `/admin-api/system/sms-callback/{channelCode}` | `@PermitAll` | 接收阿里云/腾讯云/华为云短信网关的回执状态推送，更新 `system_sms_log.receive_status` |
| **站内信 (`notify`)** | `GET/PUT` | `/admin-api/system/notify-message/{my-page,get-unread-count,update-read}` | 已登录即可 | 个人站内信分页、未读数轮询与标记已读 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package system

import "context"

// AdminUserApi 供全系统查询后台管理员信息的跨模块契约
type AdminUserApi interface {
	GetUser(ctx context.Context, id int64) (*AdminUserRespDTO, error)
	GetUserList(ctx context.Context, ids []int64) ([]*AdminUserRespDTO, error)
	GetUserListByDeptIds(ctx context.Context, deptIds []int64) ([]*AdminUserRespDTO, error)
	GetUserListByPostIds(ctx context.Context, postIds []int64) ([]*AdminUserRespDTO, error)
	ValidateUserList(ctx context.Context, ids []int64) error
}

// PermissionApi 供 security 与 bpm/dataperm 调用的权限契约
type PermissionApi interface {
	HasAnyPermissions(ctx context.Context, userID int64, permissions ...string) (bool, error)
	HasAnyRoles(ctx context.Context, userID int64, roles ...string) (bool, error)
	GetDeptDataPermission(ctx context.Context, userID int64) (*DeptDataPermissionRespDTO, error)
	GetUserRoleIdListByRoleIds(ctx context.Context, roleIds []int64) ([]int64, error)
}

// NotifyMessageSendApi 供各业务模块向管理员或会员发送站内信的契约
type NotifyMessageSendApi interface {
	SendSingleMessageToAdmin(ctx context.Context, userID int64, templateCode string, params map[string]interface{}) (int64, error)
	SendSingleMessageToMember(ctx context.Context, userID int64, templateCode string, params map[string]interface{}) (int64, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **租户套餐变更权限回收校验**：当修改租户绑定的 `package_id` 或修改 `system_tenant_package.menu_ids` 时，必须在事务中遍历该租户下的所有角色：对 `tenant_admin` 角色同步增删菜单，对普通自定义角色自动删除不再属于新套餐的 `menu_id`。
- [x] **超级管理员豁免校验**：拥有 `super_admin` 角色编码的用户默认拥有所有菜单按钮权限及全部数据权限（`All = true`）。
- [x] **部门树防环校验**：更新部门 `parent_id` 时，必须检查新父部门不能是当前部门自身或其子孙部门。

# DOC-03：yudao-module-infra 基础设施模块（必选核心基础模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：全平台的研发效能引擎与基础设施运维中枢，提供 **Go + Vue3 全栈代码生成器**、**多云文件存储服务（S3/MinIO/OSS/COS/本地/FTP/SFTP/DB）**、**动态参数配置**、**在线分布式定时任务**、**RESTful API 访问日志与错误日志审计**、**多数据源管理与数据库文档导出**以及 **Redis/DB 监控面板**。
- **Go 模块路径**：`internal/modules/infra`（对外跨模块契约：`internal/api/infra`）
- **架构 Review 核心结论**：
  1. **代码生成器双端对齐**：通过解析数据库 `information_schema.tables` 与 `columns`，使用 Go `embed.FS` + `text/template` 一次性渲染产出 Go 后端代码（DO、GORM Gen Mapper、ReqVO/RespVO、Service、Gin Handler、单元测试）和原版 Vue3（Element Plus / Vben）前端代码与 SQL 菜单脚本，覆盖单表、树表与主子表三种模式。
  2. **多云文件客户端热切换**：`FileClientFactory` 维护 `configId -> FileClient` 内存映射，当管理员在后台将某条配置切换为 `master = true` 时，通过 MQ 广播通知所有实例无缝切换默认文件存储引擎。

## 二、Go 模块标准包目录结构

```text
internal/modules/infra/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/infra/*)
│   │   ├── codegen/                  # 代码生成器：数据库表列表、导入、编辑配置、在线预览、下载 ZIP
│   │   ├── config/                   # 动态参数配置 CRUD 与 Excel 导出
│   │   ├── db/                       # 多数据源配置 CRUD、连接测试、导出 HTML/Word/MD 数据库文档
│   │   ├── file/                     # 文件存储配置 CRUD、设为主配置、文件上传、S3 预签名直传、文件列表
│   │   ├── job/                      # 定时任务 CRUD、暂停/恢复、立即触发一次、任务执行日志分页
│   │   ├── logger/                   # API 访问日志分页/导出、API 错误日志分页/标记已处理
│   │   └── redis/                    # Redis 监控面板（INFO 命令指标解析、命令耗时统计、Key 数量）
│   └── app/                          # 移动端接口 (/app-api/infra/*)
│       ├── config/                   # 移动端获取公开参数配置
│       └── file/                     # 移动端文件上传与获取 S3 预签名直传地址
├── service/
│   ├── codegen/                      # 表结构解析器 (SchemaReader) 与模板渲染引擎 (CodegenEngine)
│   ├── config/
│   ├── db/
│   ├── file/
│   ├── job/
│   └── logger/
├── dal/
│   ├── dataobject/                   # 12 张 infra_* 表对应的 GORM DO 结构体
│   └── mysql/                        # GORM Gen 类型安全 DAO
├── api/                              # 实现 FileApi, ConfigApi, ApiAccessLogApi, ApiErrorLogApi
├── mq/                               # 文件主配置变更与动态参数变更广播消费者
├── job/                              # 访问日志与错误日志定期归档清理 Job
├── enums/                            # 模块错误码段 (1_001_000_000 起) 与存储/模板枚举
├── framework/
│   ├── file/                         # 内聚文件客户端：S3, Local, FTP, SFTP, DB 五种实现与工厂
│   └── codegen/                      # embed.FS 内嵌的 Go 后端与 Vue3 前端代码模板库
└── module.go                         # 导出 infra.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `infra_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `infra_codegen_table` | 全局表 | `id`, `data_source_config_id`, `scene`(1管理端/2移动端), `table_name`, `table_comment`, `class_name`, `class_comment`, `author`, `template_type`(1单表/2树表/10主子表标准/11主子表ERP/12主子表内嵌), `front_type`(10 Vue2/20 Vue3/21 Vue3 Schema/30 Vben), `module_name`, `business_name`, `parent_menu_id`, `master_table_id`, `sub_join_column_id`, `sub_join_many`, `tree_parent_column_id`, `tree_name_column_id` | `idx_table_name` (`table_name`) |
| 02 | `infra_codegen_column` | 全局表 | `id`, `table_id`, `column_name`, `data_type`, `column_comment`, `nullable`, `primary_key`, `auto_increment`, `ordinal_position`, `java_type`/`go_type`, `java_field`/`go_field`, `dict_type`, `example`, `create_operation`, `update_operation`, `list_operation`, `list_operation_condition`, `list_operation_result`, `html_type` | `idx_table_id` (`table_id`) |
| 03 | `infra_file_config` | 全局表 | `id`, `name`, `storage`(1数据库/10本地磁盘/11 FTP/12 SFTP/20 S3对象存储), `remark`, `master`(bool 全表唯一主配置), `config`(JSON：存储 endpoint/bucket/accessKey/domain 或 basePath) | `idx_master` (`master`) |
| 04 | `infra_file` | 全局表 | `id`, `config_id`, `name`(原始文件名), `path`(相对路径), `url`(完整访问地址), `type`(MIME类型), `size`(字节数) | `idx_create_time` (`create_time`) |
| 05 | `infra_file_content` | 全局表 | `id`, `config_id`, `path`, `content`(`LONGBLOB` 二进制内容，仅当 `storage = 1` 时使用) | `idx_config_path` (`config_id`, `path`) |
| 06 | `infra_config` | 全局表 | `id`, `category`, `type`(1系统内置/2自定义), `name`, `config_key`, `value`, `visible`(前端是否可读), `remark` | `uk_config_key` (`config_key`) |
| 07 | `infra_job` | 全局表 | `id`, `name`, `status`(1开启/2暂停), `handler_name`(对应 Go `JobHandler` 名称), `handler_param`, `cron_expression`, `retry_count`, `retry_interval`, `monitor_timeout` | `uk_handler_name` (`handler_name`, `handler_param`) |
| 08 | `infra_job_log` | 全局表 | `id`, `job_id`, `handler_name`, `handler_param`, `execute_index`, `begin_time`, `end_time`, `duration`(ms), `status`(0运行中/1成功/2失败), `result` | `idx_job_id` (`job_id`), `idx_begin_time` |
| 09 | `infra_api_access_log` | **租户级 (`tenant_id`)** | `id`, `trace_id`, `user_id`, `user_type`, `application_name`, `request_method`, `request_url`, `request_params`, `response_body`, `user_ip`, `user_agent`, `operate_module`, `operate_name`, `operate_type`, `begin_time`, `end_time`, `duration`, `result_code`, `result_msg` | `idx_begin_time` (`begin_time`), `idx_user_id` |
| 10 | `infra_api_error_log` | **租户级 (`tenant_id`)** | `id`, `trace_id`, `user_id`, `user_type`, `application_name`, `request_method`, `request_url`, `request_params`, `user_ip`, `user_agent`, `exception_time`, `exception_name`, `exception_message`, `exception_root_cause_message`, `exception_stack_trace`, `process_status`(0未处理/1已处理/2已忽略), `process_time`, `process_user_id` | `idx_exception_time`, `idx_process_status` |
| 11 | `infra_data_source_config` | 全局表 | `id`, `name`, `url`(DSN 连接串), `username`, `password`(AES 加密存储) | 主键索引 `id` |
| 12 | `infra_demo01` ~ `03` | **租户级 (`tenant_id`)** | 代码生成器自带的单表（`demo01_contact`）、树表（`demo02_category`）、主子表（`demo03_student` + `course` + `grade`）示例表 | 主键索引 `id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **MQ 广播刷新** | `infra.file_config.refresh` | 当后台新增/修改文件配置或切换 `master` 主配置时广播，各实例清理并重建 `FileClientFactory` 中的客户端实例 |
| **MQ 广播刷新** | `infra.config.refresh` | 当后台修改 `infra_config` 动态参数时广播，各实例刷新进程内配置缓存 |
| **内置定时任务** | `accessLogCleanJob` / `errorLogCleanJob` / `jobLogCleanJob` | 每日凌晨定期清理超过保留天数（如 14 天）的 `infra_api_access_log`、`infra_api_error_log` 与 `infra_job_log`，防止日志表无限膨胀 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **代码生成 (`codegen`)** | `GET` | `/admin-api/infra/codegen/db/table/list` | `infra:codegen:query` | 查询指定数据源下尚未导入代码生成器的物理表列表 |
| **代码生成 (`codegen`)** | `POST/PUT/DELETE` | `/admin-api/infra/codegen/{create-list,update,sync-from-db,delete}` | `infra:codegen:*` | 批量导入表结构、修改字段生成配置、同步最新数据库表结构变更 |
| **代码生成 (`codegen`)** | `GET` | `/admin-api/infra/codegen/{preview,download}` | `infra:codegen:{preview,download}` | 在线预览生成的 Go + Vue3 全套代码，或打包为 `.zip` 流式下载 |
| **文件服务 (`file`)** | `POST/PUT/DELETE/GET` | `/admin-api/infra/file-config/{create,update,update-master,delete,get,page,test}` | `infra:file-config:*` | 文件存储配置管理、设为默认主配置、测试上传连通性 |
| **文件服务 (`file`)** | `POST / GET` | `/admin-api/infra/file/{upload,presigned-url}` | 已登录即可 | 后端中转上传文件，或获取 S3 前端直传预签名 URL |
| **文件服务 (`file`)** | `GET` | `/admin-api/infra/file/{configId}/get/**` | `@PermitAll` | 本地磁盘/FTP/SFTP/DB 存储模式下的文件二进制流读取端点 |
| **定时任务 (`job`)** | `POST/PUT/DELETE/GET` | `/admin-api/infra/job/{create,update,update-status,trigger,delete,get,page}` | `infra:job:*` | 在线管理 Cron 定时任务、暂停/恢复、立即触发一次 |
| **日志与监控** | `GET / PUT` | `/admin-api/infra/api-access-log/page`, `/admin-api/infra/api-error-log/{page,update-status}` | `infra:api-*-log:*` | 查询 API 访问日志与异常堆栈日志，标记异常日志已处理 |
| **数据库文档** | `GET` | `/admin-api/infra/db-doc/export-{html,word,markdown}` | `infra:db-doc:export` | 实时扫描数据库 Schema 导出表结构设计文档 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package infra

import "context"

// FileApi 供全系统上传文件（头像、商品图、素材、附件）调用的契约
type FileApi interface {
	CreateFile(ctx context.Context, name string, path string, content []byte) (url string, err error)
}

// ConfigApi 供各模块按 Key 读取动态系统参数配置的契约
type ConfigApi interface {
	GetConfigValueByKey(ctx context.Context, key string) (string, error)
}

// ApiAccessLogApi & ApiErrorLogApi 供 pkg/web 中间件异步批量写入访问日志与异常日志
type ApiAccessLogApi interface {
	CreateApiAccessLog(ctx context.Context, req *ApiAccessLogCreateReqDTO) error
}

type ApiErrorLogApi interface {
	CreateApiErrorLog(ctx context.Context, req *ApiErrorLogCreateReqDTO) error
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **S3 预签名直传跨域与路径安全**：生成的对象存储 `path` 必须使用 `sha256(content) + ext` 或带日期前缀的安全文件名，严禁直接使用客户端传入的含 `../` 的路径，防止目录穿越漏洞。
- [x] **数据源密码加密存储**：`infra_data_source_config.password` 落库前必须经过 AES 对称加密，前端回显时脱敏或仅在连接测试时解密。

# DOC-04：yudao-module-bpm 工作流程模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：企业级工作流与审批引擎模块（参考 [工作流演示与开发指南](https://doc.iocoder.cn/bpm-preview/)），支撑全系统所有的业务审批诉求（OA 请假/报销/公文、CRM 合同与回款审批、ERP 采购/销售审核、HRM 员工转正/调岗/离职审批）。
- **Go 模块路径**：`internal/modules/bpm`（对外跨模块契约：`internal/api/bpm`）
- **架构 Review 核心结论**：
  1. **双设计器统一 AST 模型**：原项目前端同时包含 **BPMN 标准设计器**（输出 XML）和 **SIMPLE 仿钉钉/飞书设计器**（输出树形 JSON）。在 Go 端通过 `parser_bpmn.go` 和 `parser_simple.go` 将两种格式统一编译为内存中的标准有向图结构 `ProcessGraph`（存入 `bpm_engine_deployment.compiled_graph_json`），使运行时引擎完全无需关心设计器来源。
  2. **中国式复杂审批动作原生支持**：通过 5 张轻量级 Go 引擎表（`deployment`、`instance`、`execution`、`task`、`activity_log`）直接内置支持**会签（按人数或比例）、或签、依次审批、向前加签、向后加签、减签、自由驳回至任意历史节点、转办、委派、抄送、父子流程、延迟节点、HTTP 触发器与超时自动处理**。

## 二、Go 模块标准包目录结构

```text
internal/modules/bpm/
├── controller/
│   ├── admin/                        # 管理后台与 PC 审批中心接口 (/admin-api/bpm/*)
│   │   ├── definition/               # 流程分类、动态表单、流程模型(BPMN/Simple)、流程定义、用户组、表达式、监听器
│   │   ├── task/                     # 流程实例(发起/取消/详情/预测节点)、待办/已办/抄送任务、通过/拒绝/驳回/加减签/转办/委派
│   │   └── oa/                       # 内置 OA 请假审批接入示例 (BpmOALeaveController)
│   └── app/                          # 移动端审批中心接口 (/app-api/bpm/*)
│       └── task/                     # 移动端我的待办、已办、发起流程与审批操作
├── service/
│   ├── definition/                   # 模型版本管理、动态表单校验、审批候选人解析器
│   ├── task/                         # 实例与任务状态流转、抄送记录、超时提醒与事件发布
│   └── oa/
├── dal/
│   ├── dataobject/                   # 7 张业务扩展表 + 5 张 Go DAG 审批引擎核心表 DO
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # ProcessIdRedisDAO (自定义流程编号原子自增序列)
├── api/                              # 实现 BpmProcessInstanceApi（供 OA/CRM/ERP 代码发起流程）
├── mq/                               # 流程实例状态变更事件发布 (BpmProcessInstanceStatusEvent)
├── job/                              # 任务超时自动审批/自动驳回/重复催办提醒定时任务 (TaskTimeoutJob)
├── enums/                            # 模块错误码段 (1_009_000_000 起) 与节点/任务状态枚举
├── framework/
│   └── flowengine/                   # Go 原生 DAG + Token 状态机引擎核心实现
│       ├── graph_model.go            # 统一有向图 AST 模型定义
│       ├── parser_bpmn.go            # BPMN 2.0 XML -> ProcessGraph 编译器
│       ├── parser_simple.go          # 仿钉钉/飞书 Simple JSON -> ProcessGraph 编译器
│       ├── runtime_engine.go         # 状态机执行器（Token 分裂、汇聚、跳转、回退、加签）
│       └── candidate_strategy.go     # 9 种审批人分配策略执行器
└── module.go                         # 导出 bpm.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `bpm_`）

| 序号 | 表分类 | 表名 | 租户隔离 | 核心字段与设计说明 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | 流程定义扩展 | `bpm_category` | **租户级 (`tenant_id`)** | `id`, `name`, `code`, `description`, `status`, `sort`（流程分类，如人事、财务、行政） |
| 02 | 流程定义扩展 | `bpm_form` | **租户级 (`tenant_id`)** | `id`, `name`, `status`, `conf`(表单全局配置JSON), `fields`(form-create 拖拽组件 JSON 数组), `remark` |
| 03 | 流程定义扩展 | `bpm_process_definition_info` | **租户级 (`tenant_id`)** | `id`, `process_definition_id`, `model_id`, `model_type`(10 BPMN / 20 Simple), `category`, `icon`, `description`, `form_type`(10流程动态表单/20业务自定义表单), `form_id`, `form_conf`, `form_fields`, `form_custom_create_path`, `form_custom_view_path`, `simple_model`(Simple设计器JSON), `visible`, `start_user_ids`, `manager_user_ids`, `process_id_rule`, `auto_approval_type`, `title_setting`, `summary_setting` |
| 04 | 流程辅助配置 | `bpm_user_group` | **租户级 (`tenant_id`)** | `id`, `name`, `description`, `user_ids`(JSON数组), `status`（自定义审批人分组） |
| 05 | 流程辅助配置 | `bpm_process_expression` & `bpm_process_listener` | **租户级 (`tenant_id`)** | 常用条件表达式库（`name`, `status`, `expression`）与实例/任务级监听器配置（`name`, `type`, `status`, `event`, `value_type`, `value`） |
| 06 | 业务流转记录 | `bpm_process_instance_copy` | **租户级 (`tenant_id`)** | `id`, `user_id`(被抄送人), `start_user_id`, `process_instance_id`, `process_instance_name`, `category`, `activity_id`, `activity_name`, `task_id`, `reason` |
| 07 | 接入示例表 | `bpm_oa_leave` | **租户级 (`tenant_id`)** | `id`, `user_id`, `type`, `reason`, `start_time`, `end_time`, `day`, `status`, `process_instance_id` |
| 08 | **Go 引擎核心表** | `bpm_engine_deployment` | **租户级 (`tenant_id`)** | `id`(string), `model_id`, `process_key`, `name`, `category`, `version`(递增版本号), `model_type`, `bpmn_xml`, `simple_json`, `compiled_graph_json`(编译后的标准 DAG 节点图), `suspension_state`(1激活/2挂起) |
| 09 | **Go 引擎核心表** | `bpm_engine_instance` | **租户级 (`tenant_id`)** | `id`(自定义编号或UUID), `deployment_id`, `process_key`, `name`, `category`, `business_key`(业务单据ID), `start_user_id`, `status`(1审批中/2通过/3不通过/4已取消), `form_variables`(JSON), `parent_instance_id`, `start_time`, `end_time`, `duration` |
| 10 | **Go 引擎核心表** | `bpm_engine_execution` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `parent_id`, `current_node_id`, `active`(bool), `scope_variables`(JSON)（用于并行网关多分支令牌跟踪与汇聚计数） |
| 11 | **Go 引擎核心表** | `bpm_engine_task` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `execution_id`, `node_id`, `node_name`, `assignee_user_id`(处理人), `owner_user_id`(委派原持有人), `status`(1待办/2通过/3不通过/4取消/5退回/6委派中/7待加签完成), `reason`, `sign_pic_url`, `parent_task_id`, `approve_method`(1依次/2会签/3或签), `due_time`(超时时间), `timeout_handler_type`, `end_time` |
| 12 | **Go 引擎核心表** | `bpm_engine_activity_log` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `execution_id`, `node_id`, `node_name`, `node_type`, `task_id`, `assignee_id`, `status`, `start_time`, `end_time`（供前端流程图高亮轨迹） |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `bpm:process_id_seq:%d:%s` | 当流程配置开启 `process_id_rule`（自定义流程编号前缀+日期+自增序列）时，使用 Redis `INCR` 原子生成唯一流程实例 ID |
| **MQ 事件广播** | `bpm.process_instance.status` | 当流程实例最终状态变为“审批通过(2)”、“审批不通过(3)”或“已取消(4)”时发布事件，供 OA/CRM/ERP 业务模块监听并更新业务单据状态 |
| **内置定时任务** | `bpmTaskTimeoutJob` | 每分钟扫描一次 `status = 1 AND due_time <= NOW()` 的待办任务，按节点配置的超时策略自动执行“自动提醒（发送站内信/短信）”、“自动通过”或“自动驳回” |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **模型与定义** | `POST/PUT/DELETE/GET` | `/admin-api/bpm/model/{create,update,update-bpmn,delete,get,page,deploy,update-state}` | `bpm:model:*` | 流程模型 CRUD、保存 BPMN XML、发布部署新版本、激活或挂起流程定义 |
| **Simple 设计器** | `GET / POST` | `/admin-api/bpm/model/simple/{get,update}` | `bpm:model:*` | 读取与保存仿钉钉/飞书 Simple 设计器的树形 JSON 节点配置 |
| **流程定义查询** | `GET` | `/admin-api/bpm/process-definition/{list,page,get}` | `bpm:process-definition:query` | 发起流程页获取当前用户可见的激活流程定义列表及历史部署版本分页 |
| **流程实例** | `POST / DELETE` | `/admin-api/bpm/process-instance/{create,cancel-by-start-user,cancel-by-admin}` | `bpm:process-instance:*` | 发起流程实例、发起人主动撤回、管理员强制终止流程 |
| **流程实例** | `GET` | `/admin-api/bpm/process-instance/{my-page,manager-page,get-approval-detail}` | `bpm:process-instance:query` | 我的发起列表、管理后台全量实例列表、获取审批详情页完整数据（含预测节点时间线与流程图高亮） |
| **审批任务** | `GET` | `/admin-api/bpm/task/{todo-page,done-page,manager-page}` | `bpm:task:query` | 我的待办、我的已办、管理员任务监管列表 |
| **审批操作** | `PUT` | `/admin-api/bpm/task/{approve,reject,return,delegate,transfer,create-sign,delete-sign}` | `bpm:task:update` | 审批通过、审批拒绝、自由退回（配合 `get-return-list`）、委派、转办、向前/向后加签、减签 |
| **抄送列表** | `GET` | `/admin-api/bpm/process-instance/copy/page` | `bpm:process-instance-copy:query` | 分页查询抄送给我的流程记录 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package bpm

import "context"

// BpmProcessInstanceApi 供 OA / CRM / ERP / HRM 业务模块在代码中直接发起审批流的契约
type BpmProcessInstanceApi interface {
	CreateProcessInstance(ctx context.Context, userID int64, req *BpmProcessInstanceCreateReqDTO) (instanceID string, err error)
}

type BpmProcessInstanceCreateReqDTO struct {
	ProcessDefinitionKey     string                 // 流程定义标识（如 "oa_leave", "crm_contract"）
	BusinessKey              string                 // 业务表主键 ID（如合同 ID）
	Variables                map[string]interface{} // 业务表单变量（用于条件分支判断与摘要展示）
	StartUserSelectAssignees map[string][]int64     // 发起人自选审批人映射：nodeId -> userIds
}

// BpmProcessInstanceStatusEvent 流程实例终态事件载荷（各业务模块通过 MQ 订阅或注册 Listener 接收）
type BpmProcessInstanceStatusEvent struct {
	InstanceID           string `json:"id"`
	ProcessDefinitionKey string `json:"processDefinitionKey"`
	BusinessKey          string `json:"businessKey"` // 对应业务单据 ID
	Status               int    `json:"status"`      // 2-审批通过 3-审批不通过 4-已取消
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **审批人自动去重（AutoApprovalType）**：当流程开启“发起人自动通过”或“相邻节点/全局同一审批人自动去重通过”时，`createUserTasks` 检测到候选人与发起人或历史已通过人一致，应直接将该任务标记为自动通过并递归推进下一节点。
- [x] **向前加签 vs 向后加签状态机**：向前加签（`before`）时，原任务状态置为 `7（待前加签完成）`，待所有前加签子任务审批通过后，原任务重新恢复为 `1（待办）`；向后加签（`after`）时，原任务直接标记为 `2（通过）`，待所有后加签子任务通过后直接推进至流程图下一节点。
- [x] **自由退回（Return）分支清理**：当在并行分支之后执行自由退回至汇聚前的节点时，必须清理或取消当前活跃的所有兄弟分支待办任务与 `execution` 令牌，防止残留脏待办。

# DOC-05：yudao-module-pay 支付系统模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：统一收银台与多渠道资金清算基础模块（参考 [支付系统开发指南](https://doc.iocoder.cn/pay/build/)），屏蔽微信支付（V2/V3）、支付宝、会员电子钱包及模拟支付的底层接口与签名差异，向商城（`mall`）等上层模块提供**支付下单、原路退款、转账提现、电子钱包储值**及**可靠异步通知回调**。
- **Go 模块路径**：`internal/modules/pay`（对外跨模块契约：`internal/api/pay`）
- **架构 Review 核心结论**：
  1. **主支付单与渠道扩展单 1:N 解耦**：同一个 `pay_order`（关联商城订单）用户在收银台可能先点微信支付未付款、再切回点支付宝扫码付款。每次点击拉起渠道均生成一条具有独立外部单号 `no` 的 `pay_order_extension`，确保渠道单号不冲突；若极端情况下用户两边都完成了付款，后回调的一笔由于主单已处于 `已支付(10)` 状态，系统将自动识别为重复支付并触发原路退款或告警。
  2. **可靠异步回调通知（Outbox 模式）**：三方支付回调成功后，在同一数据库事务内写入 `pay_notify_task` 表，再由后台协程向业务系统的 `notify_url`（或进程内 Event）发起通知，失败时按 `[15s, 15s, 30s, 180s, 1800s, 1800s, 1800s, 3600s]` 阶梯重试。

## 二、Go 模块标准包目录结构

```text
internal/modules/pay/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/pay/*)
│   │   ├── app/                      # 支付应用 CRUD、状态切换
│   │   ├── channel/                  # 支付渠道配置 CRUD（微信/支付宝证书与密钥配置）
│   │   ├── demo/                     # 支付、退款与转账接入示例订单
│   │   ├── notify/                   # 三方支付/退款/转账异步回调免登网关 + 回调任务日志查询
│   │   ├── order/                    # 支付订单分页、详情、收银台提交、Excel 导出
│   │   ├── refund/                   # 退款订单分页、详情、Excel 导出
│   │   ├── transfer/                 # 转账提现单创建、分页、详情
│   │   └── wallet/                   # 会员电子钱包账户管理、余额流水、充值记录与充值套餐配置
│   └── app/                          # 移动端消费者接口 (/app-api/pay/*)
│       ├── channel/                  # 获取当前应用已开启的支付渠道编码列表
│       ├── order/                    # 移动端收银台获取订单详情与提交渠道支付
│       ├── transfer/                 # 移动端查询转账单状态（微信确认收款包）
│       └── wallet/                   # 移动端查询我的钱包余额、交易流水、发起充值与充值套餐列表
├── service/
│   ├── app/
│   ├── channel/
│   ├── demo/
│   ├── notify/
│   ├── order/
│   ├── refund/
│   ├── transfer/
│   └── wallet/
├── dal/
│   ├── dataobject/                   # 12 张 pay_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # PayNoRedisDAO (生成唯一支付/退款/转账外部单号序列), NotifyLockRedisDAO
├── api/                              # 实现 PayOrderApi, PayRefundApi, PayTransferApi, PayWalletApi
├── mq/                               # 支付回调通知任务异步执行消费者
├── job/                              # 过期未支付订单自动关单 Job、退款/转账结果主动轮询同步 Job、回调重试 Job
├── enums/                            # 模块错误码段 (1_007_000_000 起) 与渠道/状态枚举
├── framework/
│   └── pay/                          # 内聚多渠道支付客户端：WechatPayClient, AlipayClient, WalletPayClient, MockPayClient
└── module.go                         # 导出 pay.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `pay_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `pay_app` | **租户级 (`tenant_id`)** | `id`, `app_key`(唯一应用标识，如 `mall`), `name`, `status`, `remark`, `order_notify_url`, `refund_notify_url`, `transfer_notify_url` | `uk_app_key` (`app_key`, `tenant_id`) |
| 02 | `pay_channel` | **租户级 (`tenant_id`)** | `id`, `code`(`wx_pub`/`wx_lite`/`wx_app`/`wx_native`/`wx_wap`/`alipay_pc`/`alipay_wap`/`alipay_app`/`alipay_qr`/`alipay_bar`/`wallet`/`mock`), `status`, `remark`, `fee_rate`, `app_id`, `config`(JSON：存储商户号与公私钥证书) | `uk_app_code` (`app_id`, `code`) |
| 03 | `pay_order` | **租户级 (`tenant_id`)** | `id`, `app_id`, `channel_id`, `channel_code`, `merchant_order_id`(业务订单ID), `subject`, `body`, `notify_url`, `price`(分), `channel_fee_rate`, `channel_fee_price`, `status`(0未支付/10已支付/20已退款/30已关闭), `user_ip`, `expire_time`, `success_time`, `extension_id`, `no`, `refund_price`(累计已退分), `channel_user_id`, `channel_order_no` | `idx_app_merchant` (`app_id`, `merchant_order_id`) |
| 04 | `pay_order_extension` | **租户级 (`tenant_id`)** | `id`, `no`(外部商户单号 `out_trade_no` 如 `P2026110400001`), `order_id`, `channel_id`, `channel_code`, `user_ip`, `status`(0待支付/1已支付/2已关闭), `channel_extras`(JSON), `channel_error_code`, `channel_error_msg`, `channel_notify_data` | `uk_no` (`no`), `idx_order_id` (`order_id`) |
| 05 | `pay_refund` | **租户级 (`tenant_id`)** | `id`, `no`(外部退款单号 `R20261104...`), `app_id`, `channel_id`, `channel_code`, `order_id`, `order_no`, `merchant_order_id`, `merchant_refund_id`, `notify_url`, `status`(0待退款/10成功/20失败), `pay_price`, `refund_price`, `reason`, `user_ip`, `channel_order_no`, `channel_refund_no`, `success_time`, `channel_error_code`, `channel_error_msg`, `channel_notify_data` | `uk_no` (`no`), `idx_order_id` |
| 06 | `pay_transfer` | **租户级 (`tenant_id`)** | `id`, `no`(外部转账单号 `T20261104...`), `app_id`, `channel_id`, `channel_code`, `merchant_transfer_id`, `type`(1支付宝/2微信零钱/3银行卡/4电子钱包), `status`(0待转账/10转账中/20成功/30失败), `price`, `subject`, `user_name`, `alipay_logon_id`, `openid`, `notify_url`, `channel_transfer_no`, `success_time` | `uk_no` (`no`) |
| 07 | `pay_wallet` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `balance`(可用余额:分), `freeze_price`(提现或消费冻结:分), `total_expense`, `total_recharge` | `uk_user` (`user_id`, `user_type`, `tenant_id`) |
| 08 | `pay_wallet_transaction` | **租户级 (`tenant_id`)** | `id`, `wallet_id`, `biz_type`(1充值/2充值退款/3支付/4支付退款/5提现/6后台调整), `biz_id`, `no`, `title`, `price`(正负变动分), `balance`(变动后余额快照) | `idx_wallet_id` (`wallet_id`) |
| 09 | `pay_wallet_recharge` | **租户级 (`tenant_id`)** | `id`, `wallet_id`, `total_price`(实际到账总金额), `pay_price`(实付金额), `bonus_price`(赠送金额), `package_id`, `pay_status`, `pay_order_id`, `pay_channel_code`, `pay_time`, `refund_status`, `pay_refund_id`, `refund_total_price` | `idx_wallet_id` |
| 10 | `pay_wallet_recharge_package` | **租户级 (`tenant_id`)** | `id`, `name`, `pay_price`, `bonus_price`, `status` | 主键索引 `id` |
| 11 | `pay_notify_task` & `pay_notify_log` | **租户级 (`tenant_id`)** | `id`, `app_id`, `type`(1支付/2退款/3转账), `data_id`, `merchant_order_id`, `merchant_refund_id`, `merchant_transfer_id`, `status`(1等待/2成功/3失败/4请求成功但业务返回失败), `next_notify_time`, `last_execute_time`, `notify_times`, `max_notify_times`, `notify_url` 及每次回调响应日志 | `idx_next_notify` (`status`, `next_notify_time`) |
| 12 | `pay_demo_order` & `pay_demo_transfer` | **租户级 (`tenant_id`)** | 支付、退款与转账接入示例业务表 | 主键索引 `id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `pay:no_seq:%s` | 生成唯一支付外部单号（`P+yyyyMMddHHmmss+自增序列`）、退款单号（`R...`）、转账单号（`T...`）与钱包流水号（`W...`） |
| **Redis 分布式锁** | `pay:order:lock:%d` / `pay:notify:lock:%d` | 保护支付单提交与三方回调并发修改，以及防止多个定时任务实例同时执行同一条 `pay_notify_task` |
| **内置定时任务** | `payOrderExpireJob` / `payOrderSyncJob` | 自动关闭超时未支付的 `pay_order`；对创建时间超过 5 分钟仍处于“待支付”的扩展单主动调用微信/支付宝查单 API 兜底同步状态 |
| **内置定时任务** | `payRefundSyncJob` / `payTransferSyncJob` / `payNotifyJob` | 定期主动向三方渠道轮询处理中的退款单与转账单状态；扫描到期未完成的 `pay_notify_task` 执行重试回调 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 端类型 / 权限 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **应用与渠道** | `POST/PUT/DELETE/GET` | `/admin-api/pay/app/*`, `/admin-api/pay/channel/*` | `pay:app:*`, `pay:channel:*` | 支付应用与各支付渠道证书参数管理 |
| **收银台提交** | `POST` | `/admin-api/pay/order/submit`, `/app-api/pay/order/submit` | 双端已登录 | 选择渠道提交支付，返回 `{status, displayMode, displayContent}`（如二维码链接、唤醒微信小程序 JSAPI JSON 参数） |
| **三方回调网关** | `POST` | `/admin-api/pay/notify/{order,refund,transfer}/{channelId}` | `@PermitAll` 免登 | 接收微信支付 V2(XML)/V3(JSON+AES-GCM) 与支付宝(Form+RSA2) 的异步回调通知 |
| **退款与转账** | `GET / POST` | `/admin-api/pay/refund/*`, `/admin-api/pay/transfer/*` | `pay:refund:*`, `pay:transfer:*` | 查询退款单分页/详情、发起转账单与查询转账明细 |
| **会员电子钱包** | `GET / POST` | `/app-api/pay/wallet/get`, `/app-api/pay/wallet-transaction/page`, `/app-api/pay/wallet-recharge/create` | 移动端已登录 | C 端会员查询钱包余额、资金明细流水、创建钱包充值订单 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package pay

import "context"

// PayOrderApi 供商城交易、钱包充值等模块调用的支付单接口
type PayOrderApi interface {
	CreateOrder(ctx context.Context, req *PayOrderCreateReqDTO) (orderID int64, err error)
	GetOrder(ctx context.Context, id int64) (*PayOrderRespDTO, error)
	UpdatePayOrderPrice(ctx context.Context, id int64, payPrice int) error
}

// PayRefundApi 供商城售后模块调用的退款单接口
type PayRefundApi interface {
	CreateRefund(ctx context.Context, req *PayRefundCreateReqDTO) (refundID int64, err error)
	GetRefund(ctx context.Context, id int64) (*PayRefundRespDTO, error)
}

// PayTransferApi 供商城分销佣金提现调用的转账单接口
type PayTransferApi interface {
	CreateTransfer(ctx context.Context, req *PayTransferCreateReqDTO) (transferID int64, err error)
	GetTransfer(ctx context.Context, id int64) (*PayTransferRespDTO, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **部分退款与累计退款上限校验**：发起退款时，必须校验 `payOrder.RefundPrice + req.RefundPrice <= payOrder.Price`，防止多次部分退款总额超过原实付金额。
- [x] **电子钱包扣款防并发超支**：使用会员钱包渠道（`channel_code = "wallet"`）支付时，扣减余额 SQL 必须携带乐观条件 `UPDATE pay_wallet SET balance = balance - ?, total_expense = total_expense + ? WHERE id = ? AND balance >= ?`。

# DOC-06：yudao-module-member 会员中心模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：面向 C 端消费者（微信小程序、H5、App）的统一账号与会员运营基础模块（参考 [会员中心开发指南](https://doc.iocoder.cn/member/build/)），为商城（`mall`）、支付钱包（`pay`）、即时通讯（`im`）提供 C 端身份认证、收货地址、用户画像（标签/分组）以及会员留存体系（等级、成长值、积分、签到）。
- **Go 模块路径**：`internal/modules/member`（对外跨模块契约：`internal/api/member`）
- **架构 Review 核心结论**：
  1. **基础设施复用与物理表隔离**：`member` 模块的登录 Token 签发直接调用 `internal/api/system.OAuth2TokenApi`（传入 `UserType = 2`），微信 OpenID 绑定直接调用 `internal/api/system.SocialUserApi`，短信验证码直接调用 `internal/api/system.SmsCodeApi`，既复用了 `system` 模块全部安全认证能力，又保证了 `member_user` 表与后台 `system_users` 表彻底解耦。
  2. **积分与成长值强一致性**：积分变动与经验值升级必须在数据库事务内完成，并记录不可篡改的 `member_point_record` 与 `member_experience_record` 流水。

## 二、Go 模块标准包目录结构

```text
internal/modules/member/
├── controller/
│   ├── admin/                        # 管理后台运营接口 (/admin-api/member/*)
│   │   ├── address/                  # 查询指定会员的收货地址列表
│   │   ├── config/                   # 会员积分抵扣与赠送全局配置
│   │   ├── group/                    # 会员分组 CRUD
│   │   ├── level/                    # 会员等级配置 CRUD、等级变动记录与经验值记录分页
│   │   ├── point/                    # 会员积分变动明细分页
│   │   ├── signin/                   # 连续签到天数奖励规则 CRUD、会员签到记录分页
│   │   ├── tag/                      # 会员标签 CRUD
│   │   └── user/                     # 会员档案分页、详情、修改资料、手动调整会员等级与积分
│   └── app/                          # C 端消费者接口 (/app-api/member/*)
│       ├── address/                  # 我的收货地址 CRUD、获取默认地址
│       ├── auth/                     # 账密登录、短信验证码登录、社交/微信小程序一键登录、登出、刷新 Token
│       ├── level/                    # 获取会员等级规则列表与我的经验值记录
│       ├── point/                    # 获取我的积分明细分页
│       ├── signin/                   # 获取签到规则、获取个人签到统计（连续签到天数/今日是否已签）、立即签到打卡
│       ├── social/                   # C 端绑定/解绑微信、获取微信小程序码与订阅消息模板
│       └── user/                     # 获取我的会员信息、修改昵称头像性别生日、换绑手机号、修改/重置密码
├── service/
│   ├── address/
│   ├── auth/
│   ├── config/
│   ├── group/
│   ├── level/
│   ├── point/
│   ├── signin/
│   ├── tag/
│   └── user/
├── dal/
│   ├── dataobject/                   # 11 张 member_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # SignInLockRedisDAO (防并发重复签到锁)
├── api/                              # 实现 MemberUserApi, MemberLevelApi, MemberPointApi, MemberAddressApi, MemberConfigApi
├── enums/                            # 模块错误码段 (1_004_000_000 起) 与积分/经验业务类型枚举
├── convert/                          # DO <-> DTO <-> VO 转换函数
└── module.go                         # 导出 member.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（11 张表，前缀 `member_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `member_user` | **租户级 (`tenant_id`)** | `id`, `mobile`, `password`(BCrypt), `status`(0正常/1禁用), `register_ip`, `register_terminal`, `login_ip`, `login_date`, `nickname`, `avatar`, `name`, `sex`, `area_id`, `birthday`, `mark`, `point`(可用积分), `tag_ids`(JSON数组), `level_id`, `experience`(成长值), `group_id` | `uk_mobile` (`mobile`, `tenant_id`, `deleted`) |
| 02 | `member_address` | **租户级 (`tenant_id`)** | `id`, `user_id`, `name`(收件人), `mobile`, `area_id`(省市区ID), `detail_address`, `default_status`(bool) | `idx_user_id` (`user_id`) |
| 03 | `member_tag` | **租户级 (`tenant_id`)** | `id`, `name` | `uk_name` (`name`, `tenant_id`) |
| 04 | `member_group` | **租户级 (`tenant_id`)** | `id`, `name`, `remark`, `status` | `uk_name` (`name`, `tenant_id`) |
| 05 | `member_level` | **租户级 (`tenant_id`)** | `id`, `name`, `level`(等级序号), `experience`(升级所需经验门槛), `discount_percent`(折扣率如 95), `icon`, `background_url`, `status` | `uk_level` (`level`, `tenant_id`) |
| 06 | `member_level_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `level_id`, `level`, `discount_percent`, `experience`, `user_experience`, `remark`, `description` | `idx_user_id` (`user_id`) |
| 07 | `member_experience_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `biz_id`, `biz_type`, `title`, `description`, `experience`(正负变动值), `total_experience`(变动后快照) | `idx_user_id` (`user_id`) |
| 08 | `member_config` | **租户级 (`tenant_id`)** | `id`, `point_trade_deduct_enable`, `point_trade_deduct_unit_price`, `point_trade_deduct_max_price`, `point_trade_give_point` | 单租户单行配置 |
| 09 | `member_point_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `biz_id`, `biz_type`, `title`, `description`, `point`(正负变动积分), `total_point`(变动后快照) | `idx_user_id` (`user_id`) |
| 10 | `member_sign_in_config` | **租户级 (`tenant_id`)** | `id`, `day`(连续第 N 天), `point`(奖励积分), `experience`(奖励经验), `status` | `uk_day` (`day`, `tenant_id`) |
| 11 | `member_sign_in_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `day`, `point`, `experience` | `idx_user_create` (`user_id`, `create_time`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **Redis 分布式锁** | `member:signin:lock:%d` | 会员点击每日签到时加锁（锁粒度：`userID`），防止高并发双击刷取多份今日签到积分与经验 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 端类型 / 权限 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **C端认证** | `POST` | `/app-api/member/auth/{login,sms-login,social-login,weixin-mini-app-login,logout,refresh-token}` | `@PermitAll` | 覆盖手机号密码登录、短信验证码自动注册登录、公众号/小程序授权一键登录与 Token 刷新 |
| **C端个人中心** | `GET / PUT` | `/app-api/member/user/{get,update,update-mobile,update-password,reset-password}` | 移动端已登录 / `@PermitAll`(重置密码) | 查询个人资料（含等级图标、积分、经验）、修改头像昵称、短信验证换绑手机 |
| **C端收货地址** | `POST/PUT/DELETE/GET` | `/app-api/member/address/{create,update,delete,get,list,get-default}` | 移动端已登录 | 管理个人收货地址与默认地址切换 |
| **C端签到与积分** | `POST / GET` | `/app-api/member/sign-in/record/{create,get-summary,page}`, `/app-api/member/point/record/page` | 移动端已登录 | 每日签到打卡、查询连续签到统计与积分收支流水 |
| **管理端会员运营** | `GET / PUT` | `/admin-api/member/user/{page,get,update,update-level,update-point}` | `member:user:*` | 后台多条件筛选会员（按标签/分组/等级/注册时间）、手动调整会员等级与积分 |
| **管理端规则配置** | `POST/PUT/DELETE/GET` | `/admin-api/member/{tag,group,level,sign-in/config,config}/*` | `member:*` | 标签、分组、等级门槛、连续签到周期奖励与积分抵扣规则管理 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package member

import "context"

// MemberUserApi 供商城、支付、IM、统计模块查询 C 端会员信息的契约
type MemberUserApi interface {
	GetUser(ctx context.Context, id int64) (*MemberUserRespDTO, error)
	GetUserList(ctx context.Context, ids []int64) ([]*MemberUserRespDTO, error)
	GetUserListByNickname(ctx context.Context, nickname string) ([]*MemberUserRespDTO, error)
	GetUserByMobile(ctx context.Context, mobile string) (*MemberUserRespDTO, error)
}

// MemberPointApi 供商城下单抵扣、取消订单退还、支付成功奖励调用的积分契约
type MemberPointApi interface {
	AddPoint(ctx context.Context, userID int64, point int, bizType int, bizID string) error
	ReducePoint(ctx context.Context, userID int64, point int, bizType int, bizID string) error
}

// MemberLevelApi 供商城计算 VIP 会员价与支付成功发放经验值调用的等级契约
type MemberLevelApi interface {
	GetMemberLevel(ctx context.Context, id int64) (*MemberLevelRespDTO, error)
	AddExperience(ctx context.Context, userID int64, experience int, bizType int, bizID string) error
}

// MemberAddressApi 供商城下单计算快递运费与收货人信息调用的地址契约
type MemberAddressApi interface {
	GetAddress(ctx context.Context, id int64, userID int64) (*MemberAddressRespDTO, error)
	GetDefaultAddress(ctx context.Context, userID int64) (*MemberAddressRespDTO, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **连续签到断签重置与周期循环检查**：若昨日未签到，今日签到连续天数 `day` 重置为 `1`；若昨日已签（连续第 `k` 天），且配置的最大签到周期为 `N` 天（如 7 天一循环），当 `k >= N` 时今日签到重置为第 `1` 天，否则为 `k + 1` 天。
- [x] **默认收货地址唯一性检查**：当新增或修改地址设置 `default_status = true` 时，必须在同一事务内先将该用户原有 `default_status = true` 的地址更新为 `false`。

# DOC-07：yudao-module-report 大屏与数据报表模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：企业级可视化数据大屏（GoView）与在线报表设计器基础模块（参考 [报表与大屏开发指南](https://doc.iocoder.cn/report/)）。
- **Go 模块路径**：`internal/modules/report`
- **架构 Review 核心结论**：
  1. **GoView 前端零改动无缝对接**：配套大屏前端项目 `yudao-ui-go-view` 通过 `/admin-api/report/go-view/**` 与后端交互，共用 `system` 模块的 Bearer Token 鉴权体系。
  2. **动态 SQL 与 HTTP 代理安全加固**：
     - 针对 `/admin-api/report/go-view/data/get-by-sql`：禁止多语句执行（`;`），禁止 DML/DDL 及系统表/敏感表（`system_users`、`system_oauth2_*`）查询，强制在 5 秒超时只读事务（`sql.TxOptions{ReadOnly: true}`）中运行，并由 GORM 多租户插件确保跨租户数据隔离。
     - 针对 `/admin-api/report/go-view/data/get-by-http`：对目标 URL 的解析 IP 进行 **SSRF（服务端请求伪造）内网地址段拦截**（禁止访问 `127.0.0.0/8`、`10.0.0.0/8`、`169.254.169.254` 云元数据地址等）。

## 二、Go 模块标准包目录结构

```text
internal/modules/report/
├── controller/
│   └── admin/                        # 管理后台与 GoView 设计器接口 (/admin-api/report/*)
│       ├── goview/
│       │   ├── go_view_project_handler.go  # 大屏项目 CRUD 与我的大屏分页
│       │   ├── go_view_data_handler.go     # SQL 动态数据集查询与 HTTP 代理数据集查询
│       │   └── go_view_vo.go               # GoView 专属 ReqVO / RespVO
│       └── jimu/
│           └── jimu_report_handler.go      # 报表设计器 Token 鉴权与自定义数据集端点
├── service/
│   └── goview/
│       ├── go_view_project_service.go
│       └── go_view_data_service.go         # SQL 安全沙箱与 SSRF 防护 HTTP 代理
├── dal/
│   ├── dataobject/
│   │   └── go_view_project_do.go           # report_go_view_project 表 DO
│   └── mysql/
│       └── go_view_project_mapper.go
├── enums/                            # 模块错误码段 (1_003_000_000 起)
├── convert/
└── module.go                         # 导出 report.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（前缀 `report_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `report_go_view_project` | **租户级 (`tenant_id`)** | `id`(主键), `name`(大屏项目名称), `status`(0已发布/1未发布), `content`(`LONGTEXT` 存储 GoView 画布全部图表组件与坐标配置 JSON), `pic_url`(大屏缩略预览图 URL), `remark`, `creator`(创建人用户ID) | `idx_creator_tenant` (`creator`, `tenant_id`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

- **大屏查询结果短缓存（`GoViewDataRedisDAO`）**：对于高频刷新的大屏 SQL 查询（如每 5 秒轮询一次的监控大屏），以 `report:goview:sql_cache:{tenantId}:{md5(sql+params)}` 为 Key 缓存结果 `3~5 秒`，避免大屏多终端同时打开时击穿数据库。

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **大屏项目管理** | `POST` | `/admin-api/report/go-view/project/create` | `report:go-view-project:create` | 新建大屏项目（初始状态为 `1-未发布`），返回项目 `id` |
| **大屏项目管理** | `PUT` | `/admin-api/report/go-view/project/update` | `report:go-view-project:update` | 更新大屏名称、发布状态（`status`）、预览图（`picUrl`）或画布 JSON（`content`） |
| **大屏项目管理** | `DELETE` | `/admin-api/report/go-view/project/delete` | `report:go-view-project:delete` | 删除指定 `id` 的大屏项目 |
| **大屏项目管理** | `GET` | `/admin-api/report/go-view/project/get` | `report:go-view-project:query` | 获取指定 `id` 的大屏项目完整详情（含 `content` 画布 JSON） |
| **大屏项目管理** | `GET` | `/admin-api/report/go-view/project/my-page` | `report:go-view-project:query` | 分页获取当前登录管理员创建的大屏列表 |
| **大屏动态数据** | `POST` | `/admin-api/report/go-view/data/get-by-sql` | `report:go-view-data:get-by-sql` | 在只读安全沙箱中执行 SELECT 查询，返回 `{header: string[], source: map[string]any[]}` |
| **大屏动态数据** | `POST` | `/admin-api/report/go-view/data/get-by-http` | `report:go-view-data:get-by-http` | 后端安全代理转发 HTTP 数据集请求并返回响应 JSON |

## 六、核心业务逻辑 Go 源码实现（SQL 沙箱与防 SSRF HTTP 代理）

```go
package goview

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"gorm.io/gorm"
)

var sqlBlacklistRegex = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE|GRANT|REVOKE|INTO\s+OUTFILE|LOAD_FILE|INFORMATION_SCHEMA|PERFORMANCE_SCHEMA|MYSQL|SLEEP|BENCHMARK|SYSTEM_USERS|SYSTEM_OAUTH2)\b`)

type GoViewDataRespVO struct {
	Header []string                 `json:"header"`
	Source []map[string]interface{} `json:"source"`
}

// GetDataBySQL 安全只读执行大屏 SQL 查询
func (s *GoViewDataService) GetDataBySQL(ctx context.Context, rawSQL string) (*GoViewDataRespVO, error) {
	trimmed := strings.TrimSpace(rawSQL)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT ") || strings.Contains(trimmed, ";") {
		return nil, errors.New("大屏 SQL 仅支持单条 SELECT 语句")
	}
	if sqlBlacklistRegex.MatchString(trimmed) {
		return nil, errors.New("SQL 包含受限系统表或危险函数")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	source := make([]map[string]interface{}, 0)
	err := s.db.WithContext(timeoutCtx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(trimmed).Scan(&source).Error
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	header := make([]string, 0)
	if len(source) > 0 {
		for col := range source[0] {
			header = append(header, col)
		}
	}
	return &GoViewDataRespVO{Header: header, Source: source}, nil
}

// ValidateExternalURL 校验 HTTP 数据集目标地址，阻断针对内网与云元数据的 SSRF 攻击
func ValidateExternalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("非法的 HTTP/HTTPS 地址")
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("无法解析目标域名")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("安全拦截：禁止大屏 HTTP 数据源访问内网私有 IP 地址")
		}
	}
	return nil
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **大屏未发布状态访问控制**：未发布（`status = 1`）的大屏仅允许创建人或拥有后台管理权限的人员预览；生产投屏查看时校验大屏必须处于已发布（`status = 0`）状态。
- [x] **SSRF 与慢 SQL 熔断测试**：单测覆盖传入 `SELECT SLEEP(10)`、`SELECT * FROM system_users` 以及 `http://127.0.0.1:8080` 时的拦截断言。

# DOC-08：yudao-module-mp 微信公众号模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：多微信公众号集中托管运营基础模块（参考 [微信公众号开发指南](https://doc.iocoder.cn/mp/build/)），支持在同一个后台接入并管理多个服务号/订阅号，涵盖账号配置、粉丝同步与标签管理、消息接收与人工客服主动回复、模板消息、自动回复（关注回复/消息兜底回复/关键词精准或半匹配回复）、自定义菜单、永久/临时素材库、图文草稿箱与发表记录、公众号数据统计。
- **Go 模块路径**：`internal/modules/mp`
- **架构 Review 核心结论**：
  1. **多实例微信 AccessToken 分布式共享**：在集群部署下，多个 Go 实例调用微信接口必须共享同一个微信公众号 `access_token`（否则相互刷新会导致旧 Token 失效报错 `40001`）。通过实现 `silenceper/wechat/v2/cache.Cache` 接口并底层桥接 `pkg/cache`（Redis），实现全集群 `access_token` 统一缓存与分布式锁刷新。
  2. **微信临时媒体素材自动转存 S3**：粉丝发送给公众号的图片、语音、视频在微信服务器仅保留 3 天（临时素材 `media_id`）。收到粉丝多媒体消息时，后台异步协程自动调用微信 Media Download API 下载二进制流，并通过 `internal/api/infra.FileApi` 上传至自有 S3 存储，将永久 URL 写入 `mp_message.media_url`。

## 二、Go 模块标准包目录结构

```text
internal/modules/mp/
├── controller/
│   └── admin/                        # 管理后台与微信开放回调接口 (/admin-api/mp/*)
│       ├── account/                  # 公众号账号 CRUD、生成二维码、清零 API 配额
│       ├── autoreply/                # 自动回复规则 CRUD（关注时回复、收到消息回复、关键词回复）
│       ├── draft/                    # 公众号图文草稿箱 CRUD（直接对接微信 Draft API）
│       ├── freepublish/              # 公众号图文发表记录分页、提交群发发表与删除
│       ├── material/                 # 永久素材（图片/语音/视频）与临时素材上传、分页、删除
│       ├── menu/                     # 自定义菜单树保存（同步发布至微信）与删除清空
│       ├── message/                  # 粉丝消息历史分页、人工客服主动向粉丝发送消息、模板消息推送
│       ├── open/                     # 微信服务器 GET 签名校验与 POST XML 消息/事件回调入口 (/admin-api/mp/open/:appId)
│       ├── statistics/               # 粉丝增减、累计用户、消息概况、接口分析统计查询
│       ├── tag/                      # 公众号标签 CRUD 与全量同步
│       └── user/                     # 公众号粉丝分页、详情、修改备注与打标签、异步全量同步粉丝
├── service/
│   ├── account/
│   ├── autoreply/
│   ├── material/
│   ├── menu/
│   ├── message/
│   ├── open/                         # 微信被动消息与事件回调责任链分发器
│   ├── statistics/
│   ├── tag/
│   └── user/
├── dal/
│   ├── dataobject/                   # 7 张 mp_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # WechatAccessTokenRedisCache (实现 wechat/v2 cache.Cache 接口)
├── mq/                               # 公众号账号配置变更广播消费者（刷新各节点 MpClientFactory）
├── enums/                            # 模块错误码段 (1_006_000_000 起) 与微信事件/消息类型常量
├── framework/
│   └── mp/                           # MpClientFactory 多公众号客户端实例池与图文素材转换器
└── module.go                         # 导出 mp.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（7 张表，前缀 `mp_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `mp_account` | **租户级 (`tenant_id`)** | `id`, `name`(公众号名称), `account`(微信号), `app_id`(微信 AppID), `app_secret`, `token`, `aes_key`(消息加解密密钥), `qr_code_url`(公众号二维码 URL), `remark` | `uk_app_id` (`app_id`) |
| 02 | `mp_user` | **租户级 (`tenant_id`)** | `id`, `openid`, `union_id`, `subscribe_status`(0已关注/1已取关), `subscribe_time`, `unsubscribe_time`, `nickname`, `head_image_url`, `language`, `country`, `province`, `city`, `remark`, `tag_ids`(JSON数组), `account_id`, `app_id` | `uk_app_openid` (`app_id`, `openid`) |
| 03 | `mp_tag` | **租户级 (`tenant_id`)** | `id`, `wx_tag_id`(微信端返回的标签ID), `name`, `count`(粉丝数), `account_id`, `app_id` | `idx_account_id` (`account_id`) |
| 04 | `mp_message` | **租户级 (`tenant_id`)** | `id`, `msg_id`(微信消息ID), `account_id`, `app_id`, `user_id`(关联 `mp_user.id`), `openid`, `type`(`text`/`image`/`voice`/`video`/`shortvideo`/`location`/`link`/`music`/`news`), `send_from`(1粉丝发给公众号/2公众号发给粉丝), `content`, `media_id`, `media_url`(转存 S3 后的永久 URL), `recognition`(语音识别文本), `format`, `title`, `description`, `thumb_media_id`, `thumb_media_url`, `url`, `location_x`, `location_y`, `scale`, `label`, `articles`(图文 JSON), `music_url`, `hq_music_url`, `event`, `event_key` | `idx_account_user` (`account_id`, `user_id`, `create_time`) |
| 05 | `mp_auto_reply` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `type`(1关注时回复/2收到消息回复/3关键词回复), `request_keyword`, `request_match`(1全匹配/2半匹配包含), `request_message_type`, `response_message_type`, `response_content`, `response_media_id`, `response_media_url`, `response_title`, `response_description`, `response_thumb_media_id`, `response_thumb_media_url`, `response_articles`, `response_music_url`, `response_hq_music_url` | `idx_account_type` (`account_id`, `type`) |
| 06 | `mp_menu` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `name`, `menu_key`, `parent_id`, `type`(`view`/`click`/`miniprogram`/`scancode_waitmsg`/`pic_sysphoto`等), `url`, `mini_program_app_id`, `mini_program_page_path`, `article_id`, `reply_message_type`, `reply_content`, `reply_media_id`, `reply_media_url`, `reply_articles` | `idx_account_parent` (`account_id`, `parent_id`) |
| 07 | `mp_material` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `media_id`(微信素材ID), `type`(`image`/`voice`/`video`/`thumb`), `permanent`(bool 是否永久素材), `url`(转存自有 S3 地址), `name`, `mp_url`(微信端图片URL), `title`, `introduction` | `idx_account_media` (`account_id`, `media_id`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `mp:wechat_access_token:%s` | 缓存指定 `appId` 的微信公众号 `access_token`（TTL 7000 秒），供全集群实例共享调用 |
| **RedisDAO** | `mp:msg_dedup:%d` | 微信服务器若在 5 秒内未收到响应会重试推送 3 次相同 `MsgId` 的 XML 报文；使用 `SETNX mp:msg_dedup:{msgId}`（TTL 60 秒）实现回调幂等去重 |
| **MQ 广播刷新** | `mp.account.refresh` | 当后台新增、修改或删除 `mp_account` 时广播，各实例刷新 `MpClientFactory` 中的 `OfficialAccount` 实例 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **微信开放回调** | `GET / POST` | `/admin-api/mp/open/{appId}` | `@PermitAll` | `GET` 处理微信公众号服务器 URL 签名验证（`echostr`）；`POST` 接收微信推送的加密/明文 XML 消息与事件 |
| **账号管理** | `POST/PUT/DELETE/GET` | `/admin-api/mp/account/{create,update,delete,get,page,list-simple,generate-qr-code,clear-quota}` | `mp:account:*` | 公众号账号 CRUD、一键生成公众号二维码、清零公众号每月 API 调用配额 |
| **粉丝管理** | `GET / PUT / POST` | `/admin-api/mp/user/{page,get,update,sync}` | `mp:user:*` | 粉丝分页与详情、更新粉丝备注与标签、异步全量从微信服务器拉取同步粉丝 |
| **标签管理** | `POST/PUT/DELETE/GET` | `/admin-api/mp/tag/{create,update,delete,page,list-simple,sync}` | `mp:tag:*` | 公众号标签 CRUD（同步调用微信标签 API）与从微信同步标签 |
| **消息与自动回复** | `GET / POST` | `/admin-api/mp/message/{page,send}`, `/admin-api/mp/auto-reply/{create,update,delete,get,page}` | `mp:message:*`, `mp:auto-reply:*` | 查看粉丝互動消息历史、人工客服主动发送消息、配置关注/消息/关键词自动回复规则 |
| **自定义菜单** | `GET / POST / DELETE` | `/admin-api/mp/menu/{list,save,delete}` | `mp:menu:*` | 获取菜单树、保存并发布自定义菜单至微信、清空公众号菜单 |
| **素材与草稿发布** | `POST/PUT/DELETE/GET` | `/admin-api/mp/material/*`, `/admin-api/mp/draft/*`, `/admin-api/mp/free-publish/*` | `mp:material:*`, `mp:draft:*`, `mp:free-publish:*` | 上传永久与临时素材、管理图文草稿箱、将草稿正式群发发表 |
| **数据统计** | `GET` | `/admin-api/mp/statistics/{user-summary,user-cumulate,upstream-message,interface-summary}` | `mp:statistics:query` | 查询指定时间区间的粉丝增减、累计粉丝、消息发送量与接口耗时分析 |

## 六、核心业务逻辑 Go 源码实现（多公众号实例池与微信回调责任链）

```go
package open

import (
	"context"
	"sync"
	"time"
	"github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	"github.com/silenceper/wechat/v2/officialaccount"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
	"oneweekonething.com/owot-pro/pkg/biz/tenant"
)

// MpClientFactory 维护 appId -> *officialaccount.OfficialAccount 的线程安全实例池
type MpClientFactory struct {
	mu         sync.RWMutex
	wc         *wechat.Wechat
	redisCache cache.Cache
	clients    map[string]*officialaccount.OfficialAccount
}

func (f *MpClientFactory) GetOrCreateClient(acc *MpAccountDO) *officialaccount.OfficialAccount {
	f.mu.RLock()
	cli, ok := f.clients[acc.AppID]
	f.mu.RUnlock()
	if ok {
		return cli
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := &offConfig.Config{
		AppID:          acc.AppID,
		AppSecret:      acc.AppSecret,
		Token:          acc.Token,
		EncodingAESKey: acc.AesKey,
		Cache:          f.redisCache, // 桥接 Redis 实现多实例共享微信 AccessToken
	}
	cli = f.wc.GetOfficialAccount(cfg)
	f.clients[acc.AppID] = cli
	return cli
}

// HandleWechatCallback 处理 /admin-api/mp/open/:appId 的微信被动消息与事件回调
func (s *MpOpenService) HandleWechatCallback(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	// 1. 忽略租户隔离查询 mp_account 获取该公众号所属的 TenantID，随后切换 Context 租户身份
	acc, err := s.accountService.GetAccountByAppIDIgnoreTenant(ctx, appID)
	if err != nil || acc == nil {
		return nil
	}
	tenantCtx := tenant.WithTenantID(ctx, acc.TenantID)

	// 2. 微信回调 5 秒重试幂等去重校验（针对带 MsgID 的普通消息）
	if msg.MsgID > 0 {
		if !s.dedupRedisDAO.TryAcquire(tenantCtx, msg.MsgID, 60*time.Second) {
			return nil
		}
	}

	// 3. 责任链分发事件与消息
	switch msg.MsgType {
	case message.MsgTypeEvent:
		switch msg.Event {
		case message.EventSubscribe:
			return s.handleSubscribeEvent(tenantCtx, acc, msg)
		case message.EventUnsubscribe:
			s.handleUnsubscribeEvent(tenantCtx, acc, string(msg.FromUserName))
			return nil
		case message.EventClick:
			return s.handleMenuClickEvent(tenantCtx, acc, msg)
		}
	default:
		return s.handleNormalMessage(tenantCtx, acc, msg)
	}
	return nil
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **微信回调免租户 Header 解析检查**：微信服务器发起的 `/admin-api/mp/open/{appId}` 回调请求不会携带 `tenant-id` 请求头，必须先使用 `tenant.WithIgnoreTenant(ctx)` 根据路径中的 `appId` 查出 `mp_account.tenant_id`，再调用 `tenant.WithTenantID(ctx, acc.TenantID)` 执行后续落库。
- [x] **微信 5 秒超时与重试去重检查**：根据 `MsgId` 在 Redis 中执行 `SETNX` 去重，耗时下载转存 S3 操作放入后台 Goroutine 异步完成，确保主请求在 500ms 内返回 XML 响应。
