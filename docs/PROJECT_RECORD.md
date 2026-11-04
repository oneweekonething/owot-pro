# OWOT-Pro 项目演进与架构决策记录（ADR）

## 一、项目状态跟踪

| 阶段 | 时间 | 内容 | 状态 |
| :--- | :--- | :--- | :--- |
| Phase 0 | 2026-11-04 | 分析原始 `yudao-go-docs`（DOC-00 ~ DOC-08），明确一期范围：管理后台用户体系 + 基础模块组件 | ✅ 已完成 |
| Phase 1 | 2026-11-05 | 按照《Go 项目工程目录结构设计规范 1.2》完成架构文档、修复 6 项 Review 问题，并落地最小可验证工程骨架（`Makefile`、`.golangci.yml`、`cmd/api`、架构守卫单测、干净 `go.mod`） | ✅ 已完成 |
| Phase 2 | 2026-11-05 起 | 已实现 System 字典、公告、操作/登录日志及其平台依赖、MySQL 迁移、HTTP/集成测试，修复复核发现的 7 项问题；IAM 与 Infra 待实现 | 🚧 进行中 |

---

## 二、开发与 Review 进展记录

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
