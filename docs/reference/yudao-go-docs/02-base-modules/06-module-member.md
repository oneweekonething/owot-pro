# DOC-06：yudao-module-member 会员中心模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：面向 C 端消费者（微信小程序、H5、App）的统一账号与会员运营基础模块（参考 [会员中心开发指南](https://doc.iocoder.cn/member/build/)），为商城（`mall`）、支付钱包（`pay`）、即时通讯（`im`）提供 C 端身份认证、收货地址、用户画像（标签/分组）以及会员留存体系（等级、成长值、积分、签到）。
- **Go 模块路径**：`internal/modules/member`（对外跨模块契约：`internal/api/member`）
- **架构 Review 核心结论**：
  1. **基础设施复用与物理表隔离**：`member` 模块的登录 Token 签发直接调用 `internal/api/system.OAuth2TokenApi`（传入 `UserType = 2`），微信 OpenID 绑定直接调用 `internal/api/system.SocialUserApi`，短信验证码直接调用 `internal/api/system.SmsCodeApi`，既复用了 `system` 模块全部安全认证能力，又保证了 `member_user` 表与后台 `system_users` 表彻底解耦。
  2. **积分与成长值强一致性**：积分变动与经验值升级必须在数据库事务内完成，并记录不可篡改的 `member_point_record` 与 `member_experience_record` 流水。

## 二、Go 模块标准包目录结构

```text
internal/modules/member/
├── controller/
│   ├── admin/                        # 管理后台运营接口 (/admin-api/member/*)
│   │   ├── address/                  # 查询指定会员的收货地址列表
│   │   ├── config/                   # 会员积分抵扣与赠送全局配置
│   │   ├── group/                    # 会员分组 CRUD
│   │   ├── level/                    # 会员等级配置 CRUD、等级变动记录与经验值记录分页
│   │   ├── point/                    # 会员积分变动明细分页
│   │   ├── signin/                   # 连续签到天数奖励规则 CRUD、会员签到记录分页
│   │   ├── tag/                      # 会员标签 CRUD
│   │   └── user/                     # 会员档案分页、详情、修改资料、手动调整会员等级与积分
│   └── app/                          # C 端消费者接口 (/app-api/member/*)
│       ├── address/                  # 我的收货地址 CRUD、获取默认地址
│       ├── auth/                     # 账密登录、短信验证码登录、社交/微信小程序一键登录、登出、刷新 Token
│       ├── level/                    # 获取会员等级规则列表与我的经验值记录
│       ├── point/                    # 获取我的积分明细分页
│       ├── signin/                   # 获取签到规则、获取个人签到统计（连续签到天数/今日是否已签）、立即签到打卡
│       ├── social/                   # C 端绑定/解绑微信、获取微信小程序码与订阅消息模板
│       └── user/                     # 获取我的会员信息、修改昵称头像性别生日、换绑手机号、修改/重置密码
├── service/
│   ├── address/
│   ├── auth/
│   ├── config/
│   ├── group/
│   ├── level/
│   ├── point/
│   ├── signin/
│   ├── tag/
│   └── user/
├── dal/
│   ├── dataobject/                   # 11 张 member_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # SignInLockRedisDAO (防并发重复签到锁)
├── api/                              # 实现 MemberUserApi, MemberLevelApi, MemberPointApi, MemberAddressApi, MemberConfigApi
├── enums/                            # 模块错误码段 (1_004_000_000 起) 与积分/经验业务类型枚举
├── convert/                          # DO <-> DTO <-> VO 转换函数
└── module.go                         # 导出 member.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（11 张表，前缀 `member_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `member_user` | **租户级 (`tenant_id`)** | `id`, `mobile`, `password`(BCrypt), `status`(0正常/1禁用), `register_ip`, `register_terminal`, `login_ip`, `login_date`, `nickname`, `avatar`, `name`, `sex`, `area_id`, `birthday`, `mark`, `point`(可用积分), `tag_ids`(JSON数组), `level_id`, `experience`(成长值), `group_id` | `uk_mobile` (`mobile`, `tenant_id`, `deleted`) |
| 02 | `member_address` | **租户级 (`tenant_id`)** | `id`, `user_id`, `name`(收件人), `mobile`, `area_id`(省市区ID), `detail_address`, `default_status`(bool) | `idx_user_id` (`user_id`) |
| 03 | `member_tag` | **租户级 (`tenant_id`)** | `id`, `name` | `uk_name` (`name`, `tenant_id`) |
| 04 | `member_group` | **租户级 (`tenant_id`)** | `id`, `name`, `remark`, `status` | `uk_name` (`name`, `tenant_id`) |
| 05 | `member_level` | **租户级 (`tenant_id`)** | `id`, `name`, `level`(等级序号), `experience`(升级所需经验门槛), `discount_percent`(折扣率如 95), `icon`, `background_url`, `status` | `uk_level` (`level`, `tenant_id`) |
| 06 | `member_level_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `level_id`, `level`, `discount_percent`, `experience`, `user_experience`, `remark`, `description` | `idx_user_id` (`user_id`) |
| 07 | `member_experience_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `biz_id`, `biz_type`, `title`, `description`, `experience`(正负变动值), `total_experience`(变动后快照) | `idx_user_id` (`user_id`) |
| 08 | `member_config` | **租户级 (`tenant_id`)** | `id`, `point_trade_deduct_enable`, `point_trade_deduct_unit_price`, `point_trade_deduct_max_price`, `point_trade_give_point` | 单租户单行配置 |
| 09 | `member_point_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `biz_id`, `biz_type`, `title`, `description`, `point`(正负变动积分), `total_point`(变动后快照) | `idx_user_id` (`user_id`) |
| 10 | `member_sign_in_config` | **租户级 (`tenant_id`)** | `id`, `day`(连续第 N 天), `point`(奖励积分), `experience`(奖励经验), `status` | `uk_day` (`day`, `tenant_id`) |
| 11 | `member_sign_in_record` | **租户级 (`tenant_id`)** | `id`, `user_id`, `day`, `point`, `experience` | `idx_user_create` (`user_id`, `create_time`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **Redis 分布式锁** | `member:signin:lock:%d` | 会员点击每日签到时加锁（锁粒度：`userID`），防止高并发双击刷取多份今日签到积分与经验 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 端类型 / 权限 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **C端认证** | `POST` | `/app-api/member/auth/{login,sms-login,social-login,weixin-mini-app-login,logout,refresh-token}` | `@PermitAll` | 覆盖手机号密码登录、短信验证码自动注册登录、公众号/小程序授权一键登录与 Token 刷新 |
| **C端个人中心** | `GET / PUT` | `/app-api/member/user/{get,update,update-mobile,update-password,reset-password}` | 移动端已登录 / `@PermitAll`(重置密码) | 查询个人资料（含等级图标、积分、经验）、修改头像昵称、短信验证换绑手机 |
| **C端收货地址** | `POST/PUT/DELETE/GET` | `/app-api/member/address/{create,update,delete,get,list,get-default}` | 移动端已登录 | 管理个人收货地址与默认地址切换 |
| **C端签到与积分** | `POST / GET` | `/app-api/member/sign-in/record/{create,get-summary,page}`, `/app-api/member/point/record/page` | 移动端已登录 | 每日签到打卡、查询连续签到统计与积分收支流水 |
| **管理端会员运营** | `GET / PUT` | `/admin-api/member/user/{page,get,update,update-level,update-point}` | `member:user:*` | 后台多条件筛选会员（按标签/分组/等级/注册时间）、手动调整会员等级与积分 |
| **管理端规则配置** | `POST/PUT/DELETE/GET` | `/admin-api/member/{tag,group,level,sign-in/config,config}/*` | `member:*` | 标签、分组、等级门槛、连续签到周期奖励与积分抵扣规则管理 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package member

import "context"

// MemberUserApi 供商城、支付、IM、统计模块查询 C 端会员信息的契约
type MemberUserApi interface {
	GetUser(ctx context.Context, id int64) (*MemberUserRespDTO, error)
	GetUserList(ctx context.Context, ids []int64) ([]*MemberUserRespDTO, error)
	GetUserListByNickname(ctx context.Context, nickname string) ([]*MemberUserRespDTO, error)
	GetUserByMobile(ctx context.Context, mobile string) (*MemberUserRespDTO, error)
}

// MemberPointApi 供商城下单抵扣、取消订单退还、支付成功奖励调用的积分契约
type MemberPointApi interface {
	AddPoint(ctx context.Context, userID int64, point int, bizType int, bizID string) error
	ReducePoint(ctx context.Context, userID int64, point int, bizType int, bizID string) error
}

// MemberLevelApi 供商城计算 VIP 会员价与支付成功发放经验值调用的等级契约
type MemberLevelApi interface {
	GetMemberLevel(ctx context.Context, id int64) (*MemberLevelRespDTO, error)
	AddExperience(ctx context.Context, userID int64, experience int, bizType int, bizID string) error
}

// MemberAddressApi 供商城下单计算快递运费与收货人信息调用的地址契约
type MemberAddressApi interface {
	GetAddress(ctx context.Context, id int64, userID int64) (*MemberAddressRespDTO, error)
	GetDefaultAddress(ctx context.Context, userID int64) (*MemberAddressRespDTO, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **连续签到断签重置与周期循环检查**：若昨日未签到，今日签到连续天数 `day` 重置为 `1`；若昨日已签（连续第 `k` 天），且配置的最大签到周期为 `N` 天（如 7 天一循环），当 `k >= N` 时今日签到重置为第 `1` 天，否则为 `k + 1` 天。
- [x] **默认收货地址唯一性检查**：当新增或修改地址设置 `default_status = true` 时，必须在同一事务内先将该用户原有 `default_status = true` 的地址更新为 `false`。
