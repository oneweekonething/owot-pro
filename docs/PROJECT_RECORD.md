# OWOT-Pro 项目演进与架构决策记录（ADR）

## 一、项目状态跟踪

| 阶段 | 时间 | 内容 | 状态 |
| :--- | :--- | :--- | :--- |
| Phase 0 | 2026-11-04 | 分析原始 `yudao-go-docs`（DOC-00 ~ DOC-08），明确一期范围：管理后台用户体系 + 基础模块组件 | ✅ 已完成 |
| Phase 1 | 2026-11-05 | 按照《Go 项目工程目录结构设计规范 1.2》完成架构文档、修复 6 项 Review 问题，并落地最小可验证工程骨架（`Makefile`、`.golangci.yml`、`cmd/api`、架构守卫单测、干净 `go.mod`） | ✅ 已完成 |
| Phase 2 | 2026-11-05 起 | 已实现 System 字典/公告/日志，IAM 认证/组织/RBAC/多租户/OAuth2，以及 Infra 参数配置、多云文件存储、定时任务、API 日志和 Redis 监控 | 🚧 进行中 |

---

## 二、开发与 Review 进展记录

### 2026-11-06 · Infra 定时任务、API 日志与 Redis 监控

- 新增 `005_infra_operations.sql`。任务支持 Quartz 调度、暂停恢复、手动执行、失败重试与逐次日志。各节点使用 MySQL 原子认领和 Redis 租约。
- API 日志按有效租户异步记录，保存脱敏请求结构与响应结果码。错误日志支持标记已处理或忽略，并保存处理人和处理时间。Redis 面板提供运行信息、键数量与命令统计。
- 启动装配注册三条日志清理处理器，`-seed` 补齐默认任务、字典和权限。升级步骤见 [Infra 部署说明](domains/INFRA.md#定时任务api-日志与-redis-监控部署)。
- 提交前复核修复了无匹配日期的 Quartz 规则导致的栈溢出、月末和闰日计算、表单清空超时以及结束日志短暂写入失败中断任务重试。补齐停机日志写入和单次日志导出上限的回归。

### 2026-11-05：Infra 多云文件存储与头像集成
- **实现范围**：
  - 落地 `infra_file_config`、`infra_file` 与 `infra_file_content` 三张表及版本化迁移（`004_infra_file.sql`），内置 `infra_file_storage` 存储器字典（DB=1、Local=10、S3=20）。
  - 支持多云存储引擎抽象：数据库 LONGBLOB 存储、基于 Go 1.24+ `os.OpenRoot` 沙箱隔离的本地磁盘存储、以及基于 MinIO Client 的 S3 对象存储。
  - 支持后端中转上传、文件安全读取（`X-Content-Type-Options: nosniff`、`CSP: sandbox` 与非安全 MIME 强制附件下载防 XSS）、S3 预签名前端直传（两阶段 `_upload/` 暂存隔离 + 幂等登记防重放）、主配置热切换与删除保护。
  - 架构解耦：移除 IAM 本地头像存储硬编码，通过 `AvatarUploader` 契约桥接到 Infra 当前主文件存储，同时兼容旧版头像本地只读路由。
- **一致性与并发安全**：
  - 基于 MySQL VIRTUAL 生成列 `master_slot` 配合唯一索引，在数据库引擎级杜绝多主配置并发冲突。
  - 主配置切换采用行锁互斥（UPDATE 锁），上传事务采用共享锁（SHARE 锁），实现高频并发上传互不阻塞、切主原子安全。
  - 配置与切主变更通过 Transactional Outbox 发布 `infra.file-config.refresh` 事件，Redis 版本更新驱动各节点 S3 客户端热重建。
  - 上传失败时尝试清理未提交的物理对象。进程崩溃或补偿失败时，最终对象可能需要人工核对清理。
- 验证覆盖 `make check`、MySQL/Redis 并发切主与上传，以及 S3 直传、预览等待、私有链接和删除重试。

### 2026-11-05：Infra 参数配置
- 已接入全局配置 CRUD、批量删除、分页、XLSX 导出和两端可见值查询。系统租户的管理权限保护私有值，内置项受删除保护。
- 配置修改和刷新事件同事务提交，内部读取通过 Redis 版本校验本地缓存。通过 `make check` 和真实 MySQL/Redis 集成测试，覆盖并发唯一键、事务回滚与广播恢复。

### 2026-11-05：IAM 一期实现与验收
- **实现范围**：认证/双令牌轮换、个人中心与头像、用户 Excel、部门/岗位、角色/菜单、五级数据权限、租户/套餐级联初始化与权限回收、OAuth2 客户端/在线令牌；新增 13 张表的版本化迁移与可重复种子数据，手动接入 System 审计和用户昵称查询。
- **一致性**：租户行锁保护配额与组织变更，全局变更按系统租户到目标租户顺序加锁；套餐回收使用当前读，覆盖等待锁期间新提交角色。权限采用同事务 Outbox、Redis 租户/全局版本与 Ristretto 30s TTL；Relay 轮询 500ms、重试上限 1s。禁用/重置密码/登出/强退同步驱逐会话，认证同时确认 Redis 与持久化会话。
- **复核修正**：数据权限单个 OR 条件误合并、MySQL 零值登录时间与 TEXT 默认值兼容、套餐等待期间角色遗漏、超级管理员状态回收、跨租户访问时登出主体归属，以及仅本人范围通过空部门创建账号的绕过，均有回归用例。
- **验证**：`make check`（含架构守卫、race、lint、build）及真实 MySQL 8.0.46 + Redis 7 的 `make test-integration` 通过；实际二进制完成初始化、健康检查、登录、权限菜单、用户查询、登出与 SIGTERM 优雅退出。前端源码不在仓库，未做浏览器联调。运行说明见 [IAM_IMPLEMENTATION.md](development/IAM_IMPLEMENTATION.md)。

### 2026-11-05：IAM 提交前双轴复核
- **规范**：令牌列表不再暴露 bearer 凭证；委派重置受目标管理员身份约束；Redis 驱逐在 SQL 提交后同步执行；精简列表仅返回选择器字段；认证区分无效令牌与依赖故障并保留错误链。
- **规格**：初始管理入口不能因停用超级角色或默认客户端而锁死；过期 AccessToken 仍可登出并销毁 RefreshToken；用户分页返回前端所需的 `deptName`。新增 HTTP 回归覆盖这些行为。

### 2026-11-05：System 模块实现与全域代码 Review
- **审查范围**：`internal/system` 全域代码（`domain`、`app`、`adapter/{http,persistence}`）、核心平台包（`dbx`、`httpx`、`outbox`、`excelx`、`timeutil`）、数据库迁移（`migrations/mysql/001_system.sql`）及各级测试用例。
- **审查结论**：
  1. **严格分层与依赖收敛**：`domain` 达成 100% 纯标准库依赖（零外部包）；`app` 仅依赖纯标准库契约包，领域间解耦与 Port 接口定义清晰，符合 Modular Monolith 规范。
  2. **租户隔离与安全防护**：租户表（公告、日志）在 GORM 回调层实现 fail-closed 拦截，更新操作忽略租户变更；基础设施错误对外脱敏；Excel 导出以文本写入字符串，并通过公式注入回归测试。
  3. **数据一致性与并发安全**：字典写操作与 Outbox 事件在同一 DB 事务中落库；父类型行锁与当前读检查覆盖已验证的并发新增/删除场景；可空唯一列支持重复软删除后复用业务标识。初次审查未覆盖批量操作的锁序冲突，已在后续复核中发现并修复。
  4. **门禁与测试**：初次 `make check` 通过，但未覆盖全部前端交互契约；后续对照实际前端接口补充了兼容性修复与回归测试。
  5. **持续优化建议**：提出增强 HTTP 查询参数（如时间范围）在不同前端序列化格式下的兼容性等改进建议。

### 2026-11-05：复核问题修复
- **7 项修复**：CORS 预检与响应头；超长 User-Agent 导致审计丢失；批量删除与跨类型更新的父锁顺序冲突；`export-excel` 导出路径；字典类型精简列表包含禁用类型；操作人返回 `userName`；操作日志支持 `action` 与 `bizId` 筛选。
- **回归方式**：修复前先用持久化回归用例复现，修复后覆盖实际 HTTP 组装、SQLite/miniredis 及真实 MySQL 并发事务；IAM 未接入时的默认 401 行为保留。
- **验证结果**：`make check` 全部通过，lint 为 0 issues；`make test-integration` 在真实 MySQL 8.0.46 与 Redis 7 上通过，包含批量删除与数据移动的并发回归。

---

## 三、核心架构决策记录（ADR）

### ADR-001：采用单 Go Module 模块化单体，废除顶层 `pkg/` 与 `go.work`
- **决策**：遵循《规范 1.2》第 03/10 节，采用单 `go.mod`，所有代码放入 `cmd/` 与 `internal/`，跨领域技术能力放入 `internal/platform/`，依赖随真实代码按需引入。

### ADR-002：将原 `system` 模块拆分为 `internal/iam` 与 `internal/system` 两个领域
- **决策**：将高复杂度、强规则的“认证、用户、部门、岗位、角色、菜单、权限、多租户、OAuth2”独立为 **`internal/iam` 领域**；将“数据字典、通知公告、操作日志、登录日志”保留在 **`internal/system` 领域**。

### ADR-003：严格分层边界 —— `domain` 100% 纯标准库，`platform` 拆分纯契约包与框架包
- **背景**：若允许 `domain` 导入平台包或 `bcrypt`，或让 `app` 导入含 Gin/GORM/Redis 的混合平台包，会导致核心业务层被外部框架污染。
- **决策**：
  1. `internal/*/domain` **100% 仅允许导入 Go 标准库**；密码哈希通过 `app/ports.go` 的 `PasswordHasher` 接口由 `adapter/security` 实现；数值错误码映射在 `adapter/http` 完成。
  2. `internal/platform` 拆分为纯标准库包（`ctxmeta`、`pagination`）与框架适配包（`httpx`、`dbx`、`cachex` 等）；`internal/*/app` 仅允许导入纯标准库包。
  3. 通过 [`cmd/api/arch_test.go`](../cmd/api/arch_test.go) 在每次 `make test` 时用 `go/parser` 静态扫描全仓 Import 树强制守护。

### ADR-004：权限与配置缓存失效采用 Transactional Outbox + 版本号 + 30s 有界 TTL
- **背景**：单纯事务后内存/PubSub 广播在进程退出或断线时会丢失失效通知，导致已撤销权限继续生效。
- **决策**：权限/字典/主配置变更在同一 DB 事务内写入 `sys_outbox_event`，由实时广播（`< 100ms`）+ Outbox Relay 轮询重放（`≤ 2s`）+ Redis 权限版本号 + 本地缓存 `30s` 硬过期 TTL 共同保障；Token 强退直接删除 Redis 权威会话（`0ms` 立即生效）。

### ADR-005：双轨集成测试（SQLite/miniredis 快速替身 + 真实 MySQL/Redis 集成测试）
- **决策**：`make test` 使用内存替身提供秒级反馈；`make test-integration` 连接真实 MySQL 8.0 与 Redis 7 验证 SQL 方言、事务隔离、Outbox 投递与 Lua 分布式锁。

### ADR-006：用户类型枚举值采用 `Admin=1, Member=2`
- **决策**：遵循项目手册 `DOC-01` 定义：`UserTypeAdmin = 1`（访问 `/admin-api/**`），`UserTypeMember = 2`（访问 `/app-api/**`）。
