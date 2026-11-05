# System 模块实现与运行

本轮覆盖 `internal/system`：字典类型、字典数据、通知公告、操作日志、登录日志，以及这些功能实际消费的平台能力。代码按 `domain → app → adapter` 分层，在 `cmd/api/server.go` 手动组装。

## 本地启动

Go 版本以 `go.mod` 为准。SQLite 快速测试需要 CGO；正常服务连接 MySQL 与 Redis。

```bash
docker compose -f compose.integration.yml up -d --wait
cp .env.example .env
set -a
source .env
set +a
go run ./cmd/api -migrate -seed -check
go run ./cmd/api -addr :48080
```

- `-migrate` 显式初始化开发表。生产环境应按编号执行 `migrations/mysql/` 中的迁移。
- `-seed` 初始化缺失的基础字典及 IAM 默认数据，包括 IAM 枚举和 Infra 的配置类型、可见性选项。可重复运行，保留已有名称、状态和数据项修改。
- `-check` 校验连接、表存在性与依赖组装后退出；不会启动监听与 Relay。
- 不配置 MySQL/Redis 时保留原来的健康检查模式；只配置其中一项会报错。
- 配置通过环境变量读取，程序不会自动加载 `.env`。数据库时间统一存储为 UTC，前端字符串时间区间按 Asia/Shanghai 解析。
- CORS 预检允许 `Authorization`、`Content-Type`、`tenant-id`、`visit-tenant-id`、`trace-id`，暴露 `trace-id` 和下载文件名响应头；`OWOT_CORS_ORIGINS` 可配置逗号分隔的来源列表。不配置时允许任意来源，不启用 Cookie 凭证。实际请求仍执行认证与权限检查。

```bash
curl http://localhost:48080/actuator/health
curl 'http://localhost:48080/app-api/system/dict-data/type?type=common_status'
make check
make test-integration
docker compose -f compose.integration.yml down -v
```

Compose 使用独立项目 `owot-system-tests`，端口为本机 `13306`（MySQL 8）与 `16379`（Redis 7）。真实集成测试仅应指向专用测试数据库。

## 认证与领域接入

IAM 已通过 Composition Root 注入 `TokenVerifier`、`PermissionChecker`、`TenantValidator` 与 `DataPermissionProvider`，启动与初始账号见 [IAM_IMPLEMENTATION.md](IAM_IMPLEMENTATION.md)。测试中保留未注入安全端口的场景，确保管理接口返回 `401`。静态 Token 替身只存在于测试代码。

- `/app-api/system/dict-data/type` 公开访问且忽略租户，仅返回启用类型下的启用数据项。
- 管理接口校验 Token 用户类型为 Admin=1、`tenant-id` 与登录租户一致，并检查具体权限。
- `visit-tenant-id` 仅允许由可信认证实现标记的超级管理员使用。
- `app.UserNicknameReader` 已在根组装中桥接 IAM，批量回填操作人昵称及解析昵称筛选；保持租户隔离。
- `system/adapter/http.OperationRecorder` 实现 `httpx.OperateLogRecorder`；成功的字典和公告写操作在有界上下文内同步记录操作日志，不创建脱离服务生命周期的协程。
- 适配器按 Unicode 字符截断超长 `User-Agent`，避免客户端通过请求头长度使整条操作日志被拒绝。
- 登录流程通过 `AuditLogService.RecordLoginLog` 写入成功、账密错误、账号禁用及登出事件；不提供客户端可随意伪造审计日志的写接口。
- `DictionaryTranslator` 实现 `excelx.DictTranslator`，供其他适配器调用字典值/标签互译。

## HTTP 契约

所有 JSON 接口使用 HTTP 200 与 `{code,data,msg}`，空列表为 `[]`，时间为毫秒时间戳。XLSX 导出直接返回工作簿。

| 资源 | 路径前缀 | 动作 |
| --- | --- | --- |
| 字典类型 | `/admin-api/system/dict-type/` | `create`、`update`、`delete`、`delete-list`、`get`、`page`、`simple-list`、`list-all-simple`、`export` |
| 字典数据 | `/admin-api/system/dict-data/` | 同上，另含 `type` |
| 公告 | `/admin-api/system/notice/` | `create`、`update`、`delete`、`get`、`page` |
| 操作日志 | `/admin-api/system/operate-log/` | `page`、`export` |
| 登录日志 | `/admin-api/system/login-log/` | `page`、`export` |

创建使用 POST，更新使用 PUT，删除使用 DELETE，其他动作使用 GET。字典权限为 `system:dict:{create,update,delete,query,export}`；公告为 `system:notice:{create,update,delete,query}`；日志为 `system:{operate-log,login-log}:{query,export}`。精简列表与管理端按类型查询只要求已登录。

四类导出同时支持 `export` 与前端使用的 `export-excel`。字典类型精简列表包含开启与禁用类型；字典数据精简列表及公开查询仍只提供启用项。操作日志的操作人显示字段为 `userName`。

- 分页支持 `pageNo`、`pageSize`，默认 1/10，上限 200，`pageSize=-1` 返回全部。
- 批量删除同时接受 `ids=1,2` 和 `ids=1&ids=2`，最多 1000 个 ID，整批事务提交或回滚。
- 字典类型支持名称/类型模糊匹配、状态、创建时间；数据支持类型精确匹配、标签模糊匹配及状态；公告支持标题、类型与状态。
- 操作日志支持用户 ID、昵称、模块、操作名、操作内容 `action`（模糊匹配）、业务编号 `bizId`（精确匹配）与创建时间，分页和导出共用筛选条件；登录日志支持用户名、IP、登录成功状态 `status=true/false`、创建时间。
- 时间区间接受 `createTime[0]` 与 `createTime[1]`，支持毫秒或 `YYYY-MM-DD HH:mm:ss`。
- 领域错误码映射集中在 `internal/system/adapter/http/error_codes.go`；未知错误只向客户端返回 `500 系统异常`。

## 数据约束与事件

字典是全局表；公告与两类日志强制按上下文租户隔离。无租户上下文的租户表查询、写入和删除会被数据库回调拒绝；创建时拒绝冲突的租户字段，更新时租户字段不可修改。Raw SQL 仅供受信任的适配器与迁移使用，调用方须显式写租户条件。

字典类型标识创建后不可变，含数据项时禁止删除。字典类型与 `(dict_type, value)` 分别通过唯一索引保护；内部可空的 `active_type/active_value` 在软删除时清空，支持多次删除后复用同一业务标识。

字典数据写入与类型删除都先锁定父类型。跨类型更新与批量删除在修改前收集所需父类型，统一按类型标识排序、去重后加锁，避免按 ID 与按类型排序混用产生的反向锁等待。删除使用当前读检查数据项，避免 MySQL REPEATABLE READ 快照遗漏刚提交的数据项。业务变更和 `system.dict.invalidate` 事件在同一事务落库，提交后立即尝试 Redis 广播并递增 `system.dict.invalidate:version:0`。失败事件保留在 Outbox，Relay 每 500ms 扫描，失败采用封顶 1 秒的指数退避（满足 IAM 两秒恢复契约），投递语义为至少一次，消费者应按事件 ID 去重。

当前字典读取直接查库，避免本地缓存残留。后续接入本地缓存时仍须遵循架构文档的版本检查与有界 TTL 规则。

## 验证覆盖

- `internal/system/domain`：实体格式、状态、日志类型与结果枚举。
- `internal/system/app`：租户/Trace 元数据归属与错误链。
- `internal/platform/dbx`：租户读写隔离、批量创建、审计字段、软删除、事务传播与提交回调。
- `internal/platform/outbox`：原子追加、回滚、失败补发、Redis 版本与 Pub/Sub。
- `cmd/api/system_test.go`：实际 Composition Root 组装的 HTTP 闭环，覆盖权限、租户、字典生命周期、批量回滚、公告、日志查询、默认字典、XLSX 与公式注入防护。
- `cmd/api/review_regression_test.go`：CORS、超长请求头审计、导出别名、全量字典类型、操作人字段、操作日志筛选及统一父锁顺序。
- `internal/platform/httpx/cors_test.go`：来源白名单、预检方法/请求头校验和下载响应头暴露。
- `tests/integration/system_test.go`：在真实 MySQL/Redis 上执行版本化 DDL、唯一键、事务回滚、父类型行锁、租户隔离、Outbox 补发与广播验证。

2026-11-05 本地验收：`make check` 全部通过，lint 为 0 issues；`make test-integration` 在 MySQL 8.0.46 与 Redis 7.4.12 上通过。编译产物通过 `-migrate -seed -check`，实际启动后健康检查、公开字典查询、管理接口 401 与 SIGTERM 优雅退出均通过。验收使用的独立 Compose 容器及卷已清理。
