# IAM 实现与运行

一期 IAM 已实现账密认证、个人中心、用户/部门/岗位、角色/菜单、权限分配与五级数据权限、租户/套餐、OAuth2 客户端和在线令牌管理。13 张 IAM 业务表的迁移位于 `migrations/mysql/002_iam.sql`。完整服务启动前需按编号应用 `migrations/mysql/` 中的迁移，开发环境可使用 `-migrate`。

## 启动

```bash
docker compose -f compose.integration.yml up -d --wait
cp .env.example .env
set -a
source .env
set +a
GIN_MODE=release go run ./cmd/api -migrate -seed -check
GIN_MODE=release go run ./cmd/api -addr :48080
```

`-seed` 创建系统租户 `id=1 / 芋道源码`、初始用户 `id=1 / admin / admin123`、内置 `super_admin` 角色、`default` OAuth2 客户端、系统/基础设施菜单按钮树及基础字典。已有密码和业务配置保留；首次登录后可通过个人中心修改密码。`-check` 检查依赖和表存在性后退出。未配置 MySQL/Redis 时仅提供健康检查。

```bash
curl -X POST http://localhost:48080/admin-api/system/auth/login \
  -H 'Content-Type: application/json' -H 'tenant-id: 1' \
  -d '{"username":"admin","password":"admin123"}'
```

后续请求携带 `Authorization: Bearer <accessToken>` 和 `tenant-id: 1`；也支持 Query `token`。系统超级管理员可通过 `visit-tenant-id` 访问其他租户。全局菜单写入、租户、套餐与 OAuth2 客户端管理额外要求系统超级管理员。公开租户解析不要求租户头。

## 契约与行为

- 路由以 `docs/domains/IAM.md` 为准。精简列表提供旧名称与 `simple-list` 别名，导出同时支持 `export` / `export-excel`。JSON 使用 HTTP 200、`{code,data,msg}`、毫秒时间与非空数组；数值错误码集中在 `internal/iam/adapter/http/error_codes.go`。
- 登录使用 bcrypt cost=10，兼容 Spring `$2a$10$`。默认 AccessToken 有效期 30 分钟、RefreshToken 30 天，客户端配置可以调整。密码比对在租户写锁之外执行，签发前在锁内复核账号状态和密码哈希。刷新会轮换并销毁旧的双令牌，旧 RefreshToken 不可重放。每次认证读取 Redis、确认数据库会话、检查用户/租户/客户端并重新确认超级管理员身份。
- 用户创建按租户行锁串行检查配额；更新/删除同时受租户隔离和数据权限约束。五种范围为全部、自定义部门、本部门、本部门及以下、仅本人，多角色取并集，没有角色时不授予业务数据。仅本人范围不能通过不填部门创建范围外账号，用户的部门变更也必须落在授权范围内。个人中心只操作令牌所属用户。委派重置不能接管超级管理员；内置超级角色和默认客户端不可停用，以保留初始管理入口。
- 创建租户与部门、内置管理员角色、管理员账号和关联关系在同一事务提交。套餐变更后，`tenant_admin` 同步全部套餐菜单，自定义角色保留交集。全局操作先锁系统租户，再按租户 ID 升序锁目标；级联读取使用当前读，避免 MySQL REPEATABLE READ 快照遗漏等待期间提交的角色。
- 用户名、非空手机号/邮箱、岗位名/编码、角色编码、租户名、套餐名与客户端标识由唯一索引保护。软删除清空可空的有效标识列，支持重复删除后复用。部门与菜单防环，引用中的部门、岗位、菜单和套餐受删除保护。
- 权限、用户角色、部门和套餐变更同事务写 Outbox。提交后立即尝试广播，Redis `iam:perm_ver:<tenantId>` 与全局版本 `iam:perm_ver:0` 参与每次缓存校验；Ristretto 本地缓存 TTL 上限 30 秒，Pub/Sub 加速失效。Relay 每 500ms 扫描，重试退避封顶 1 秒，广播恢复后在两秒内重放。Redis 不可用时拒绝使用旧缓存授权。
- 禁用用户、重置密码、登出和强退先在事务中销毁数据库双令牌，提交成功后在响应前同步清理 Redis；回滚保留原会话。Redis 清理暂时失败时，数据库校验仍立即拒绝已撤销的会话。登出可通过所属租户的过期 AccessToken 撤销配对 RefreshToken。在线令牌列表的 `accessToken` 字段返回不可认证的 `session:<id>` 标识，前端可沿用该字段强退。登录、登出、强退及管理操作接入 System 审计；操作人昵称通过 Composition Root 中的桥接查询。
- 用户 Excel 使用“用户账号、用户昵称、部门编号、用户邮箱、手机号码、用户性别、帐号状态、初始密码”列；可先下载 `user/get-import-template`。性别和状态通过 System 字典互译。导入最多 1000 行，按行报告创建、更新和失败；`updateSupport=true` 允许更新现有账号。新账号未填写初始密码时使用 `admin123`，更新不会覆盖已有密码。导出不包含密码。
- 头像支持 PNG/JPEG/GIF、最大 2 MiB。新头像通过 `AvatarUploader` 端口写入 Infra 当前主存储。既有头像继续经 `/admin-api/system/user/profile/avatar/:name` 从 `OWOT_AVATAR_DIR`（默认 `data/iam-avatars`）读取，升级时保留原持久卷。

滑块验证码、短信/社交登录和 OAuth2 授权码 SSO 属于后续范围。Infra 菜单已初始化，对应业务实现仍由 Infra 阶段提供。

## 验证

```bash
make check
make test-integration
```

- 纯领域测试覆盖密码规则、部门/菜单防环、子孙展开、五级权限并集、菜单差集和套餐裁剪。
- `cmd/api/iam_test.go` 使用实际 HTTP 组装、SQLite 与 miniredis，覆盖认证、轮换/重放、禁用/强退、组织 CRUD、五种数据范围、跨租户隔离、租户级联/配额/套餐回收、Excel、头像、客户端保护及审计桥接。
- `tests/integration/iam_test.go` 在独立 MySQL schema 执行版本化 DDL，验证真实唯一键、重复软删除、并发配额、事务与 Outbox 原子性、两秒恢复，以及套餐等待锁期间新增角色的回收。测试账号需要创建和删除测试数据库权限。

前端源码不在本仓库，验证覆盖后端 HTTP 契约与业务闭环，未执行真实前端浏览器联调。
