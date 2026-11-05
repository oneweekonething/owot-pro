# Infra 基础设施领域设计规范（`internal/infra`）

> **领域定位**：提供动态系统参数配置（`Config`）、多引擎文件存储服务（`FileConfig` / `File`，支持 DB / 本地磁盘 / S3 兼容存储）、分布式定时任务管理（`Job` / `JobLog`）、RESTful API 访问与异常日志审计（`ApiAccessLog` / `ApiErrorLog`）以及 Redis 运行监控面板。

## 一、目录结构（严格分层边界）

遵循《规范 1.2》与根 `AGENTS.md`：
- `domain/` **100% 仅依赖 Go 标准库**，使用标准库定义哨兵错误（`errors.New`），并提供文件路径防目录穿越纯函数校验。
- `app/` 仅依赖本领域 `domain`、Go 标准库与纯标准库契约包 `internal/platform/{ctxmeta,pagination}`，不直接导入 `httpx` 或 `jobx`。
- `adapter/http` 负责 Gin 路由处理、将 `domain` 哨兵错误映射为前端契约数值错误码（`1_001_000_000` 起），并提供实现 `httpx.AccessLogWriter` 与 `httpx.ErrorLogWriter` 的外层桥接适配器。

```text
internal/infra/
├── AGENTS.md                          # Infra 领域局部规则
├── domain/                            # 100% 纯 Go 标准库
│   ├── config.go                      # Config 参数配置实体（type: 1内置/2自定义，visible 前端可见性）
│   ├── file.go                        # FileConfig、FileObject 实体、StorageType(1-DB/10-Local/20-S3)、路径防穿越纯函数
│   ├── job.go                         # Job、JobLog 实体、状态机（NORMAL/PAUSED）与规则校验
│   ├── api_log.go                     # ApiAccessLog、ApiErrorLog 实体与处理状态枚举（0未处理/1已处理/2已忽略）
│   └── errors.go                      # 纯标准库哨兵错误（如 ErrConfigKeyExists, ErrBuiltinConfigDelete, ErrMasterFileConfigDelete）
├── app/                               # 仅依赖标准库、infra/domain、platform/{ctxmeta,pagination}
│   ├── ports.go                       # 仓储接口 + FileStorageEngine 端口 + JobSchedulerPort + OutboxPublisher + RedisInspector
│   ├── config_service.go              # 参数配置 CRUD + 内存缓存 + Outbox 事件写入
│   ├── file_service.go                # 文件配置 CRUD、切换 Master（同事务写 Outbox）、文件上传/删除/读取、预签名 URL 生成
│   ├── job_service.go                 # 定时任务 CRUD、暂停/恢复、立即触发、JobLog 纯业务记录与查询
│   ├── job_runner.go                  # 持久化认领、Redis 租约、重试与执行记录
│   ├── api_log_service.go             # RecordAccessLog / RecordErrorLog 纯业务方法、分页查询、标记已处理、过期日志清理
│   └── redis_monitor_service.go       # 解析 Redis INFO、CommandStats 与 DBSize
└── adapter/
    ├── http/
    │   ├── routes.go                  # 挂载 /admin-api/infra/* 与 /app-api/infra/*
    │   ├── error_codes.go             # domain 哨兵错误 -> 前端数值错误码 (1_001_xxx_xxx) 映射
    │   ├── bridges.go                 # 外层桥接器：将 ApiLogService 适配为 httpx.AccessLogWriter 与 httpx.ErrorLogWriter
    │   ├── config_handler.go
    │   ├── file_handler.go
    │   ├── job_handler.go
    │   ├── api_log_handler.go
    │   └── redis_handler.go
    ├── persistence/
    │   ├── models.go                  # 8 张表的 GORM PO 定义（包内私有）
    │   ├── config_repo.go
    │   ├── file_repo.go
    │   ├── job_repo.go
    │   └── api_log_repo.go
    └── storage/                       # FileStorageEngine 端口的多协议适配器
        ├── factory.go                 # 动态客户端缓存池（configId -> Client）与 Master 引用
        ├── db_storage.go              # 数据库 BLOB 存储实现（infra_file_content）
        ├── local_storage.go           # 本地磁盘存储实现（含安全路径清洗）
        └── s3_storage.go              # 基于 minio-go/v7 的 S3/MinIO/OSS/COS 存储与预签名直传实现
```

---

## 二、涉及数据库表（8 张业务表）

| 序号 | 物理表名 | 租户隔离 | 对应领域实体 | 核心业务约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `infra_config` | **全局表** | `domain.Config` | `config_key` 唯一；`type=1`(系统内置参数) 禁止删除 |
| 02 | `infra_file_config` | **全局表** | `domain.FileConfig` | `master=true` 全表唯一；设为 Master 时在同一事务中将其余配置置为 `false` 并写入 `sys_outbox_event`；Master 配置禁止删除 |
| 03 | `infra_file` | **全局表** | `domain.FileObject` | 记录 `config_id`、`name`、`path`、`url`、`type`、`size` |
| 04 | `infra_file_content` | **全局表** | 内部存储表 | `storage=1(DB)` 时存储文件二进制内容，`(config_id, path)` 唯一 |
| 05 | `infra_job` | **全局表** | `domain.Job` | `handler_name` 对应注册的 Go 任务处理器；支持在线启停与立即触发 |
| 06 | `infra_job_log` | **全局表** | `domain.JobLog` | 记录每次调度的时间、耗时、执行序号与结果 |
| 07 | `infra_api_access_log` | **租户级 (`tenant_id`)** | `domain.ApiAccessLog` | 记录请求方法、URL、参数、响应、IP、UA、耗时、TraceID |
| 08 | `infra_api_error_log` | **租户级 (`tenant_id`)** | `domain.ApiErrorLog` | 记录未捕获异常堆栈及人工处理状态（`process_status`、`process_user_id`） |

---

## 三、核心领域规则

1. **文件路径防目录穿越（`domain.SanitizeFilePath`）**：
   - 任何客户端传入的 `path` 或 `name` 必须剥离路径分隔符与 `..`，统一生成 `{yyyyMMdd}/{sha256_prefix}_{clean_name}`，杜绝 `../` 任意文件读写漏洞。
2. **文件主配置热切换（`FileService.UpdateFileConfigMaster`）**：
   - 在单事务内执行 `UPDATE infra_file_config SET master = false` + `UPDATE infra_file_config SET master = true WHERE id = ?` + 写入 `sys_outbox_event`，由实时广播与 Outbox Relay 确保所有节点刷新 `storage.Factory` 缓存。
3. **内置日志清理定时任务（`accessLogCleanJob` / `errorLogCleanJob` / `jobLogCleanJob`）**：
   - 启动时注册 Go 处理器，`-seed` 创建默认调度，定期清理超过 14 天的历史日志。

---

## 四、RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 / 选项 | 功能说明 |
| :--- | :--- | :--- | :--- | :--- |
| **参数配置** | `POST/PUT/DELETE/GET` | `/admin-api/infra/config/{create,update,delete,delete-list,get,page,export,export-excel}` | 系统租户账号 + `infra:config:*` | 全局参数配置 CRUD、批量删除、分页与 XLSX 导出 |
| **管理端配置值** | `GET` | `/admin-api/infra/config/get-value-by-key` | 已登录 | 按 Key 读取可见参数 |
| **C端配置** | `GET` | `/app-api/infra/config/get-value-by-key` | `PermitAll` + `TenantIgnore` | 移动端读取公开参数（仅允许 `visible=true`） |
| **文件配置** | `POST/PUT/DELETE/GET` | `/admin-api/infra/file-config/{create,update,update-master,delete,delete-list,get,page,test}` | `infra:file-config:*` | 文件配置 CRUD、设为主配置、测试上传连通性 |
| **文件服务** | `POST / GET / DELETE` | `/admin-api/infra/file/{upload,presigned-url,create,delete,delete-list,page}` | 已登录 / `infra:file:*` | 后端上传文件、获取 S3 预签名 URL、前端直传完成登记、删除文件、文件分页 |
| **文件读取** | `GET / HEAD` | `/admin-api/infra/file/:configId/get/*path` | `PermitAll` + `TenantIgnore` | 按配置编号读取已登记文件 |
| **定时任务** | `POST/PUT/DELETE/GET` | `/admin-api/infra/job/{create,update,update-status,trigger,delete,delete-list,get,page,export-excel,get_next_times,sync}` | `infra:job:*` | 定时任务 CRUD、暂停/恢复、立即执行一次、分页、导出、预览执行时间与校验调度 |
| **任务日志** | `GET` | `/admin-api/infra/job-log/{get,page,export-excel}` | `infra:job:query` / `infra:job:export` | 定时任务执行日志详情、分页与导出 |
| **访问日志** | `GET` | `/admin-api/infra/api-access-log/{get,page,export-excel}` | `infra:api-access-log:*` | API 访问日志分页与导出 |
| **错误日志** | `PUT / GET` | `/admin-api/infra/api-error-log/{update-status,get,page,export-excel}` | `infra:api-error-log:*` | API 错误日志分页、导出与标记已处理/已忽略 |
| **Redis 监控** | `GET` | `/admin-api/infra/redis/get-monitor-info` | `infra:redis:get-monitor-info` | 返回 `{info, dbSize, commandStats}` |

参数配置表由 `migrations/mysql/003_infra_config.sql` 创建。升级已有部署时在启动前执行该迁移，并运行 `go run ./cmd/api -seed -check` 补齐配置类型和可见性字典。本地开发可用 `go run ./cmd/api -migrate -seed -check` 初始化。

管理接口创建自定义项，更新时保留原类型。数据库根据配置键和软删除状态生成唯一键，SQL 初始化内置项时填写业务字段即可。内置项禁止删除，批量删除遇到内置项时整批回滚。服务内调用方可通过 `ConfigService.Value(ctx, key)` 读取参数，包括不可见项。修改与 `infra.config.refresh` 事件同事务写入，每次投递生成新的 Redis 缓存版本，内部读取按版本校验 30 秒本地缓存，Redis 不可用时回源 SQL。两端的按键接口直接从数据库同时读取值和可见性。C 端对缺失和隐藏的键都返回 `data: null`。

### 文件存储部署

升级时执行 [004_infra_file.sql](../../migrations/mysql/004_infra_file.sql)，再运行 `go run ./cmd/api -seed -check` 添加存储器字典。迁移会初始化编号为 1 的 DB 主配置。系统超级管理员可测试存储、修改配置和切换主配置。文件列表及删除按当前租户隔离。主配置和仍有文件或有效直传预约的配置受删除保护。主配置切换与 Outbox 事件同事务提交，文件继续由写入时的配置提供读取。

- 本地存储的 `basePath` 位于 `OWOT_FILE_DIR`（默认 `data/infra-files`）内，配置之间使用独立子目录。多个服务实例使用本地存储时，共享同一持久卷。
- 后端上传和直传登记均限制为 20 MiB。S3 直传需要浏览器能访问 `endpoint`，并在 bucket 配置允许前端源站发起 PUT 的 CORS 规则。预签名 PUT 有效期为 10 分钟，写入 `_upload/` 前缀。按上传者和实际内容登记后，服务写入最终对象。预览请求可等待登记，最长 15 秒。20 MiB 限额在登记时校验，浏览器发起的 PUT 由 bucket 接收。为 `_upload/` 配置生命周期过期规则，以清理未登记及重复 PUT 的临时对象。
- 同源部署可由前端代理 `/admin-api/infra/file/`。前后端分离时，将 `OWOT_FILE_PUBLIC_URL` 设为浏览器能访问的后端地址，DB 和本地配置的 `domain` 可覆盖此值。私有 S3 文件通过 24 小时签名链接读取，重新打开文件列表可获取新链接。S3 公开配置填写 `domain` 时可返回对象直链，bucket 的公开读取策略仅覆盖最终对象，排除 `_upload/`。头像使用稳定的 API 地址。
- S3 配置详情以占位符显示密钥，编辑保留占位符或留空时沿用现有密钥。测试接口返回一条可下载的样例文件，样例可从文件列表删除。存储删除失败时，文件仍在管理列表中供重试，API 下载在此期间停用。

### 定时任务、API 日志与 Redis 监控部署

升级时执行 [005_infra_operations.sql](../../migrations/mysql/005_infra_operations.sql)，再运行 `go run ./cmd/api -seed -check` 补齐运维字典、权限按钮和三条默认日志清理任务。开发环境也可运行 `go run ./cmd/api -migrate -seed -check`。全局定时任务和 Redis 监控由系统租户授权访问，API 日志按请求访问的有效租户隔离。

- Cron 使用 Asia/Shanghai 时区和 Quartz 语法。每个 Go 处理器只能绑定一条有效任务，新增处理器需在启动装配中注册。默认日志清理任务于每日凌晨 02:00、02:10、02:20 执行。
- 所有实例约每秒读取待执行任务，使用 Redis 租约防止同一处理器并发运行。MySQL 在同一事务内认领任务并写入首条运行日志。停机期间错过的多次调度合并执行一次。暂停阻止后续自动调度，仍可手动触发。重复请求在待执行期间合并。已经认领的执行继续运行，任务处理器须响应上下文取消。硬退出可能留下运行中的日志，已认领的次数由日志和任务状态供排查。
- 重试间隔和执行超时均以毫秒为单位。超时为 0 时使用五分钟。每次尝试单独记录结果。同步接口校验已持久化的任务和处理器，各节点直接读取数据库中的调度状态。
- API 日志在请求结束后异步写入。已确认租户的请求可记录访问日志，内部错误和 panic 额外记录脱敏堆栈。请求参数保留字段名，响应仅保留结果码，避免写入凭证、上传内容和业务响应。公开无租户接口和未验证租户的鉴权请求跳过 API 日志，账号登录仍由登录审计记录。日志队列满时丢弃新记录并写入告警。任务日志和 API 日志单次最多导出 5000 条，可按时间缩小查询范围。
- Redis 监控使用应用连接读取 `INFO`、`INFO commandstats` 与 `DBSIZE`。连接账户需要这三项命令的读取权限。
