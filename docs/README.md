# OWOT-Pro 工程设计文档总览（基于《Go 项目工程目录结构设计规范 1.2》）

> **品牌理念**：*One Week, One Module; One Week, One System.*
> **Go Module**：`oneweekonething.com/owot-pro`
> **架构基线**：单 Go Module 模块化单体（Modular Monolith）· 领域驱动 + 六边形（Domain / App / Adapter）渐进式分层

---

## 一、设计演进说明：为什么要按规范 1.2 重构目录？

原 `yudao-go-docs` 初稿沿用了 Java / Spring Boot 的包组织习惯（顶层 `pkg/` + `internal/modules/<mod>/controller|service|dal|convert|enums` + `internal/api/` + Wire 注入）。在 Go 工程实践中，这种结构存在以下痛点：
1. **技术分层割裂业务内聚**：修改一个“用户管理”功能需要横跨 `controller/admin/user`、`service/user`、`dal/mysql`、`dal/dataobject`、`convert`、`enums` 6 个远距目录。
2. **核心业务被 ORM 与 Web 框架污染**：Service 直接操作带 GORM 标签的 `DO` 和 Gin 的 Context，无法脱离数据库对核心业务规则做纯单元测试。
3. **集中式 `internal/api` 导致胖接口**：提供方定义巨型接口，消费方被迫依赖不需要的方法，违反 Go“接口由消费方定义（Accept interfaces, return structs）”的惯用法。
4. **`pkg/common` 易退化为大杂烩**：任何不好归类的代码都会塞进 `common`，最终导致依赖失控。

为此，本版设计严格遵循**《Go 项目工程目录结构设计规范 1.2》**，将原 `system`（29 表）+ `infra`（12 表）按**一期范围（用户 + 基础模块组件）**重新梳理为 **3 个高内聚业务领域 + 1 个内部技术平台层**。

---

## 二、一期功能范围与领域划分

一期聚焦**管理后台用户与基础模块组件**（共 **27 张核心表**，含 26 张业务表 + 1 张事务性 Outbox 表 `sys_outbox_event`），将原 `system` 拆分为职责更清晰的 **`iam`（身份权限与租户核心）** 与 **`system`（系统通用支撑）** 两个业务领域，配合 **`infra`（基础设施运维）** 领域：

| 领域 / 目录 | 领域定位 | 一期包含的业务能力（表数量） | 内部组织形态 | 延后能力（二期+） |
| :--- | :--- | :--- | :--- | :--- |
| **`internal/iam`** | **身份认证、组织架构、RBAC 权限、SaaS 多租户核心** | 认证登录、后台用户、个人中心、部门、岗位、角色、菜单、权限分配与 5 级数据权限、SaaS 租户与租户套餐、OAuth2 客户端与令牌管理（**13 张业务表**） | **完整三层**（`domain/` + `app/` + `adapter/`），`domain` 100% 仅依赖标准库 | 短信/社交快捷登录、滑块验证码、OAuth2 授权码 SSO |
| **`internal/system`** | **系统通用支撑领域** | 数据字典（类型/数据项）、通知公告、操作审计日志、登录日志（**5 张业务表**） | **轻量分层 / 扁平化**（CRUD 与查询为主） | 短信渠道/模板、邮件服务、站内信、敏感词、动态错误码、IP 地区树 |
| **`internal/infra`** | **基础设施与运维中枢领域** | 动态参数配置、文件存储（配置 + 本地/DB/S3 引擎）、定时任务（配置 + 调度 + 执行日志）、API 访问日志与错误日志、Redis 监控面板（**8 张业务表**） | **按子能力内聚**（文件引擎与定时任务采用端口+适配器） | 代码生成器、多数据源配置、数据库文档导出、FTP/SFTP 存储 |
| **`internal/platform`** | **跨领域稳定复用的内部技术能力** | **纯标准库契约包**（`ctxmeta`、`pagination`）+ **技术基础设施适配包**（`apperr`、`timeutil`、`config`、`observability`、`httpx`、`dbx`、`cachex`、`mqx`、`outbox`、`lockx`、`jobx`、`excelx`、`testhelper`） | 严格隔离“纯上下文/数据结构包”与“外部框架驱动包”，确保 `app` 层零框架污染 | WebSocket、OpenTelemetry 远端导出 |

---

## 三、文档导航与渐进式阅读体系

```text
owot-pro/
├── AGENTS.md                                  # [入口] AI Agent 与开发者全局约束与快速导航
├── Makefile                                   # [门禁] fmt / vet / test / race / lint / build / check / test-integration
├── .golangci.yml                              # [门禁] 静态代码与依赖边界检查配置
├── cmd/api/main.go                            # [骨架] Phase 01 最小可构建与可验证服务入口
└── docs/
    ├── README.md                              # [总览] 本文档：范围界定、领域划分与文档索引
    ├── PROJECT_RECORD.md                      # [记录] 项目演进记录与架构决策记录 (ADR-001 ~ ADR-006)
    ├── architecture/
    │   ├── ARCHITECTURE.md                    # [核心] 工程目录结构、严格依赖方向、Outbox 跨领域协作与启动组装
    │   └── PLATFORM.md                        # [底座] internal/platform 纯契约包与框架适配包分离规范
    ├── domains/
    │   ├── IAM.md                             # [领域] internal/iam 身份、用户、部门、RBAC、多租户与权限缓存一致性规范
    │   ├── SYSTEM.md                          # [领域] internal/system 字典、公告、操作与登录审计日志设计规范
    │   └── INFRA.md                           # [领域] internal/infra 配置、文件、定时任务、API 日志与监控设计规范
    ├── development/
    │   ├── GO_CONVENTIONS.md                  # [研发] Go 编码规范、三层错误映射、Context 与并发安全准则
    │   ├── IAM_IMPLEMENTATION.md                 # [运行] IAM 启动、认证、权限与验证说明
    │   └── API_CONVENTIONS.md                 # [契约] RESTful API 规范（100% 兼容 yudao-ui-admin-vue3 前端）
    ├── database/
    │   └── DATABASE.md                        # [数据] 27 张表清单、多租户隔离、5 级数据权限、Outbox 表与迁移规范
    ├── testing/
    │   └── TESTING.md                         # [质量] 单元/替身集成/真实 MySQL+Redis 集成/E2E 测试与验收清单
    └── reference/                             # [基线] 原始 yudao-go-docs 参考文档（只读归档）
```

### 任务与文档查阅映射表

| 当前任务 | 必读文档 |
| :--- | :--- |
| 了解整体目录、依赖规则与启动组装 | [`AGENTS.md`](../AGENTS.md) → [`architecture/ARCHITECTURE.md`](architecture/ARCHITECTURE.md) |
| 实现或修改 `internal/platform/*` 技术底座 | [`architecture/PLATFORM.md`](architecture/PLATFORM.md) + [`development/GO_CONVENTIONS.md`](development/GO_CONVENTIONS.md) |
| 开发用户、登录、角色菜单权限、部门岗位、租户套餐 | [`domains/IAM.md`](domains/IAM.md) + [`internal/iam/AGENTS.md`](../internal/iam/AGENTS.md) |
| 开发字典、通知公告、操作日志、登录日志 | [`domains/SYSTEM.md`](domains/SYSTEM.md) + [`internal/system/AGENTS.md`](../internal/system/AGENTS.md) |
| 开发参数配置、文件上传、定时任务、API 日志、Redis 监控 | [`domains/INFRA.md`](domains/INFRA.md) + [`internal/infra/AGENTS.md`](../internal/infra/AGENTS.md) |
| 对接前端路由、分页、毫秒时间戳、错误码 | [`development/API_CONVENTIONS.md`](development/API_CONVENTIONS.md) |
| 修改表结构、多租户白名单、Outbox 表、数据权限字段映射 | [`database/DATABASE.md`](database/DATABASE.md) |
| 编写单测、真实 MySQL/Redis 集成测试、E2E 验收与跑 CI | [`testing/TESTING.md`](testing/TESTING.md) |
| 运行已实现的 System 模块、初始化字典与接入认证接口 | [`development/SYSTEM_IMPLEMENTATION.md`](development/SYSTEM_IMPLEMENTATION.md) |
