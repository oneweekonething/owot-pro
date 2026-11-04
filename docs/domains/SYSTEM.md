# System 系统支撑领域设计规范（`internal/system`）

> **领域定位**：提供全平台通用的数据字典（`DictType` / `DictData`）、系统通知公告（`Notice`）以及管理后台操作审计日志（`OperateLog`）与登录审计日志（`LoginLog`）。

## 一、目录结构（严格分层边界）

遵循《规范 1.2》与根 `AGENTS.md`：
- `domain/` **100% 仅依赖 Go 标准库**，错误定义为标准库哨兵错误（`errors.New`），不包含数值错误码。
- `app/` 仅依赖本领域 `domain`、Go 标准库与纯标准库契约包 `internal/platform/{ctxmeta,pagination}`，不直接依赖 `httpx` 或 `excelx`。
- `adapter/http` 负责 Gin 路由处理、将 `domain` 哨兵错误映射为前端契约数值错误码（`1_002_006_xxx` 字典、`1_002_008_xxx` 公告），并提供实现 `httpx.OperateLogRecorder` 与 `excelx.DictTranslator` 的外层桥接适配器。

```text
internal/system/
├── AGENTS.md                          # System 领域局部规则
├── domain/                            # 100% 纯 Go 标准库
│   ├── dict.go                        # DictType、DictData 实体与状态校验
│   ├── notice.go                      # Notice 通知公告实体
│   ├── audit_log.go                   # OperateLog、LoginLog 实体与登录结果枚举
│   └── errors.go                      # 纯标准库哨兵错误（如 ErrDictTypeNotFound, ErrDictTypeHasData）
├── app/                               # 仅依赖标准库、system/domain、platform/{ctxmeta,pagination}
│   ├── ports.go                       # DictRepository、NoticeRepository、AuditLogRepository、OutboxPublisher、UserNicknameReader
│   ├── dict_service.go                # 字典类型与数据项管理 + GetLabel/GetValue 纯业务查询
│   ├── notice_service.go              # 通知公告 CRUD 与分页
│   └── audit_log_service.go           # RecordOperateLog / RecordLoginLog 纯业务记录与分页查询
└── adapter/
    ├── http/
    │   ├── routes.go                  # 挂载 /admin-api/system/dict-*|notice|operate-log|login-log 与 /app-api/system/dict-data/*
    │   ├── error_codes.go             # domain 哨兵错误 -> 前端数值错误码 (1_002_006_xxx / 1_002_008_xxx) 映射
    │   ├── bridges.go                 # 外层桥接器：将 AuditLogService 适配为 httpx.OperateLogRecorder，将 DictService 适配为 excelx.DictTranslator
    │   ├── dict_handler.go
    │   ├── notice_handler.go
    │   └── audit_log_handler.go
    └── persistence/
        ├── models.go                  # 5 张表的 GORM PO 定义（包内私有）
        ├── dict_repo.go
        ├── notice_repo.go
        └── audit_log_repo.go
```

---

## 二、涉及数据库表（5 张业务表）

| 序号 | 物理表名 | 租户隔离 | 对应领域实体 | 核心业务约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `system_dict_type` | **全局表** | `domain.DictType` | `type` 全局唯一且创建后不可修改；存在字典数据项时禁止删除 |
| 02 | `system_dict_data` | **全局表** | `domain.DictData` | `(dict_type, value)` 唯一；变更时在同一事务写入 `sys_outbox_event` 触发缓存失效 |
| 03 | `system_notice` | **租户级 (`tenant_id`)** | `domain.Notice` | `type`(1通知/2公告)，`status`(0开启/1关闭) |
| 04 | `system_operate_log` | **租户级 (`tenant_id`)** | `domain.OperateLog` | 对齐 yudao 新版操作日志表：`trace_id`、`user_id`、`user_type`、`type`(模块)、`sub_type`(操作名)、`biz_id`、`action`、`extra`、`request_method`、`request_url`、`user_ip`、`user_agent` |
| 05 | `system_login_log` | **租户级 (`tenant_id`)** | `domain.LoginLog` | `log_type`(100账号登录/200主动登出/202强制退出)、`result`(0成功/10账号或密码不正确/20用户被禁用) |

---

## 三、RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 / 选项 | 功能说明 |
| :--- | :--- | :--- | :--- | :--- |
| **字典类型** | `POST/PUT/DELETE/GET` | `/admin-api/system/dict-type/{create,update,delete,delete-list,get,page,list-all-simple,simple-list,export}` | `system:dict:*` | 字典类型 CRUD、全量精简列表与 Excel 导出 |
| **字典数据** | `POST/PUT/DELETE/GET` | `/admin-api/system/dict-data/{create,update,delete,delete-list,get,page,list-all-simple,simple-list,type,export}` | `system:dict:*` | 字典数据 CRUD、按类型查询与 Excel 导出 |
| **C端字典** | `GET` | `/app-api/system/dict-data/type` | `PermitAll` + `TenantIgnore` | 移动端按 `type` 查询启用状态的字典项列表 |
| **通知公告** | `POST/PUT/DELETE/GET` | `/admin-api/system/notice/{create,update,delete,get,page}` | `system:notice:*` | 通知公告 CRUD 与分页 |
| **操作日志** | `GET` | `/admin-api/system/operate-log/{page,export}` | `system:operate-log:{query,export}` | 操作审计日志分页查询（回填操作人昵称）与导出 |
| **登录日志** | `GET` | `/admin-api/system/login-log/{page,export}` | `system:login-log:{query,export}` | 登录日志分页查询与导出 |
