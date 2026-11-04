# DOC-07：yudao-module-report 大屏与数据报表模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：企业级可视化数据大屏（GoView）与在线报表设计器基础模块（参考 [报表与大屏开发指南](https://doc.iocoder.cn/report/)）。
- **Go 模块路径**：`internal/modules/report`
- **架构 Review 核心结论**：
  1. **GoView 前端零改动无缝对接**：配套大屏前端项目 `yudao-ui-go-view` 通过 `/admin-api/report/go-view/**` 与后端交互，共用 `system` 模块的 Bearer Token 鉴权体系。
  2. **动态 SQL 与 HTTP 代理安全加固**：
     - 针对 `/admin-api/report/go-view/data/get-by-sql`：禁止多语句执行（`;`），禁止 DML/DDL 及系统表/敏感表（`system_users`、`system_oauth2_*`）查询，强制在 5 秒超时只读事务（`sql.TxOptions{ReadOnly: true}`）中运行，并由 GORM 多租户插件确保跨租户数据隔离。
     - 针对 `/admin-api/report/go-view/data/get-by-http`：对目标 URL 的解析 IP 进行 **SSRF（服务端请求伪造）内网地址段拦截**（禁止访问 `127.0.0.0/8`、`10.0.0.0/8`、`169.254.169.254` 云元数据地址等）。

## 二、Go 模块标准包目录结构

```text
internal/modules/report/
├── controller/
│   └── admin/                        # 管理后台与 GoView 设计器接口 (/admin-api/report/*)
│       ├── goview/
│       │   ├── go_view_project_handler.go  # 大屏项目 CRUD 与我的大屏分页
│       │   ├── go_view_data_handler.go     # SQL 动态数据集查询与 HTTP 代理数据集查询
│       │   └── go_view_vo.go               # GoView 专属 ReqVO / RespVO
│       └── jimu/
│           └── jimu_report_handler.go      # 报表设计器 Token 鉴权与自定义数据集端点
├── service/
│   └── goview/
│       ├── go_view_project_service.go
│       └── go_view_data_service.go         # SQL 安全沙箱与 SSRF 防护 HTTP 代理
├── dal/
│   ├── dataobject/
│   │   └── go_view_project_do.go           # report_go_view_project 表 DO
│   └── mysql/
│       └── go_view_project_mapper.go
├── enums/                            # 模块错误码段 (1_003_000_000 起)
├── convert/
└── module.go                         # 导出 report.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（前缀 `report_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `report_go_view_project` | **租户级 (`tenant_id`)** | `id`(主键), `name`(大屏项目名称), `status`(0已发布/1未发布), `content`(`LONGTEXT` 存储 GoView 画布全部图表组件与坐标配置 JSON), `pic_url`(大屏缩略预览图 URL), `remark`, `creator`(创建人用户ID) | `idx_creator_tenant` (`creator`, `tenant_id`) |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

- **大屏查询结果短缓存（`GoViewDataRedisDAO`）**：对于高频刷新的大屏 SQL 查询（如每 5 秒轮询一次的监控大屏），以 `report:goview:sql_cache:{tenantId}:{md5(sql+params)}` 为 Key 缓存结果 `3~5 秒`，避免大屏多终端同时打开时击穿数据库。

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **大屏项目管理** | `POST` | `/admin-api/report/go-view/project/create` | `report:go-view-project:create` | 新建大屏项目（初始状态为 `1-未发布`），返回项目 `id` |
| **大屏项目管理** | `PUT` | `/admin-api/report/go-view/project/update` | `report:go-view-project:update` | 更新大屏名称、发布状态（`status`）、预览图（`picUrl`）或画布 JSON（`content`） |
| **大屏项目管理** | `DELETE` | `/admin-api/report/go-view/project/delete` | `report:go-view-project:delete` | 删除指定 `id` 的大屏项目 |
| **大屏项目管理** | `GET` | `/admin-api/report/go-view/project/get` | `report:go-view-project:query` | 获取指定 `id` 的大屏项目完整详情（含 `content` 画布 JSON） |
| **大屏项目管理** | `GET` | `/admin-api/report/go-view/project/my-page` | `report:go-view-project:query` | 分页获取当前登录管理员创建的大屏列表 |
| **大屏动态数据** | `POST` | `/admin-api/report/go-view/data/get-by-sql` | `report:go-view-data:get-by-sql` | 在只读安全沙箱中执行 SELECT 查询，返回 `{header: string[], source: map[string]any[]}` |
| **大屏动态数据** | `POST` | `/admin-api/report/go-view/data/get-by-http` | `report:go-view-data:get-by-http` | 后端安全代理转发 HTTP 数据集请求并返回响应 JSON |

## 六、核心业务逻辑 Go 源码实现（SQL 沙箱与防 SSRF HTTP 代理）

```go
package goview

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
	"gorm.io/gorm"
)

var sqlBlacklistRegex = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|TRUNCATE|GRANT|REVOKE|INTO\s+OUTFILE|LOAD_FILE|INFORMATION_SCHEMA|PERFORMANCE_SCHEMA|MYSQL|SLEEP|BENCHMARK|SYSTEM_USERS|SYSTEM_OAUTH2)\b`)

type GoViewDataRespVO struct {
	Header []string                 `json:"header"`
	Source []map[string]interface{} `json:"source"`
}

// GetDataBySQL 安全只读执行大屏 SQL 查询
func (s *GoViewDataService) GetDataBySQL(ctx context.Context, rawSQL string) (*GoViewDataRespVO, error) {
	trimmed := strings.TrimSpace(rawSQL)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT ") || strings.Contains(trimmed, ";") {
		return nil, errors.New("大屏 SQL 仅支持单条 SELECT 语句")
	}
	if sqlBlacklistRegex.MatchString(trimmed) {
		return nil, errors.New("SQL 包含受限系统表或危险函数")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	source := make([]map[string]interface{}, 0)
	err := s.db.WithContext(timeoutCtx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(trimmed).Scan(&source).Error
	}, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	header := make([]string, 0)
	if len(source) > 0 {
		for col := range source[0] {
			header = append(header, col)
		}
	}
	return &GoViewDataRespVO{Header: header, Source: source}, nil
}

// ValidateExternalURL 校验 HTTP 数据集目标地址，阻断针对内网与云元数据的 SSRF 攻击
func ValidateExternalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("非法的 HTTP/HTTPS 地址")
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("无法解析目标域名")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("安全拦截：禁止大屏 HTTP 数据源访问内网私有 IP 地址")
		}
	}
	return nil
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **大屏未发布状态访问控制**：未发布（`status = 1`）的大屏仅允许创建人或拥有后台管理权限的人员预览；生产投屏查看时校验大屏必须处于已发布（`status = 0`）状态。
- [x] **SSRF 与慢 SQL 熔断测试**：单测覆盖传入 `SELECT SLEEP(10)`、`SELECT * FROM system_users` 以及 `http://127.0.0.1:8080` 时的拦截断言。
