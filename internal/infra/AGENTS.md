# Infra 领域局部规则（`internal/infra`）

> 继承根 `/AGENTS.md`，以下为 `internal/infra` 领域的局部增量约束：

1. **严格分层依赖**：
   - `domain/` 100% 仅导入 Go 标准库，错误定义为标准库哨兵错误（`errors.New`）。
   - `app/` 仅导入标准库、本领域 `domain` 与 `internal/platform/{ctxmeta,pagination}`，严禁导入 `httpx` 或 `jobx`。
   - 协议数值错误码映射（`error_codes.go`）及对接 `httpx.AccessLogWriter` / `httpx.ErrorLogWriter` 的桥接适配器（`bridges.go`）统一置于 `adapter/http`。
2. **文件路径安全红线**：`domain/file.go` 必须使用纯标准库函数对所有上传文件路径与名称做防目录穿越清洗（拒绝或剥离 `..`、`/`、`\`），任何 Adapter 不得绕过该校验直接使用客户端原始路径。
3. **文件存储引擎解耦与主配置一致性**：DB、本地磁盘、S3（MinIO）三种存储引擎统一实现 `app.FileStorageEngine` 端口，置于 `adapter/storage/` 下；切换 `master` 文件配置必须在单事务内完成并写入 `sys_outbox_event`，触发全节点刷新客户端实例池。
4. **内置参数保护**：`infra_config` 中 `type=1`（系统内置）的配置项禁止删除；C 端 `/app-api/infra/config/get-value-by-key` 仅允许读取 `visible=true` 的配置。
5. **定时任务执行上下文**：定时任务触发器执行时默认使用 `ctxmeta.WithIgnoreTenant(ctx)`，每次执行必须通过 `JobLogRecorder` 记录 `infra_job_log`。
6. **详细规格**：见 [`docs/domains/INFRA.md`](../../docs/domains/INFRA.md)。
