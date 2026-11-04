# System 领域局部规则（`internal/system`）

> 继承根 `/AGENTS.md`，以下为 `internal/system` 领域的局部增量约束：

1. **严格分层依赖**：
   - `domain/` 100% 仅导入 Go 标准库，错误定义为标准库哨兵错误（`errors.New`）。
   - `app/` 仅导入标准库、本领域 `domain` 与 `internal/platform/{ctxmeta,pagination}`，严禁导入 `httpx` 或 `excelx`。
   - 协议数值错误码映射（`error_codes.go`）及对接 `httpx.OperateLogRecorder` / `excelx.DictTranslator` 的桥接适配器（`bridges.go`）统一置于 `adapter/http`。
2. **全局表与租户表区分**：
   - `system_dict_type`、`system_dict_data` 为**系统全局表**（无 `tenant_id`）；`DictType.Type` 创建后不可修改，包含字典项时禁止删除字典类型；字典变更同事务写入 `sys_outbox_event` 触发缓存失效。
   - `system_notice`、`system_operate_log`、`system_login_log` 为**租户隔离表**。
3. **详细规格**：见 [`docs/domains/SYSTEM.md`](../../docs/domains/SYSTEM.md)。
