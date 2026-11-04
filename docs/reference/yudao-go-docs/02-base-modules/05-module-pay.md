# DOC-05：yudao-module-pay 支付系统模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：统一收银台与多渠道资金清算基础模块（参考 [支付系统开发指南](https://doc.iocoder.cn/pay/build/)），屏蔽微信支付（V2/V3）、支付宝、会员电子钱包及模拟支付的底层接口与签名差异，向商城（`mall`）等上层模块提供**支付下单、原路退款、转账提现、电子钱包储值**及**可靠异步通知回调**。
- **Go 模块路径**：`internal/modules/pay`（对外跨模块契约：`internal/api/pay`）
- **架构 Review 核心结论**：
  1. **主支付单与渠道扩展单 1:N 解耦**：同一个 `pay_order`（关联商城订单）用户在收银台可能先点微信支付未付款、再切回点支付宝扫码付款。每次点击拉起渠道均生成一条具有独立外部单号 `no` 的 `pay_order_extension`，确保渠道单号不冲突；若极端情况下用户两边都完成了付款，后回调的一笔由于主单已处于 `已支付(10)` 状态，系统将自动识别为重复支付并触发原路退款或告警。
  2. **可靠异步回调通知（Outbox 模式）**：三方支付回调成功后，在同一数据库事务内写入 `pay_notify_task` 表，再由后台协程向业务系统的 `notify_url`（或进程内 Event）发起通知，失败时按 `[15s, 15s, 30s, 180s, 1800s, 1800s, 1800s, 3600s]` 阶梯重试。

## 二、Go 模块标准包目录结构

```text
internal/modules/pay/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/pay/*)
│   │   ├── app/                      # 支付应用 CRUD、状态切换
│   │   ├── channel/                  # 支付渠道配置 CRUD（微信/支付宝证书与密钥配置）
│   │   ├── demo/                     # 支付、退款与转账接入示例订单
│   │   ├── notify/                   # 三方支付/退款/转账异步回调免登网关 + 回调任务日志查询
│   │   ├── order/                    # 支付订单分页、详情、收银台提交、Excel 导出
│   │   ├── refund/                   # 退款订单分页、详情、Excel 导出
│   │   ├── transfer/                 # 转账提现单创建、分页、详情
│   │   └── wallet/                   # 会员电子钱包账户管理、余额流水、充值记录与充值套餐配置
│   └── app/                          # 移动端消费者接口 (/app-api/pay/*)
│       ├── channel/                  # 获取当前应用已开启的支付渠道编码列表
│       ├── order/                    # 移动端收银台获取订单详情与提交渠道支付
│       ├── transfer/                 # 移动端查询转账单状态（微信确认收款包）
│       └── wallet/                   # 移动端查询我的钱包余额、交易流水、发起充值与充值套餐列表
├── service/
│   ├── app/
│   ├── channel/
│   ├── demo/
│   ├── notify/
│   ├── order/
│   ├── refund/
│   ├── transfer/
│   └── wallet/
├── dal/
│   ├── dataobject/                   # 12 张 pay_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # PayNoRedisDAO (生成唯一支付/退款/转账外部单号序列), NotifyLockRedisDAO
├── api/                              # 实现 PayOrderApi, PayRefundApi, PayTransferApi, PayWalletApi
├── mq/                               # 支付回调通知任务异步执行消费者
├── job/                              # 过期未支付订单自动关单 Job、退款/转账结果主动轮询同步 Job、回调重试 Job
├── enums/                            # 模块错误码段 (1_007_000_000 起) 与渠道/状态枚举
├── framework/
│   └── pay/                          # 内聚多渠道支付客户端：WechatPayClient, AlipayClient, WalletPayClient, MockPayClient
└── module.go                         # 导出 pay.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `pay_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `pay_app` | **租户级 (`tenant_id`)** | `id`, `app_key`(唯一应用标识，如 `mall`), `name`, `status`, `remark`, `order_notify_url`, `refund_notify_url`, `transfer_notify_url` | `uk_app_key` (`app_key`, `tenant_id`) |
| 02 | `pay_channel` | **租户级 (`tenant_id`)** | `id`, `code`(`wx_pub`/`wx_lite`/`wx_app`/`wx_native`/`wx_wap`/`alipay_pc`/`alipay_wap`/`alipay_app`/`alipay_qr`/`alipay_bar`/`wallet`/`mock`), `status`, `remark`, `fee_rate`, `app_id`, `config`(JSON：存储商户号与公私钥证书) | `uk_app_code` (`app_id`, `code`) |
| 03 | `pay_order` | **租户级 (`tenant_id`)** | `id`, `app_id`, `channel_id`, `channel_code`, `merchant_order_id`(业务订单ID), `subject`, `body`, `notify_url`, `price`(分), `channel_fee_rate`, `channel_fee_price`, `status`(0未支付/10已支付/20已退款/30已关闭), `user_ip`, `expire_time`, `success_time`, `extension_id`, `no`, `refund_price`(累计已退分), `channel_user_id`, `channel_order_no` | `idx_app_merchant` (`app_id`, `merchant_order_id`) |
| 04 | `pay_order_extension` | **租户级 (`tenant_id`)** | `id`, `no`(外部商户单号 `out_trade_no` 如 `P2026110400001`), `order_id`, `channel_id`, `channel_code`, `user_ip`, `status`(0待支付/1已支付/2已关闭), `channel_extras`(JSON), `channel_error_code`, `channel_error_msg`, `channel_notify_data` | `uk_no` (`no`), `idx_order_id` (`order_id`) |
| 05 | `pay_refund` | **租户级 (`tenant_id`)** | `id`, `no`(外部退款单号 `R20261104...`), `app_id`, `channel_id`, `channel_code`, `order_id`, `order_no`, `merchant_order_id`, `merchant_refund_id`, `notify_url`, `status`(0待退款/10成功/20失败), `pay_price`, `refund_price`, `reason`, `user_ip`, `channel_order_no`, `channel_refund_no`, `success_time`, `channel_error_code`, `channel_error_msg`, `channel_notify_data` | `uk_no` (`no`), `idx_order_id` |
| 06 | `pay_transfer` | **租户级 (`tenant_id`)** | `id`, `no`(外部转账单号 `T20261104...`), `app_id`, `channel_id`, `channel_code`, `merchant_transfer_id`, `type`(1支付宝/2微信零钱/3银行卡/4电子钱包), `status`(0待转账/10转账中/20成功/30失败), `price`, `subject`, `user_name`, `alipay_logon_id`, `openid`, `notify_url`, `channel_transfer_no`, `success_time` | `uk_no` (`no`) |
| 07 | `pay_wallet` | **租户级 (`tenant_id`)** | `id`, `user_id`, `user_type`, `balance`(可用余额:分), `freeze_price`(提现或消费冻结:分), `total_expense`, `total_recharge` | `uk_user` (`user_id`, `user_type`, `tenant_id`) |
| 08 | `pay_wallet_transaction` | **租户级 (`tenant_id`)** | `id`, `wallet_id`, `biz_type`(1充值/2充值退款/3支付/4支付退款/5提现/6后台调整), `biz_id`, `no`, `title`, `price`(正负变动分), `balance`(变动后余额快照) | `idx_wallet_id` (`wallet_id`) |
| 09 | `pay_wallet_recharge` | **租户级 (`tenant_id`)** | `id`, `wallet_id`, `total_price`(实际到账总金额), `pay_price`(实付金额), `bonus_price`(赠送金额), `package_id`, `pay_status`, `pay_order_id`, `pay_channel_code`, `pay_time`, `refund_status`, `pay_refund_id`, `refund_total_price` | `idx_wallet_id` |
| 10 | `pay_wallet_recharge_package` | **租户级 (`tenant_id`)** | `id`, `name`, `pay_price`, `bonus_price`, `status` | 主键索引 `id` |
| 11 | `pay_notify_task` & `pay_notify_log` | **租户级 (`tenant_id`)** | `id`, `app_id`, `type`(1支付/2退款/3转账), `data_id`, `merchant_order_id`, `merchant_refund_id`, `merchant_transfer_id`, `status`(1等待/2成功/3失败/4请求成功但业务返回失败), `next_notify_time`, `last_execute_time`, `notify_times`, `max_notify_times`, `notify_url` 及每次回调响应日志 | `idx_next_notify` (`status`, `next_notify_time`) |
| 12 | `pay_demo_order` & `pay_demo_transfer` | **租户级 (`tenant_id`)** | 支付、退款与转账接入示例业务表 | 主键索引 `id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `pay:no_seq:%s` | 生成唯一支付外部单号（`P+yyyyMMddHHmmss+自增序列`）、退款单号（`R...`）、转账单号（`T...`）与钱包流水号（`W...`） |
| **Redis 分布式锁** | `pay:order:lock:%d` / `pay:notify:lock:%d` | 保护支付单提交与三方回调并发修改，以及防止多个定时任务实例同时执行同一条 `pay_notify_task` |
| **内置定时任务** | `payOrderExpireJob` / `payOrderSyncJob` | 自动关闭超时未支付的 `pay_order`；对创建时间超过 5 分钟仍处于“待支付”的扩展单主动调用微信/支付宝查单 API 兜底同步状态 |
| **内置定时任务** | `payRefundSyncJob` / `payTransferSyncJob` / `payNotifyJob` | 定期主动向三方渠道轮询处理中的退款单与转账单状态；扫描到期未完成的 `pay_notify_task` 执行重试回调 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 端类型 / 权限 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **应用与渠道** | `POST/PUT/DELETE/GET` | `/admin-api/pay/app/*`, `/admin-api/pay/channel/*` | `pay:app:*`, `pay:channel:*` | 支付应用与各支付渠道证书参数管理 |
| **收银台提交** | `POST` | `/admin-api/pay/order/submit`, `/app-api/pay/order/submit` | 双端已登录 | 选择渠道提交支付，返回 `{status, displayMode, displayContent}`（如二维码链接、唤醒微信小程序 JSAPI JSON 参数） |
| **三方回调网关** | `POST` | `/admin-api/pay/notify/{order,refund,transfer}/{channelId}` | `@PermitAll` 免登 | 接收微信支付 V2(XML)/V3(JSON+AES-GCM) 与支付宝(Form+RSA2) 的异步回调通知 |
| **退款与转账** | `GET / POST` | `/admin-api/pay/refund/*`, `/admin-api/pay/transfer/*` | `pay:refund:*`, `pay:transfer:*` | 查询退款单分页/详情、发起转账单与查询转账明细 |
| **会员电子钱包** | `GET / POST` | `/app-api/pay/wallet/get`, `/app-api/pay/wallet-transaction/page`, `/app-api/pay/wallet-recharge/create` | 移动端已登录 | C 端会员查询钱包余额、资金明细流水、创建钱包充值订单 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package pay

import "context"

// PayOrderApi 供商城交易、钱包充值等模块调用的支付单接口
type PayOrderApi interface {
	CreateOrder(ctx context.Context, req *PayOrderCreateReqDTO) (orderID int64, err error)
	GetOrder(ctx context.Context, id int64) (*PayOrderRespDTO, error)
	UpdatePayOrderPrice(ctx context.Context, id int64, payPrice int) error
}

// PayRefundApi 供商城售后模块调用的退款单接口
type PayRefundApi interface {
	CreateRefund(ctx context.Context, req *PayRefundCreateReqDTO) (refundID int64, err error)
	GetRefund(ctx context.Context, id int64) (*PayRefundRespDTO, error)
}

// PayTransferApi 供商城分销佣金提现调用的转账单接口
type PayTransferApi interface {
	CreateTransfer(ctx context.Context, req *PayTransferCreateReqDTO) (transferID int64, err error)
	GetTransfer(ctx context.Context, id int64) (*PayTransferRespDTO, error)
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **部分退款与累计退款上限校验**：发起退款时，必须校验 `payOrder.RefundPrice + req.RefundPrice <= payOrder.Price`，防止多次部分退款总额超过原实付金额。
- [x] **电子钱包扣款防并发超支**：使用会员钱包渠道（`channel_code = "wallet"`）支付时，扣减余额 SQL 必须携带乐观条件 `UPDATE pay_wallet SET balance = balance - ?, total_expense = total_expense + ? WHERE id = ? AND balance >= ?`。
