# `internal/platform` 内部技术平台层规范

> 对应《Go 项目工程目录结构设计规范 1.2》第 02、04 节：**为了防止 `app` 用例层因导入上下文或分页类型而间接引入 Gin、GORM、Redis 等外部框架依赖，`internal/platform` 严格拆分为“纯标准库契约包”与“外部框架基础设施包”两层。**

## 一、平台子包分层清单与依赖隔离表

### 1.1 第一层：纯标准库契约包（零外部依赖，允许 `app` 与 `adapter` 导入）

| 子包路径 | 单一职责 | 外部依赖 | 允许被谁导入 |
| :--- | :--- | :--- | :--- |
| `internal/platform/ctxmeta` | 在 `context.Context` 中读写 `LoginUser`、`TenantID`、`IgnoreTenant`、`TraceID`、`DataScope` 以及异步协程 `CloneContext(ctx)` | **仅 Go 标准库** | `app`、`adapter`、`platform/*` |
| `internal/platform/pagination` | 分页请求 `PageQuery`（含 `PageSize=-1` 不分页支持）与泛型分页结果 `PageResult[T]`（保证空列表序列化为 `[]`） | **仅 Go 标准库** | `app`、`adapter`、`platform/*` |
| `internal/platform/apperr` | 协议无关的错误码结构 `ErrorCode` 与 `AppError`，供 `adapter` 层将 `domain` 错误映射为前端契约错误码 | **仅 Go 标准库** | `adapter`、`platform/*` |
| `internal/platform/timeutil` | 毫秒时间戳 `MillisTime` JSON 序列化/反序列化、时间范围解析 | **仅 Go 标准库** | `adapter`、`platform/*` |

> [!IMPORTANT]
> `internal/platform/ctxmeta` 与 `internal/platform/pagination` **严禁导入 Gin、GORM、Redis 或任何第三方库**。这样 `internal/*/app` 在读取当前登录用户、切换租户上下文或返回分页结果时，其编译依赖树依然保持 100% 纯标准库。

### 1.2 第二层：外部框架基础设施包（仅允许 `cmd` 与 `adapter` 导入，`app` 与 `domain` 严禁导入）

| 子包路径 | 单一职责 | 消费方回调接口（由业务层在 `cmd/api` 注入） |
| :--- | :--- | :--- |
| `internal/platform/config` | YAML 配置文件加载、`${ENV:default}` 环境变量展开、Profile 覆盖合并 | 无 |
| `internal/platform/observability` | Zap 结构化日志生命周期、Prometheus HTTP 指标采集与 `/metrics` 端点 | 无 |
| `internal/platform/httpx` | Gin 引擎封装、`/admin-api` 与 `/app-api` 路由组、声明式路由元数据（`PermitAll` / `TenantIgnore` / `OperateLog`）、统一 `{code,data,msg}` 响应、Recovery/CORS/AccessLog/Demo/Auth/Tenant/Permission/Idempotent/RateLimit 中间件 | `TokenVerifier`、`PermissionChecker`、`TenantValidator`、`OperateLogRecorder`、`AccessLogWriter`、`ErrorLogWriter` |
| `internal/platform/dbx` | GORM 连接池初始化（MySQL / SQLite）、`BasePO` / `TenantBasePO` 基类、`TxManager` 事务传播、**GORM 多租户自动隔离插件**、**GORM 5 级部门数据权限动态改写插件** | `DataPermissionProvider` |
| `internal/platform/cachex` | `go-redis/v9` 连接管理（支持测试替身）、泛型 JSON 缓存 `RedisStore[T]`、`Ristretto` 进程内二级缓存（含 Singleflight 防击穿） | 无 |
| `internal/platform/mqx` | 轻量消息总线 `Bus`：支持 `Publish`（Redis Stream 集群消费组）与 `Broadcast`（Redis Pub/Sub 多实例广播），提供 Memory 与 Redis 两种实现 | 无 |
| `internal/platform/outbox` | 事务性 Outbox 表（`sys_outbox_event`）同事务写入器 + 后台 Relay 轮询投递与失败指数退避重试器 | 无 |
| `internal/platform/lockx` | Redis Lua 分布式锁 `Locker`（`SET NX PX` + Token 校验释放） | 无 |
| `internal/platform/jobx` | 基于 `robfig/cron/v3` + `lockx.Locker` 的集群定时任务调度器（支持在线增删改、暂停恢复、立即触发、失败重试） | `JobLogRecorder` |
| `internal/platform/excelx` | 基于 `excelize/v2` 的结构体标签（`excel:"name:状态;dict:common_status"`）流式导入导出工具 | `DictTranslator` |
| `internal/platform/testhelper` | 测试辅助容器：支持 SQLite + miniredis 单元测试替身，以及基于环境变量/Docker 的真实 MySQL + Redis 集成测试环境初始化 | 无 |

---

## 二、HTTP 中间件标准执行管道（`internal/platform/httpx`）

所有进入 `/admin-api/**` 与 `/app-api/**` 的请求按以下固定顺序经过 9 层中间件（所有写入 Context 的元数据统一存放在纯标准库包 `ctxmeta` 中）：

```text
[1. Recovery]       捕获 panic，返回 500，异步调用 ErrorLogWriter 落库堆栈
      ↓
[2. Trace]          解析或生成 32 位 TraceID，通过 ctxmeta.WithTraceID 注入 Context 并回写响应头 trace-id
      ↓
[3. CORS]           处理跨域预检与响应头
      ↓
[4. Metrics]        按路由模板记录 Prometheus 请求计数与耗时直方图
      ↓
[5. AccessLog]      缓存请求体，请求结束后异步调用 AccessLogWriter 记录 infra_api_access_log
      ↓
[6. TenantContext]  解析请求头 tenant-id / visit-tenant-id，通过 ctxmeta.WithTenantID 写入 Context
      ↓
[7. Authentication] 解析 Authorization: Bearer <token>，调用 TokenVerifier 校验并校验端类型(Admin=1/Member=2)，通过 ctxmeta.WithLoginUser 写入 Context；未登录且非 PermitAll 返回 401
      ↓
[8. TenantSecurity] 非 TenantIgnore 路由强制要求携带合法 tenant-id，调用 TenantValidator 校验租户状态/过期时间，并比对 LoginUser.TenantID 防跨租户越权
      ↓
[9. DemoMode]       若开启演示模式，拦截非白名单的 POST/PUT/DELETE 请求返回 901
      ↓
[路由级守卫]        RequirePermission → Idempotent → RateLimit → OperateLog → 领域 HTTP Handler
```

---

## 三、核心技术机制规范

### 3.1 GORM 多租户透明隔离插件（`internal/platform/dbx`）
- **上下文读取**：从 `ctxmeta.TenantID(ctx)` 与 `ctxmeta.IsIgnoreTenant(ctx)` 读取状态。
- **生效判定**：表对应的 GORM Schema 含 `tenant_id` 列，且表名不在全局白名单 `IgnoreTables` 中，且当前 `Context` 未标记 `ctxmeta.WithIgnoreTenant(ctx)`。
- **行为**：
  - `Query / Row / Update / Delete`：自动追加 `WHERE <table>.tenant_id = ?`。
  - `Create`：若 `TenantID == 0`，自动写入当前 `Context` 中的 `TenantID`。
  - **安全兜底**：当访问租户隔离表但 `Context` 中既无 `TenantID` 也未显式声明 `WithIgnoreTenant` 时，直接阻断查询并返回错误，严防因漏传 Context 导致全表数据泄露。

### 3.2 5 级部门数据权限动态改写插件（`internal/platform/dbx`）
- 支持 5 种范围：`1-全部数据`、`2-自定义部门`、`3-本部门数据`、`4-本部门及以下`、`5-仅本人数据`。
- 对已注册映射的表（一期：`system_users` 映射 `dept_id`/`id`，`system_dept` 映射 `id`），在 GORM 查询回调中自动注入 `WHERE (dept_id IN (?) OR id = ?)`。
- 同一请求内的数据权限计算结果缓存在 `ctxmeta.LoginUser` 请求级缓存中，避免单次请求内多次 DB 查询重复计算。

### 3.3 事务传播与 Outbox 协同（`internal/platform/dbx` & `outbox`）
- `dbx.TxManager.WithinTx(ctx, func(ctx context.Context) error)` 将事务会话放入 Context。
- 业务仓储方法与 `outbox.Store.Append(ctx, event)` 均通过 `dbx.FromContext(ctx, defaultDB)` 获取同一事务连接，保证“业务状态变更”与“缓存失效/领域事件落库”100% 原子提交。
