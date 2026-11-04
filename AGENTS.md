# AGENTS.md

## Project
`owot-pro` (`oneweekonething.com/owot-pro`) —— 基于 Go 模块化单体（Modular Monolith）架构的企业级后台底座，对标 `ruoyi-vue-pro` 的用户与基础模块能力，前端无缝兼容 `yudao-ui-admin-vue3`。

## Toolchain
- 使用 `go.mod` 指定的 Go 版本（单 Go Module 起步，不预建 `go.work`，依赖随真实代码按需引入）。
- 使用 `Makefile` 中定义的标准校验命令（`make fmt`、`make vet`、`make test`、`make race`、`make lint`、`make build`、`make check`）。
- 依赖在 `cmd/api` 中显式手动组装（Composition Root），不使用全局变量、`init()` 隐式副作用或代码生成式 DI（如 Wire）。

## Architecture (Mandatory Rules)
1. **业务边界优先**：所有代码位于 `internal/` 下，按业务领域划分（`internal/iam`、`internal/system`、`internal/infra`），严禁按 `controller/service/dao` 顶层技术分层。
2. **依赖向核心收敛（严格分层边界）**：
   - `domain`：**100% 仅允许依赖 Go 标准库**（严禁导入任何第三方库或任何 `internal/platform/*` 包）。密码哈希、ID 生成等外部技术能力通过 `app/ports.go` 定义消费方接口，由 `adapter` 实现；HTTP/RPC 数值错误码映射在 `adapter` 层完成。
   - `app`：仅允许依赖本领域的 `domain`、Go 标准库，以及纯标准库实现的 `internal/platform/ctxmeta` 与 `internal/platform/pagination`。**严禁 `app` 直接或间接导入 Gin、GORM、Redis、Cron、Excelize 等外部框架包**。
   - `adapter`：实现 `app/ports.go` 接口，可导入对应的 `internal/platform/{httpx,dbx,cachex,mqx,lockx,jobx,excelx}` 及底层驱动。
3. **接口由消费方定义**：小而明确的 Port 接口定义在消费方的 `app/ports.go` 中；仅在需要替换、单测隔离或跨领域解耦时定义接口。
4. **禁止万能大杂烩包**：严禁创建 `common`、`utils`、`shared`、`pkg` 等职责不清的目录；跨领域稳定复用的纯技术能力按具体职责放入 `internal/platform/<capability>`。
5. **按需创建目录**：不预建空壳目录；复杂领域采用 `domain/ + app/ + adapter/`，简单子域可直接扁平组织。

## 编码准则

- 先澄清再编码：明确需求、假设、验收标准和取舍；关键歧义或规则冲突先确认，已明确的要求直接执行。
- 优先简单方案：只实现当前需求，复用现有模式；抽象、配置和异常分支须有实际场景支撑。
- 只改必要内容：遵循已有风格，不顺带重构或调整无关注释、格式；清理本次改动产生的无用代码，既有无关问题先记录。
- 以验证结果收尾：缺陷先复现，行为变更按用例验收，重构对照前后行为；检查范围与风险相称，说明结果及未验证项。

## Validation
- 本地与 CI 统一门禁：`make check`（依次执行 `fmt` -> `vet` -> `test` -> `race` -> `lint` -> `build`）
- 真实依赖集成测试：`make test-integration`（针对真实 MySQL + Redis 验证事务、锁与 SQL 兼容性）

## Progressive Guidance (Read on Demand)
- 文档总索引与范围说明：[docs/README.md](docs/README.md)
- 总体工程架构与目录规范：[docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md)
- 内部技术平台层规范（`internal/platform`）：[docs/architecture/PLATFORM.md](docs/architecture/PLATFORM.md)
- 领域设计（一期范围）：
  - IAM 身份认证、用户、权限与多租户领域：[docs/domains/IAM.md](docs/domains/IAM.md)（局部规则：[internal/iam/AGENTS.md](internal/iam/AGENTS.md)）
  - System 系统支撑领域（字典、通知公告、审计日志）：[docs/domains/SYSTEM.md](docs/domains/SYSTEM.md)（局部规则：[internal/system/AGENTS.md](internal/system/AGENTS.md)）
  - Infra 基础设施领域（参数配置、文件存储、定时任务、API 日志、Redis 监控）：[docs/domains/INFRA.md](docs/domains/INFRA.md)（局部规则：[internal/infra/AGENTS.md](internal/infra/AGENTS.md)）
- 专项研发规范：
  - Go 编码与错误处理规范：[docs/development/GO_CONVENTIONS.md](docs/development/GO_CONVENTIONS.md)
  - HTTP API 契约规范（兼容 yudao-ui-admin-vue3）：[docs/development/API_CONVENTIONS.md](docs/development/API_CONVENTIONS.md)
  - 数据库、多租户、Outbox 与迁移规范：[docs/database/DATABASE.md](docs/database/DATABASE.md)
  - 测试分层（含真实 MySQL/Redis 集成测试）与质量验收规范：[docs/testing/TESTING.md](docs/testing/TESTING.md)
  - 项目记录与架构决策（ADR）：[docs/PROJECT_RECORD.md](docs/PROJECT_RECORD.md)
