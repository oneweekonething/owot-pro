# IAM 领域设计规范（`internal/iam`）

> **领域定位**：全平台的身份认证（Authentication）、后台用户与组织架构（User / Dept / Post）、RBAC 菜单与 5 级数据权限（Role / Menu / Permission）、SaaS 多租户与套餐（Tenant / TenantPackage）以及 OAuth2 令牌管理核心领域。

## 一、分层职责与零外部依赖边界

1. **`internal/iam/domain`（100% 纯 Go 标准库）**：
   - 包含 `AdminUser`、`Dept`、`Post`、`Role`、`Menu`、`Tenant`、`TenantPackage`、`OAuth2Client`、`AccessToken` 等实体与纯函数算法（部门树防环、子孙部门 ID 展开、角色菜单差集计算、租户套餐变更菜单裁剪、5 级数据权限并集计算）。
   - **不导入任何第三方库或 `internal/platform/*` 包**：密码哈希计算不放在 `domain`，`domain` 仅校验明文密码长度/格式规则；领域错误定义为标准库哨兵错误（如 `var ErrUserNotFound = errors.New("iam: user not found")`）。
2. **`internal/iam/app`（用例编排 + 消费方端口）**：
   - 仅依赖 `iam/domain`、Go 标准库、`internal/platform/ctxmeta` 与 `internal/platform/pagination`。
   - 在 `ports.go` 中定义消费方接口：`UserRepository`、`DeptRepository`、`RoleRepository`、`MenuRepository`、`TenantRepository`、`OAuth2Repository`、`PasswordHasher`、`TokenCache`、`PermissionCache`、`OutboxPublisher`、`TxRunner`、`LoginLogRecorder`、`AvatarUploader`、`DictTranslator`。
3. **`internal/iam/adapter`（技术实现适配层）**：
   - `adapter/http`：Gin 路由与 Handler，负责参数绑定并将 `domain` 错误映射为前端契约错误码（`1_002_xxx_xxx`）。
   - `adapter/persistence`：GORM 仓储实现 + 同事务写入 `sys_outbox_event`。
   - `adapter/cache`：Redis AccessToken 缓存 + 租户权限版本号校验 + Ristretto 本地二级缓存。
   - `adapter/security`：基于 `golang.org/x/crypto/bcrypt` 实现 `app.PasswordHasher`（兼容 Spring `$2a$10$` 格式）。

---

## 二、目录结构

```text
internal/iam/
├── AGENTS.md                          # IAM 领域局部研发约束
├── domain/                            # 100% 纯 Go 标准库领域层
│   ├── user.go                        # AdminUser 聚合根、明文密码规则、状态流转
│   ├── dept.go                        # Dept 实体、树形防环校验、子孙部门 ID 展开纯函数
│   ├── post.go                        # Post 岗位实体与校验
│   ├── role.go                        # Role 实体、内置角色保护（super_admin / tenant_admin）、DataScope 枚举(1~5)
│   ├── menu.go                        # Menu 实体、类型枚举（1目录/2菜单/3按钮）、父子层级校验
│   ├── permission.go                  # 5 级数据权限合并计算、套餐变更角色菜单裁剪算法
│   ├── tenant.go                      # Tenant 与 TenantPackage 实体、过期与配额校验
│   ├── token.go                       # OAuth2Client、AccessToken、RefreshToken 实体与有效期规则
│   ├── errors.go                      # 标准库哨兵错误定义（errors.New）
│   └── permission_test.go             # 纯领域规则单元测试
├── app/                               # 用例编排层 + 消费方端口定义（零框架依赖）
│   ├── ports.go                       # 仓储接口、PasswordHasher、OutboxPublisher 及跨领域最小接口
│   ├── auth_service.go                # 账密登录、登出、RefreshToken 续期、get-permission-info 权限菜单树组装
│   ├── user_service.go                # 用户 CRUD（含租户配额校验）、重置密码、状态修改、个人中心、导入导出
│   ├── dept_post_service.go           # 部门 CRUD、岗位 CRUD
│   ├── permission_service.go          # 菜单/角色 CRUD、赋权、权限校验与权限版本递增
│   ├── tenant_service.go              # 租户 CRUD（创建级联初始化）、租户套餐 CRUD（更新级联回收权限）
│   └── oauth2_service.go              # OAuth2Client CRUD、Token 签发/校验/刷新/强退
└── adapter/                           # 外部适配器层
    ├── http/                          # Gin 路由与 Handler + 错误码映射表（error_codes.go）
    ├── persistence/                   # 13 张业务表的 GORM 仓储实现 + Outbox 事件写入
    ├── cache/                         # Redis Token 存储 + 权限版本校验 + Ristretto 本地缓存
    └── security/                      # bcrypt PasswordHasher 实现
```

---

## 三、涉及数据库表（13 张业务表）

| 序号 | 物理表名 | 租户隔离 | 对应领域实体 | 核心业务约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `system_users` | **租户级 (`tenant_id`)** | `domain.AdminUser` | 同一租户内 `username`、`mobile`、`email` 唯一；`post_ids` 为 JSON 数组 |
| 02 | `system_dept` | **租户级 (`tenant_id`)** | `domain.Dept` | 树形结构 `parent_id`；禁止将部门挂载到自身或其子孙部门下 |
| 03 | `system_post` | **租户级 (`tenant_id`)** | `domain.Post` | 同一租户内岗位 `code` 与 `name` 唯一 |
| 04 | `system_user_post` | **租户级 (`tenant_id`)** | 关联表 | 用户与岗位多对多关联 |
| 05 | `system_role` | **租户级 (`tenant_id`)** | `domain.Role` | `type=1`(内置角色 `super_admin` / `tenant_admin`) 禁止删除或修改标识；含 `data_scope`(1~5) 与 `data_scope_dept_ids` |
| 06 | `system_menu` | **全局表** | `domain.Menu` | 1-目录 2-菜单 3-按钮；父菜单只能是目录或菜单；存在子菜单时禁止删除 |
| 07 | `system_user_role` | **租户级 (`tenant_id`)** | 关联表 | 用户与角色多对多关联；删除用户或角色时级联清理 |
| 08 | `system_role_menu` | **租户级 (`tenant_id`)** | 关联表 | 角色与菜单多对多关联；按差集增删 |
| 09 | `system_tenant` | **全局表** | `domain.Tenant` | `package_id=0` 表示系统租户；含 `account_count` 账号配额、`expire_time`、`websites` 域名数组 |
| 10 | `system_tenant_package` | **全局表** | `domain.TenantPackage` | `menu_ids` JSON 数组；被租户引用中的套餐禁止删除 |
| 11 | `system_oauth2_client` | **全局表** | `domain.OAuth2Client` | `client_id` 唯一；内置 `default` 客户端禁止删除 |
| 12 | `system_oauth2_access_token` | **租户级 (`tenant_id`)** | `domain.AccessToken` | 同步写入 DB 与 Redis；支持在线令牌分页与强退 |
| 13 | `system_oauth2_refresh_token` | **租户级 (`tenant_id`)** | `domain.RefreshToken` | 刷新 AccessToken 时校验有效期，登出时级联移除 |

---

## 四、核心领域规则与权限缓存故障恢复

### 4.1 创建租户级联初始化（`TenantService.CreateTenant`）
在单个数据库事务中以新建租户身份（`ctxmeta.WithTenantID(ctx, newTenantID)`）依次执行：
1. 校验租户套餐存在且启用，写入 `system_tenant` 获取 `newTenantID`。
2. 创建默认根部门（名称 = 租户名称，`parent_id = 0`）。
3. 创建租户管理员角色（`code = "tenant_admin"`，`name = "租户管理员"`，`type = 1` 内置），并将租户套餐的 `menu_ids` 全部授予该角色。
4. 调用 `PasswordHasher.Hash` 加密初始密码，创建租户管理员用户（绑定默认根部门），将该用户赋予 `tenant_admin` 角色。
5. 回填 `system_tenant.contact_user_id`，并在同事务写入 `sys_outbox_event`。

### 4.2 租户套餐变更权限同步（`TenantService.UpdateTenantRoleMenu`）
当修改租户的 `package_id` 或修改套餐的 `menu_ids` 时：
- 对该租户下的 `tenant_admin` 角色：菜单权限全量同步为新套餐的 `menu_ids`。
- 对该租户下的自定义角色：计入交集，**自动回收不在新套餐 `menu_ids` 范围内的菜单权限**。

### 4.3 权限与部门树缓存失效及失败恢复保证（SLA 契约）
当发生角色删除/禁用、角色菜单变更、数据权限变更、用户角色变更、用户禁用或租户套餐缩减时：
1. **同事务 Outbox 落库**：在业务变更的同一 DB 事务中调用 `OutboxPublisher.PublishInTx(ctx, PermCacheInvalidateEvent{TenantID, Scope})` 写入 `sys_outbox_event`。
2. **双通道失效（立即广播 + Relay 重放）**：
   - 事务提交后立即递增 Redis `iam:perm_ver:{tenantId}` 并发送 Pub/Sub 广播，正常情况下集群所有实例在 **`< 100ms`** 内清空对应本地 Ristretto 缓存。
   - 若事务提交后进程崩溃或网络闪断导致广播失败，后台 `outbox.Relay` 轮询器在 **`≤ 2s`** 内重放未确认的 Outbox 事件并完成版本号递增与广播。
3. **有界 TTL 兜底（`≤ 30s`）**：
   - 本地 Ristretto 权限与部门树缓存强制设置 **最大 30 秒 TTL**；即使某实例与 Redis Pub/Sub 短暂失联，旧权限缓存最长不超过 **30 秒** 必然失效回源。
4. **Token 强退与用户禁用立即生效（`0ms`）**：
   - 禁用用户或后台强退 Token 时，同步删除 Redis 中的 `oauth2_access_token:%s` 记录；由于每次请求认证必查 Redis Token（不缓存在进程内 Ristretto），账号禁用与强退在全集群 **`0ms` 立即生效**。

---

## 五、RESTful API 路由契约表（兼容 `yudao-ui-admin-vue3`）

| 子域 | HTTP 方法 | 路由路径 | 权限标识 / 选项 | 功能说明 |
| :--- | :--- | :--- | :--- | :--- |
| **认证** | `POST` | `/admin-api/system/auth/login` | `PermitAll` | 账密登录，返回 `{userId, accessToken, refreshToken, expiresTime}` |
| **认证** | `POST` | `/admin-api/system/auth/logout` | `PermitAll` | 登出并销毁 AccessToken / RefreshToken |
| **认证** | `POST` | `/admin-api/system/auth/refresh-token` | `PermitAll` | Query `refreshToken` 换取新令牌 |
| **认证** | `GET` | `/admin-api/system/auth/get-permission-info` | 已登录 | 获取当前用户的 `{user, roles, permissions, menus}` |
| **认证** | `GET` | `/admin-api/system/auth/list-menus` | 已登录 | 获取当前用户的树形路由菜单 |
| **个人中心** | `GET / PUT` | `/admin-api/system/user/profile/{get,update,update-password}` | 已登录 | 查询/修改个人资料、修改个人密码 |
| **个人中心** | `POST / PUT` | `/admin-api/system/user/profile/update-avatar` | 已登录 | 上传并更新个人头像 |
| **用户** | `POST/PUT/DELETE` | `/admin-api/system/user/{create,update,delete,delete-list}` | `system:user:{create,update,delete}` | 用户增删改（含批量删除） |
| **用户** | `PUT` | `/admin-api/system/user/{update-password,update-status}` | `system:user:{update-password,update}` | 重置密码、启用/禁用账号（禁用时立即驱逐全部在线 Token） |
| **用户** | `GET` | `/admin-api/system/user/{get,page,list-all-simple,simple-list}` | `system:user:query` / 已登录 | 用户详情、分页（含子孙部门展开）、精简列表 |
| **用户** | `GET / POST` | `/admin-api/system/user/{export,get-import-template,import}` | `system:user:{export,import}` | Excel 导出、下载导入模板、Excel 批量导入 |
| **部门** | `POST/PUT/DELETE/GET` | `/admin-api/system/dept/{create,update,delete,delete-list,get,list,list-all-simple,simple-list}` | `system:dept:*` | 部门树 CRUD 与精简列表 |
| **岗位** | `POST/PUT/DELETE/GET` | `/admin-api/system/post/{create,update,delete,delete-list,get,page,list-all-simple,simple-list,export}` | `system:post:*` | 岗位 CRUD、分页与导出 |
| **角色** | `POST/PUT/DELETE/GET` | `/admin-api/system/role/{create,update,update-status,delete,delete-list,get,page,list-all-simple,simple-list,export}` | `system:role:*` | 角色 CRUD、状态切换、分页与导出 |
| **菜单** | `POST/PUT/DELETE/GET` | `/admin-api/system/menu/{create,update,delete,get,list,list-all-simple,simple-list}` | `system:menu:*` | 菜单 CRUD 与当前租户可用菜单精简树 |
| **权限分配** | `GET / POST` | `/admin-api/system/permission/{list-role-menus,assign-role-menu,assign-role-data-scope,list-user-roles,assign-user-role}` | `system:permission:*` | 查询/分配角色菜单、分配数据权限、查询/分配用户角色 |
| **租户** | `GET` | `/admin-api/system/tenant/{get-id-by-name,get-by-website,simple-list}` | `PermitAll` + `TenantIgnore` | 登录页根据租户名/域名解析租户、获取开启租户列表 |
| **租户** | `POST/PUT/DELETE/GET` | `/admin-api/system/tenant/{create,update,delete,delete-list,get,page,export-excel}` | `system:tenant:*` | 租户 CRUD、分页与导出 |
| **租户套餐** | `POST/PUT/DELETE/GET` | `/admin-api/system/tenant-package/{create,update,delete,delete-list,get,page,get-simple-list,simple-list}` | `system:tenant-package:*` | 租户套餐 CRUD 与精简列表 |
| **OAuth2** | `POST/PUT/DELETE/GET` | `/admin-api/system/oauth2-client/{create,update,delete,get,page}` | `system:oauth2-client:*` | OAuth2 客户端管理 |
| **在线令牌** | `GET / DELETE` | `/admin-api/system/oauth2-token/{page,delete}` | `system:oauth2-token:{page,delete}` | 在线 AccessToken 分页监控与强退 |
