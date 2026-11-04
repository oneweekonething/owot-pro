# DOC-02：yudao-module-system 系统功能模块（必选核心基础模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：全平台的身份认证中心（IAM）、RBAC 菜单与按钮权限底座、5 级部门数据权限中枢、SaaS 多租户与套餐管控中心，以及统一消息通知渠道（短信、邮件、站内信）。
- **Go 模块路径**：`internal/modules/system`（对外跨模块契约：`internal/api/system`）
- **架构 Review 核心结论**：
  1. **全局表 vs 租户隔离表严格区分**：`system` 模块共 29 张表，其中 13 张为租户隔离表（含 `tenant_id`），16 张为系统全局表（如 `system_menu`、`system_dict_data`、`system_tenant`、`system_sms_channel`）。在注册 `pkg/biz/tenant` 插件时，必须将这 16 张全局表配置进 `ignoreTables` 白名单。
  2. **高频权限与字典查询零 DB 压力**：角色权限、部门树、数据字典、敏感词在每次请求中均可能被调用，Go 实现采用 `ristretto` 进程内缓存 + `Watermill` Redis Pub/Sub 广播刷新机制，确保平均查询耗时 `<10μs`。
  3. **密码算法无缝兼容**：原项目使用 Spring Security `BCryptPasswordEncoder`（前缀 `$2a$10$...`），Go 标准扩展库 `golang.org/x/crypto/bcrypt` 原生支持 `$2a$` 格式，老数据库用户无需重置密码即可直接登录。

## 二、Go 模块标准包目录结构

```text
internal/modules/system/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/system/*)
│   │   ├── auth/                     # 登录、登出、刷新 Token、获取权限菜单树、短信/社交快捷登录
│   │   ├── captcha/                  # 滑动拼图与点选验证码获取与校验
│   │   ├── dept/                     # 部门树 CRUD 与岗位 CRUD
│   │   ├── dict/                     # 字典类型与字典数据项管理
│   │   ├── errorcode/                # 动态错误码管理
│   │   ├── ip/                       # 省市区行政区划树与 IP 归属地查询
│   │   ├── logger/                   # 操作日志与登录日志分页查询与导出
│   │   ├── mail/                     # 邮箱账号、邮件模板与发送日志
│   │   ├── notice/                   # 系统通知公告发布管理
│   │   ├── notify/                   # 站内信模板与我的站内信列表（已读/未读统计）
│   │   ├── oauth2/                   # OAuth2 客户端管理、在线用户 Token 监控与强退、SSO 授权页
│   │   ├── permission/               # 菜单管理、角色管理、赋予角色菜单权限/数据权限、赋予用户角色
│   │   ├── sensitiveword/            # 敏感词 CRUD 与文本检测验证
│   │   ├── sms/                      # 短信渠道、短信模板、短信发送日志与渠道异步回调
│   │   ├── social/                   # 三方社交客户端配置、社交用户与绑定记录
│   │   ├── tenant/                   # SaaS 租户 CRUD、租户套餐 CRUD、域名/名称解析租户 ID
│   │   └── user/                     # 后台用户 CRUD、重置密码、修改状态、个人中心资料修改、Excel 导入导出
│   └── app/                          # 移动端 App 接口 (/app-api/system/*)
│       ├── dict/                     # 移动端数据字典查询
│       ├── ip/                       # 移动端省市区树查询
│       └── tenant/                   # 移动端租户信息获取
├── service/                          # 15 个核心业务子域 Service 接口与实现
│   ├── auth/
│   ├── dept/
│   ├── dict/
│   ├── errorcode/
│   ├── logger/
│   ├── mail/
│   ├── notice/
│   ├── notify/
│   ├── oauth2/
│   ├── permission/
│   ├── sensitiveword/
│   ├── sms/
│   ├── social/
│   ├── tenant/
│   └── user/
├── dal/
│   ├── dataobject/                   # 29 张数据库表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO 与自定义 Mapper
│   └── redis/                        # OAuth2AccessTokenRedisDAO, CaptchaRedisDAO, SocialStateRedisDAO
├── api/                              # 实现 internal/api/system 中定义的 12 个跨模块接口
├── mq/                               # 短信/邮件异步发送消费者、角色菜单/字典/敏感词缓存刷新广播
├── job/                              # 过期 Token 清理任务
├── enums/                            # 模块错误码段 (1_002_000_000 起) 与业务枚举常量
├── convert/                          # DO <-> DTO <-> VO 类型转换函数
├── framework/
│   ├── captcha/                      # 内聚滑块验证码组件实现
│   └── sms/                          # 阿里云、腾讯云、华为云、调试钉钉短信客户端实现
└── module.go                         # 导出 system.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（29 张表，前缀 `system_`）

| 序号 | 表名 | 租户隔离 | 主键与核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `system_users` | **租户级 (`tenant_id`)** | `id`, `username`, `password`(BCrypt), `nickname`, `remark`, `dept_id`, `post_ids`(JSON数组), `email`, `mobile`, `sex`, `avatar`, `status`, `login_ip`, `login_date` | `uk_username_tenant` (`username`, `deleted`, `tenant_id`) |
| 02 | `system_role` | **租户级 (`tenant_id`)** | `id`, `name`, `code`(`super_admin`/`tenant_admin`等), `sort`, `data_scope`(1~5), `data_scope_dept_ids`(JSON), `status`, `type`(1内置/2自定义) | `idx_tenant_code` (`tenant_id`, `code`) |
| 03 | `system_menu` | 全局表（无 `tenant_id`） | `id`, `name`, `permission`(如 `system:user:create`), `type`(1目录/2菜单/3按钮), `sort`, `parent_id`, `path`, `icon`, `component`, `component_name`, `status`, `visible`, `keep_alive`, `always_show` | `idx_parent_id` (`parent_id`) |
| 04 | `system_user_role` | **租户级 (`tenant_id`)** | `id`, `user_id`, `role_id` | `idx_user_id`, `idx_role_id` |
| 05 | `system_role_menu` | **租户级 (`tenant_id`)** | `id`, `role_id`, `menu_id` | `idx_role_menu` (`role_id`, `menu_id`) |
| 06 | `system_dept` | **租户级 (`tenant_id`)** | `id`, `name`, `parent_id`, `sort`, `leader_user_id`, `phone`, `email`, `status` | `idx_parent_id` (`parent_id`, `tenant_id`) |
| 07 | `system_post` | **租户级 (`tenant_id`)** | `id`, `code`, `name`, `sort`, `status`, `remark` | `idx_tenant_code` (`tenant_id`, `code`) |
| 08 | `system_user_post` | **租户级 (`tenant_id`)** | `id`, `user_id`, `post_id` | `idx_user_post` (`user_id`, `post_id`) |
| 09 | `system_tenant` | 全局表 | `id`, `name`, `contact_user_id`, `contact_name`, `contact_mobile`, `status`, `websites`(绑定域名JSON数组), `package_id`(0表示系统租户), `expire_time`, `account_count`(最大账号配额) | `idx_name` (`name`) |
| 10 | `system_tenant_package` | 全局表 | `id`, `name`, `status`, `remark`, `menu_ids`(JSON数组，定义该套餐开放的菜单ID集合) | 主键索引 `id` |
| 11 | `system_oauth2_client` | 全局表 | `id`, `client_id`, `secret`, `name`, `logo`, `description`, `status`, `access_token_validity_seconds`, `refresh_token_validity_seconds`, `redirect_uris`, `authorized_grant_types`, `scopes`, `auto_approve_scopes` | `uk_client_id` (`client_id`) |
| 12 | `system_oauth2_access_token` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`(1管理员/2会员), `user_info`(JSON), `access_token`, `refresh_token`, `client_id`, `scopes`, `expires_time` | `idx_access_token`, `idx_refresh_token` |
| 13 | `system_oauth2_refresh_token` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `refresh_token`, `client_id`, `scopes`, `expires_time` | `idx_refresh_token` |
| 14 | `system_oauth2_code` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `code`, `client_id`, `scopes`, `expires_time`, `redirect_uri`, `state` | `idx_code` (`code`) |
| 15 | `system_oauth2_approve` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `client_id`, `scope`, `approved`, `expires_time` | `idx_user_client` (`user_id`, `client_id`) |
| 16 | `system_social_client` | **租户级 (`tenant_id`)** | `id`, `name`, `social_type`(10公众号/20企微/30钉钉/31小程序), `user_type`, `client_id`, `client_secret`, `agent_id`, `status` | `idx_tenant_social` (`tenant_id`, `social_type`, `user_type`) |
| 17 | `system_social_user` | **租户级 (`tenant_id`)** | `id`, `type`, `openid`, `token`, `raw_token_info`, `nickname`, `avatar`, `raw_user_info`, `code`, `state` | `idx_type_openid` (`type`, `openid`) |
| 18 | `system_social_user_bind` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `social_type`, `social_user_id` | `idx_user_social` (`user_id`, `user_type`, `social_type`) |
| 19 | `system_dict_type` | 全局表 | `id`, `name`, `type`(如 `common_status`), `status`, `remark` | `uk_type` (`type`) |
| 20 | `system_dict_data` | 全局表 | `id`, `sort`, `label`, `value`, `dict_type`, `status`, `color_type`, `css_class`, `remark` | `idx_dict_type` (`dict_type`) |
| 21 | `system_sms_channel` | 全局表 | `id`, `signature`, `code`(`ALIYUN`/`TENCENT`/`HUAWEI`), `status`, `remark`, `api_key`, `api_secret`, `callback_url` | `idx_code` (`code`) |
| 22 | `system_sms_template` | 全局表 | `id`, `type`, `status`, `code`, `name`, `content`, `params`(JSON数组), `remark`, `api_template_id`, `channel_id`, `channel_code` | `uk_code` (`code`) |
| 23 | `system_sms_log` | 全局表 | `id`, `channel_id`, `channel_code`, `template_id`, `template_code`, `template_type`, `template_content`, `template_params`, `api_template_id`, `mobile`, `user_id`, `user_type`, `send_status`, `send_time`, `api_send_code`, `api_serial_no`, `receive_status`, `receive_time` | `idx_mobile` (`mobile`), `idx_send_time` |
| 24 | `system_sms_code` | **租户级 (`tenant_id`)** | `id`, `mobile`, `code`, `create_ip`, `scene`(1登录/2修改手机/3忘记密码), `today_index`, `used`, `used_time`, `used_ip` | `idx_mobile_scene` (`mobile`, `scene`) |
| 25 | `system_mail_account` | 全局表 | `id`, `mail`, `username`, `password`, `host`, `port`, `ssl_enable`, `starttls_enable` | `idx_mail` (`mail`) |
| 26 | `system_mail_template` | 全局表 | `id`, `name`, `code`, `account_id`, `nickname`, `title`, `content`, `params`, `status`, `remark` | `uk_code` (`code`) |
| 27 | `system_mail_log` | 全局表 | `id`, `user_id`, `user_type`, `to_mail`, `account_id`, `from_mail`, `template_id`, `template_code`, `template_nickname`, `template_title`, `template_content`, `template_params`, `send_status`, `send_time`, `send_exception` | `idx_to_mail` (`to_mail`) |
| 28 | `system_notify_template` & `system_notify_message` | 全局模板 / **租户级消息** | 模板：`id`, `name`, `code`, `nickname`, `content`, `type`, `params`, `status`；消息：`id`, `user_id`, `user_type`, `template_id`, `template_code`, `template_nickname`, `template_content`, `template_type`, `template_params`, `read_status`, `read_time` | `idx_user_read` (`user_id`, `user_type`, `read_status`) |
| 29 | `system_notice` / `system_sensitive_word` / `system_error_code` / `system_operate_log` / `system_login_log` | 公告与日志为**租户级**；敏感词与错误码为全局表 | 覆盖通知公告、敏感词库（含标签）、动态错误码、操作审计日志（模块/名/类型/方法/参数/结果/耗时/biz_id）与登录审计日志 | `idx_create_time`, `idx_user_id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与消费/过期策略 |
| :--- | :--- | :--- |
| **RedisDAO** | `oauth2_access_token:%s` | 签发或校验 AccessToken 时读写，TTL 等于 Token 剩余有效期（默认 30 分钟），强退用户时立即删除 |
| **RedisDAO** | `captcha_code:%s` | 生成滑块验证码时写入 AES 坐标值，TTL 2 分钟，校验一次后立即 `DEL` 防重放 |
| **MQ 集群消费** | `system.sms.send` (`SmsSendMessage`) | 调用 `SmsSendService.SendSingleSms` 落库 `system_sms_log` 后异步投递，由消费者调用阿里云/腾讯云 SDK 发送 |
| **MQ 集群消费** | `system.mail.send` (`MailSendMessage`) | 调用 `MailSendService.SendSingleMail` 落库 `system_mail_log` 后异步投递，由消费者通过 SMTP 发送邮件 |
| **MQ 广播刷新** | `system.cache.refresh` | 当菜单、角色权限、部门树、字典、敏感词、短信/邮件模板变更时广播，各节点收到后刷新本地 `ristretto` 缓存 |

## 五、全量 RESTful API 路由契约表（核心接口）

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/login` | `@PermitAll` | 账号密码登录（支持校验滑块验证码 `captchaVerification`），返回 `{userId, accessToken, refreshToken, expiresTime}` |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/refresh-token` | `@PermitAll` | 传入 Query 参数 `refreshToken` 换取新的双 Token |
| **认证 (`auth`)** | `GET` | `/admin-api/system/auth/get-permission-info` | 已登录即可 | 返回当前登录管理员的 `{user, roles: string[], permissions: string[], menus: MenuVO[]}` |
| **认证 (`auth`)** | `POST` | `/admin-api/system/auth/logout` | `@PermitAll` | 提取 Header Token 并从数据库及 Redis 中同步移除会话 |
| **用户 (`user`)** | `GET` | `/admin-api/system/user/page` | `system:user:query` | 用户分页查询（若传入 `deptId`，自动通过内存部门树展开包含其所有子孙部门的用户） |
| **用户 (`user`)** | `POST/PUT/DELETE` | `/admin-api/system/user/{create,update,delete}` | `system:user:{create,update,delete}` | 创建用户（校验租户账号配额 `account_count`）、更新用户、删除用户 |
| **用户 (`user`)** | `PUT` | `/admin-api/system/user/update-password` | `system:user:update-password` | 管理员重置指定用户的登录密码（BCrypt Hash） |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-role-menu` | `system:permission:assign-role-menu` | 给角色分配菜单权限（对比差异集执行批量新增与软删除，并触发缓存刷新广播） |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-role-data-scope` | `system:permission:assign-role-data-scope` | 给角色配置 5 种数据范围权限及自定义部门 ID 集合 |
| **权限 (`permission`)** | `POST` | `/admin-api/system/permission/assign-user-role` | `system:permission:assign-user-role` | 给管理员分配角色列表 |
| **租户 (`tenant`)** | `GET` | `/admin-api/system/tenant/{get-id-by-name,get-by-website}` | `@PermitAll` | 登录页根据租户名称或当前访问域名解析 `tenantId` |
| **租户 (`tenant`)** | `POST/PUT` | `/admin-api/system/tenant/{create,update}` | `system:tenant:{create,update}` | 创建新租户（级联创建默认部门、管理员角色与初始账号）；更新租户套餐时自动回收超出新套餐范围的角色菜单权限 |
| **短信 (`sms`)** | `POST` | `/admin-api/system/sms-callback/{channelCode}` | `@PermitAll` | 接收阿里云/腾讯云/华为云短信网关的回执状态推送，更新 `system_sms_log.receive_status` |
| **站内信 (`notify`)** | `GET/PUT` | `/admin-api/system/notify-message/{my-page,get-unread-count,update-read}` | 已登录即可 | 个人站内信分页、未读数轮询与标记已读 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package system

import "context"

// AdminUserApi 供全系统查询后台管理员信息的跨模块契约
type AdminUserApi interface {
	GetUser(ctx context.Context, id int64) (*AdminUserRespDTO, error)
	GetUserList(ctx context.Context, ids []int64) ([]*AdminUserRespDTO, error)
	GetUserListByDeptIds(ctx context.Context, deptIds []int64) ([]*AdminUserRespDTO, error)
	GetUserListByPostIds(ctx context.Context, postIds []int64) ([]*AdminUserRespDTO, error)
	ValidateUserList(ctx context.Context, ids []int64) error
}

// PermissionApi 供 security 与 bpm/dataperm 调用的权限契约
type PermissionApi interface {
	HasAnyPermissions(ctx context.Context, userID int64, permissions ...string) (bool, error)
	HasAnyRoles(ctx context.Context, userID int64, roles ...string) (bool, error)
	GetDeptDataPermission(ctx context.Context, userID int64) (*DeptDataPermissionRespDTO, error)
	GetUserRoleIdListByRoleIds(ctx context.Context, roleIds []int64) ([]int64, error)
}

// NotifyMessageSendApi 供各业务模块向管理员或会员发送站内信的契约
type NotifyMessageSendApi interface {
	SendSingleMessageToAdmin(ctx context.Context, userID int64, templateCode string, params map[string]interface{}) (int64, error)
	SendSingleMessageToMember(ctx context.Context, userID int64, templateCode string, params map[string]interface{}) (int64, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **租户套餐变更权限回收校验**：当修改租户绑定的 `package_id` 或修改 `system_tenant_package.menu_ids` 时，必须在事务中遍历该租户下的所有角色：对 `tenant_admin` 角色同步增删菜单，对普通自定义角色自动删除不再属于新套餐的 `menu_id`。
- [x] **超级管理员豁免校验**：拥有 `super_admin` 角色编码的用户默认拥有所有菜单按钮权限及全部数据权限（`All = true`）。
- [x] **部门树防环校验**：更新部门 `parent_id` 时，必须检查新父部门不能是当前部门自身或其子孙部门。
