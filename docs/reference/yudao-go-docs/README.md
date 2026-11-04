# DOC-00：ruoyi-vue-pro 基础功能模块 Review 总览与 Go 工程总体架构规范

本文档库基于开源项目 [YunaiV/ruoyi-vue-pro](https://github.com/YunaiV/ruoyi-vue-pro) 与[官方开发指南（doc.iocoder.cn）](https://doc.iocoder.cn/project-intro/)，经过全面架构与代码级 Review 后整理输出。全套文档采用**按模块独立存放的多文件目录结构**，优先完成全平台最核心的 **基础功能模块（顶层容器 + 框架底座 + 2 个必选核心模块 + 5 个通用基础支撑模块）**。

## 一、多文件目录结构与模块文档清单

```text
owot-pro/docs/reference/yudao-go-docs/
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
