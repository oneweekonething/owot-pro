# Go 编码与工程研发规范

## 一、包命名与严格分层依赖铁律
1. **包名简短、单数、纯小写**：如 `iam`、`domain`、`app`、`ctxmeta`、`httpx`、`dbx`；不使用下划线或 `util`/`common`/`helper` 等空泛命名。
2. **严格分层依赖（与根 `AGENTS.md` 100% 一致）**：
   - **`domain` 层**：**100% 仅允许依赖 Go 标准库**（`context`、`time`、`errors`、`fmt`、`strings`、`slices` 等），严禁导入任何第三方库（如 `bcrypt`、`gorm`、`gin`）或任何 `internal/platform/*` 包。
   - **`app` 层**：仅允许依赖同领域的 `domain`、Go 标准库，以及同样仅依赖标准库的 `internal/platform/ctxmeta` 与 `internal/platform/pagination`。**严禁 `app` 直接或间接导入 Gin、GORM、Redis、MQ、Cron、Excelize 等框架包**。
   - **`adapter` 层**：实现 `app/ports.go` 接口，可导入对应的 `internal/platform/*` 包及外部驱动；不同 Adapter 子包（如 `adapter/http` 与 `adapter/persistence`）之间互不直接引用。
3. **接口最小化与消费方定义**：
   - 接口定义在消费方的 `ports.go` 中；仅包含消费方真正调用的 1~5 个方法。
   - 密码加密（`PasswordHasher`）、事务执行（`TxRunner`）、Outbox 投递（`OutboxPublisher`）均在 `app/ports.go` 中定义为小接口，由 `adapter` 或 `platform` 实现并在 `cmd/api` 中注入。

## 二、错误处理与三层映射规范
1. **`domain` 层使用标准库哨兵错误或纯领域错误结构**：
   - 在 `domain/errors.go` 中使用标准库定义业务规则错误：`var ErrUsernameExists = errors.New("iam: username already exists")`。
   - `domain` 不感知 HTTP 状态码或前端 10 位数值错误码。
2. **`app` 层透传领域错误并包装基础设施错误**：
   - 领域校验不通过直接返回 `domain.ErrXxx`。
   - 仓储或外部端口调用失败使用 `fmt.Errorf("find user by id %d: %w", id, err)` 保留错误链。
3. **`adapter/http` 层完成协议错误码映射**：
   - 每个领域的 `adapter/http/error_codes.go` 将 `domain.ErrXxx` 映射为前端契约要求的 `apperr.ErrorCode`（如 `domain.ErrUsernameExists -> {Code: 1_002_003_001, Msg: "用户账号已经存在"}`）。
   - 未映射的未知错误由 `httpx.WriteError(c, err)` 统一记录 Error 日志并返回 `500 系统异常`，绝不向客户端泄露底层 SQL 或堆栈。

## 三、Context 与并发安全规范
1. **所有阻塞或跨边界方法首参必为 `ctx context.Context`**：涵盖全部 `app` 用例方法与 `ports.go` 接口方法。
2. **异步 Goroutine 上下文克隆**：
   - 请求内启动后台协程（如写审计日志）必须使用纯标准库函数 `ctxmeta.CloneContext(ctx)`，保留 `TenantID`、`TraceID`、`LoginUser`，同时脱离 HTTP 请求的 Cancel 信号。
   - 后台常驻协程（如 `outbox.Relay`、日志刷写 Worker）必须由 `cmd/api` 统一管理生命周期，停机时通过 `context.Context` + `sync.WaitGroup` 优雅退出。
