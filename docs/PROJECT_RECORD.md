# OWOT-Pro 项目演进与架构决策记录（ADR）

## 一、项目状态跟踪

| 阶段 | 时间 | 内容 | 状态 |
| :--- | :--- | :--- | :--- |
| Phase 0 | 2026-11-04 | 分析原始 `yudao-go-docs`（DOC-00 ~ DOC-08），明确一期范围：管理后台用户体系 + 基础模块组件 | ✅ 已完成 |
| Phase 1 | 2026-11-05 | 按照《Go 项目工程目录结构设计规范 1.2》完成架构文档、修复 6 项 Review 问题，并落地最小可验证工程骨架（`Makefile`、`.golangci.yml`、`cmd/api`、架构守卫单测、干净 `go.mod`） | ✅ 已完成 |
| Phase 2 | 待执行 | 按文档实现 `internal/platform`、`internal/iam`、`internal/system`、`internal/infra` 与单元/集成/E2E 测试 | ⏳ 待开始 |

---

## 二、核心架构决策记录（ADR）

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
