# 数据库、多租户、Outbox 与迁移规范

## 一、通用字段与软删除约定

所有业务表统一包含以下审计与软删除列（由 `internal/platform/dbx` 基类 `BasePO` / `TenantBasePO` 提供）：

| 列名 | 类型 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `id` | `bigint` | 自增主键 | 业务主键 |
| `creator` | `varchar(64)` | `''` | 创建者用户 ID 字符串（GORM Create 回调从 `ctxmeta` 自动注入） |
| `create_time` | `datetime` | `CURRENT_TIMESTAMP` | 创建时间 |
| `updater` | `varchar(64)` | `''` | 更新者用户 ID 字符串（GORM Update 回调从 `ctxmeta` 自动注入） |
| `update_time` | `datetime` | `CURRENT_TIMESTAMP` | 更新时间 |
| `deleted` | `tinyint(1)` | `0` | 软删除标志：`0` 未删除，`1` 已删除（`gorm.io/plugin/soft_delete`） |
| `tenant_id` | `bigint` | `0` | 仅租户隔离表包含，建立索引 |

---

## 二、一期 27 张核心表全景与多租户分类

### 2.1 全局表（13 张，列入 `dbx.IgnoreTenantTables` 白名单）
1. **IAM 领域（4 张）**：`system_menu`、`system_tenant`、`system_tenant_package`、`system_oauth2_client`
2. **System 领域（2 张）**：`system_dict_type`、`system_dict_data`
3. **Infra 领域（6 张）**：`infra_config`、`infra_file_config`、`infra_file`、`infra_file_content`、`infra_job`、`infra_job_log`
4. **Platform 事务性事件表（1 张）**：`sys_outbox_event`

### 2.2 租户隔离表（14 张，自动注入 `WHERE tenant_id = ?`）
1. **IAM 领域（9 张）**：`system_users`、`system_dept`、`system_post`、`system_user_post`、`system_role`、`system_user_role`、`system_role_menu`、`system_oauth2_access_token`、`system_oauth2_refresh_token`
2. **System 领域（3 张）**：`system_notice`、`system_operate_log`、`system_login_log`
3. **Infra 领域（2 张）**：`infra_api_access_log`、`infra_api_error_log`

---

## 三、事务性 Outbox 表结构（`sys_outbox_event`）

用于保证权限变更、套餐回收、字典变更、文件主配置切换等跨实例缓存失效与领域事件在进程崩溃或网络抖动时 **100% 最终投递成功**：

| 列名 | 类型 | 说明 |
| :--- | :--- | :--- |
| `id` | `bigint` PK | 自增主键 |
| `topic` | `varchar(128)` | 事件主题（如 `iam.perm.invalidate`、`system.dict.invalidate`、`infra.file_config.invalidate`） |
| `tenant_id` | `bigint` | 关联租户 ID（全局事件为 0） |
| `payload` | `text` | JSON 事件载荷 |
| `status` | `tinyint` | `0-PENDING`（待投递）、`1-PUBLISHED`（已投递）、`2-FAILED`（超过最大重试次数） |
| `retry_count` | `int` | 已重试次数 |
| `next_retry_time` | `datetime` | 下次重试时间（索引：`idx_status_retry(status, next_retry_time)`） |
| `create_time` | `datetime` | 创建时间 |
| `update_time` | `datetime` | 更新时间 |

---

## 四、5 级数据权限表列映射配置

| 表名 | 部门列（DeptColumn） | 用户列（UserColumn） | 说明 |
| :--- | :--- | :--- | :--- |
| `system_users` | `dept_id` | `id` | 管理员按角色 DataScope 仅能查看/管理权限范围内的部门用户或本人 |
| `system_dept` | `id` | — | 管理员按角色 DataScope 仅能查看权限范围内的部门节点 |

---

## 五、迁移与初始化种子数据规范
- **双引擎验证**：所有仓储层 SQL 同时通过 **SQLite**（快速单测替身）与 **真实 MySQL 8.0**（集成测试 `make test-integration`）双重验证。
- **初始化种子数据**：
  - 默认系统租户（`id=1, name="芋道源码", package_id=0`）
  - 默认超级管理员（`id=1, username="admin", password=BCrypt("admin123")`）与内置角色 `super_admin`
  - 默认 OAuth2 客户端（`client_id="default"`）
  - 一期完整后台菜单与按钮权限树（系统管理 + 基础设施）
  - 核心数据字典与默认 DB 文件存储主配置（`id=1, storage=1, master=true`）
