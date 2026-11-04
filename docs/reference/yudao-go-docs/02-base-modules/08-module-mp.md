# DOC-08：yudao-module-mp 微信公众号模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：多微信公众号集中托管运营基础模块（参考 [微信公众号开发指南](https://doc.iocoder.cn/mp/build/)），支持在同一个后台接入并管理多个服务号/订阅号，涵盖账号配置、粉丝同步与标签管理、消息接收与人工客服主动回复、模板消息、自动回复（关注回复/消息兜底回复/关键词精准或半匹配回复）、自定义菜单、永久/临时素材库、图文草稿箱与发表记录、公众号数据统计。
- **Go 模块路径**：`internal/modules/mp`
- **架构 Review 核心结论**：
  1. **多实例微信 AccessToken 分布式共享**：在集群部署下，多个 Go 实例调用微信接口必须共享同一个微信公众号 `access_token`（否则相互刷新会导致旧 Token 失效报错 `40001`）。通过实现 `silenceper/wechat/v2/cache.Cache` 接口并底层桥接 `pkg/cache`（Redis），实现全集群 `access_token` 统一缓存与分布式锁刷新。
  2. **微信临时媒体素材自动转存 S3**：粉丝发送给公众号的图片、语音、视频在微信服务器仅保留 3 天（临时素材 `media_id`）。收到粉丝多媒体消息时，后台异步协程自动调用微信 Media Download API 下载二进制流，并通过 `internal/api/infra.FileApi` 上传至自有 S3 存储，将永久 URL 写入 `mp_message.media_url`。

## 二、Go 模块标准包目录结构

```text
internal/modules/mp/
├── controller/
│   └── admin/                        # 管理后台与微信开放回调接口 (/admin-api/mp/*)
│       ├── account/                  # 公众号账号 CRUD、生成二维码、清零 API 配额
│       ├── autoreply/                # 自动回复规则 CRUD（关注时回复、收到消息回复、关键词回复）
│       ├── draft/                    # 公众号图文草稿箱 CRUD（直接对接微信 Draft API）
│       ├── freepublish/              # 公众号图文发表记录分页、提交群发发表与删除
│       ├── material/                 # 永久素材（图片/语音/视频）与临时素材上传、分页、删除
│       ├── menu/                     # 自定义菜单树保存（同步发布至微信）与删除清空
│       ├── message/                  # 粉丝消息历史分页、人工客服主动向粉丝发送消息、模板消息推送
│       ├── open/                     # 微信服务器 GET 签名校验与 POST XML 消息/事件回调入口 (/admin-api/mp/open/:appId)
│       ├── statistics/               # 粉丝增减、累计用户、消息概况、接口分析统计查询
│       ├── tag/                      # 公众号标签 CRUD 与全量同步
│       └── user/                     # 公众号粉丝分页、详情、修改备注与打标签、异步全量同步粉丝
├── service/
│   ├── account/
│   ├── autoreply/
│   ├── material/
│   ├── menu/
│   ├── message/
│   ├── open/                         # 微信被动消息与事件回调责任链分发器
│   ├── statistics/
│   ├── tag/
│   └── user/
├── dal/
│   ├── dataobject/                   # 7 张 mp_* 表对应的 GORM DO 结构体
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # WechatAccessTokenRedisCache (实现 wechat/v2 cache.Cache 接口)
├── mq/                               # 公众号账号配置变更广播消费者（刷新各节点 MpClientFactory）
├── enums/                            # 模块错误码段 (1_006_000_000 起) 与微信事件/消息类型常量
├── framework/
│   └── mp/                           # MpClientFactory 多公众号客户端实例池与图文素材转换器
└── module.go                         # 导出 mp.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（7 张表，前缀 `mp_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `mp_account` | **租户级 (`tenant_id`)** | `id`, `name`(公众号名称), `account`(微信号), `app_id`(微信 AppID), `app_secret`, `token`, `aes_key`(消息加解密密钥), `qr_code_url`(公众号二维码 URL), `remark` | `uk_app_id` (`app_id`) |
| 02 | `mp_user` | **租户级 (`tenant_id`)** | `id`, `openid`, `union_id`, `subscribe_status`(0已关注/1已取关), `subscribe_time`, `unsubscribe_time`, `nickname`, `head_image_url`, `language`, `country`, `province`, `city`, `remark`, `tag_ids`(JSON数组), `account_id`, `app_id` | `uk_app_openid` (`app_id`, `openid`) |
| 03 | `mp_tag` | **租户级 (`tenant_id`)** | `id`, `wx_tag_id`(微信端返回的标签ID), `name`, `count`(粉丝数), `account_id`, `app_id` | `idx_account_id` (`account_id`) |
| 04 | `mp_message` | **租户级 (`tenant_id`)** | `id`, `msg_id`(微信消息ID), `account_id`, `app_id`, `user_id`(关联 `mp_user.id`), `openid`, `type`(`text`/`image`/`voice`/`video`/`shortvideo`/`location`/`link`/`music`/`news`), `send_from`(1粉丝发给公众号/2公众号发给粉丝), `content`, `media_id`, `media_url`(转存 S3 后的永久 URL), `recognition`(语音识别文本), `format`, `title`, `description`, `thumb_media_id`, `thumb_media_url`, `url`, `location_x`, `location_y`, `scale`, `label`, `articles`(图文 JSON), `music_url`, `hq_music_url`, `event`, `event_key` | `idx_account_user` (`account_id`, `user_id`, `create_time`) |
| 05 | `mp_auto_reply` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `type`(1关注时回复/2收到消息回复/3关键词回复), `request_keyword`, `request_match`(1全匹配/2半匹配包含), `request_message_type`, `response_message_type`, `response_content`, `response_media_id`, `response_media_url`, `response_title`, `response_description`, `response_thumb_media_id`, `response_thumb_media_url`, `response_articles`, `response_music_url`, `response_hq_music_url` | `idx_account_type` (`account_id`, `type`) |
| 06 | `mp_menu` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `name`, `menu_key`, `parent_id`, `type`(`view`/`click`/`miniprogram`/`scancode_waitmsg`/`pic_sysphoto`等), `url`, `mini_program_app_id`, `mini_program_page_path`, `article_id`, `reply_message_type`, `reply_content`, `reply_media_id`, `reply_media_url`, `reply_articles` | `idx_account_parent` (`account_id`, `parent_id`) |
| 07 | `mp_material` | **租户级 (`tenant_id`)** | `id`, `account_id`, `app_id`, `media_id`(微信素材ID), `type`(`image`/`voice`/`video`/`thumb`), `permanent`(bool 是否永久素材), `url`(转存自有 S3 地址), `name`, `mp_url`(微信端图片URL), `title`, `introduction` | `idx_account_media` (`account_id`, `media_id`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `mp:wechat_access_token:%s` | 缓存指定 `appId` 的微信公众号 `access_token`（TTL 7000 秒），供全集群实例共享调用 |
| **RedisDAO** | `mp:msg_dedup:%d` | 微信服务器若在 5 秒内未收到响应会重试推送 3 次相同 `MsgId` 的 XML 报文；使用 `SETNX mp:msg_dedup:{msgId}`（TTL 60 秒）实现回调幂等去重 |
| **MQ 广播刷新** | `mp.account.refresh` | 当后台新增、修改或删除 `mp_account` 时广播，各实例刷新 `MpClientFactory` 中的 `OfficialAccount` 实例 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **微信开放回调** | `GET / POST` | `/admin-api/mp/open/{appId}` | `@PermitAll` | `GET` 处理微信公众号服务器 URL 签名验证（`echostr`）；`POST` 接收微信推送的加密/明文 XML 消息与事件 |
| **账号管理** | `POST/PUT/DELETE/GET` | `/admin-api/mp/account/{create,update,delete,get,page,list-simple,generate-qr-code,clear-quota}` | `mp:account:*` | 公众号账号 CRUD、一键生成公众号二维码、清零公众号每月 API 调用配额 |
| **粉丝管理** | `GET / PUT / POST` | `/admin-api/mp/user/{page,get,update,sync}` | `mp:user:*` | 粉丝分页与详情、更新粉丝备注与标签、异步全量从微信服务器拉取同步粉丝 |
| **标签管理** | `POST/PUT/DELETE/GET` | `/admin-api/mp/tag/{create,update,delete,page,list-simple,sync}` | `mp:tag:*` | 公众号标签 CRUD（同步调用微信标签 API）与从微信同步标签 |
| **消息与自动回复** | `GET / POST` | `/admin-api/mp/message/{page,send}`, `/admin-api/mp/auto-reply/{create,update,delete,get,page}` | `mp:message:*`, `mp:auto-reply:*` | 查看粉丝互動消息历史、人工客服主动发送消息、配置关注/消息/关键词自动回复规则 |
| **自定义菜单** | `GET / POST / DELETE` | `/admin-api/mp/menu/{list,save,delete}` | `mp:menu:*` | 获取菜单树、保存并发布自定义菜单至微信、清空公众号菜单 |
| **素材与草稿发布** | `POST/PUT/DELETE/GET` | `/admin-api/mp/material/*`, `/admin-api/mp/draft/*`, `/admin-api/mp/free-publish/*` | `mp:material:*`, `mp:draft:*`, `mp:free-publish:*` | 上传永久与临时素材、管理图文草稿箱、将草稿正式群发发表 |
| **数据统计** | `GET` | `/admin-api/mp/statistics/{user-summary,user-cumulate,upstream-message,interface-summary}` | `mp:statistics:query` | 查询指定时间区间的粉丝增减、累计粉丝、消息发送量与接口耗时分析 |

## 六、核心业务逻辑 Go 源码实现（多公众号实例池与微信回调责任链）

```go
package open

import (
	"context"
	"sync"
	"time"
	"github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	"github.com/silenceper/wechat/v2/officialaccount"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/silenceper/wechat/v2/officialaccount/message"
	"oneweekonething.com/owot-pro/pkg/biz/tenant"
)

// MpClientFactory 维护 appId -> *officialaccount.OfficialAccount 的线程安全实例池
type MpClientFactory struct {
	mu         sync.RWMutex
	wc         *wechat.Wechat
	redisCache cache.Cache
	clients    map[string]*officialaccount.OfficialAccount
}

func (f *MpClientFactory) GetOrCreateClient(acc *MpAccountDO) *officialaccount.OfficialAccount {
	f.mu.RLock()
	cli, ok := f.clients[acc.AppID]
	f.mu.RUnlock()
	if ok {
		return cli
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cfg := &offConfig.Config{
		AppID:          acc.AppID,
		AppSecret:      acc.AppSecret,
		Token:          acc.Token,
		EncodingAESKey: acc.AesKey,
		Cache:          f.redisCache, // 桥接 Redis 实现多实例共享微信 AccessToken
	}
	cli = f.wc.GetOfficialAccount(cfg)
	f.clients[acc.AppID] = cli
	return cli
}

// HandleWechatCallback 处理 /admin-api/mp/open/:appId 的微信被动消息与事件回调
func (s *MpOpenService) HandleWechatCallback(ctx context.Context, appID string, msg *message.MixMessage) *message.Reply {
	// 1. 忽略租户隔离查询 mp_account 获取该公众号所属的 TenantID，随后切换 Context 租户身份
	acc, err := s.accountService.GetAccountByAppIDIgnoreTenant(ctx, appID)
	if err != nil || acc == nil {
		return nil
	}
	tenantCtx := tenant.WithTenantID(ctx, acc.TenantID)

	// 2. 微信回调 5 秒重试幂等去重校验（针对带 MsgID 的普通消息）
	if msg.MsgID > 0 {
		if !s.dedupRedisDAO.TryAcquire(tenantCtx, msg.MsgID, 60*time.Second) {
			return nil
		}
	}

	// 3. 责任链分发事件与消息
	switch msg.MsgType {
	case message.MsgTypeEvent:
		switch msg.Event {
		case message.EventSubscribe:
			return s.handleSubscribeEvent(tenantCtx, acc, msg)
		case message.EventUnsubscribe:
			s.handleUnsubscribeEvent(tenantCtx, acc, string(msg.FromUserName))
			return nil
		case message.EventClick:
			return s.handleMenuClickEvent(tenantCtx, acc, msg)
		}
	default:
		return s.handleNormalMessage(tenantCtx, acc, msg)
	}
	return nil
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **微信回调免租户 Header 解析检查**：微信服务器发起的 `/admin-api/mp/open/{appId}` 回调请求不会携带 `tenant-id` 请求头，必须先使用 `tenant.WithIgnoreTenant(ctx)` 根据路径中的 `appId` 查出 `mp_account.tenant_id`，再调用 `tenant.WithTenantID(ctx, acc.TenantID)` 执行后续落库。
- [x] **微信 5 秒超时与重试去重检查**：根据 `MsgId` 在 Redis 中执行 `SETNX` 去重，耗时下载转存 S3 操作放入后台 Goroutine 异步完成，确保主请求在 500ms 内返回 XML 响应。
