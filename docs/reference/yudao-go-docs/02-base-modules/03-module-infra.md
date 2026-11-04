# DOC-03：yudao-module-infra 基础设施模块（必选核心基础模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：全平台的研发效能引擎与基础设施运维中枢，提供 **Go + Vue3 全栈代码生成器**、**多云文件存储服务（S3/MinIO/OSS/COS/本地/FTP/SFTP/DB）**、**动态参数配置**、**在线分布式定时任务**、**RESTful API 访问日志与错误日志审计**、**多数据源管理与数据库文档导出**以及 **Redis/DB 监控面板**。
- **Go 模块路径**：`internal/modules/infra`（对外跨模块契约：`internal/api/infra`）
- **架构 Review 核心结论**：
  1. **代码生成器双端对齐**：通过解析数据库 `information_schema.tables` 与 `columns`，使用 Go `embed.FS` + `text/template` 一次性渲染产出 Go 后端代码（DO、GORM Gen Mapper、ReqVO/RespVO、Service、Gin Handler、单元测试）和原版 Vue3（Element Plus / Vben）前端代码与 SQL 菜单脚本，覆盖单表、树表与主子表三种模式。
  2. **多云文件客户端热切换**：`FileClientFactory` 维护 `configId -> FileClient` 内存映射，当管理员在后台将某条配置切换为 `master = true` 时，通过 MQ 广播通知所有实例无缝切换默认文件存储引擎。

## 二、Go 模块标准包目录结构

```text
internal/modules/infra/
├── controller/
│   ├── admin/                        # 管理后台 RESTful 接口 (/admin-api/infra/*)
│   │   ├── codegen/                  # 代码生成器：数据库表列表、导入、编辑配置、在线预览、下载 ZIP
│   │   ├── config/                   # 动态参数配置 CRUD 与 Excel 导出
│   │   ├── db/                       # 多数据源配置 CRUD、连接测试、导出 HTML/Word/MD 数据库文档
│   │   ├── file/                     # 文件存储配置 CRUD、设为主配置、文件上传、S3 预签名直传、文件列表
│   │   ├── job/                      # 定时任务 CRUD、暂停/恢复、立即触发一次、任务执行日志分页
│   │   ├── logger/                   # API 访问日志分页/导出、API 错误日志分页/标记已处理
│   │   └── redis/                    # Redis 监控面板（INFO 命令指标解析、命令耗时统计、Key 数量）
│   └── app/                          # 移动端接口 (/app-api/infra/*)
│       ├── config/                   # 移动端获取公开参数配置
│       └── file/                     # 移动端文件上传与获取 S3 预签名直传地址
├── service/
│   ├── codegen/                      # 表结构解析器 (SchemaReader) 与模板渲染引擎 (CodegenEngine)
│   ├── config/
│   ├── db/
│   ├── file/
│   ├── job/
│   └── logger/
├── dal/
│   ├── dataobject/                   # 12 张 infra_* 表对应的 GORM DO 结构体
│   └── mysql/                        # GORM Gen 类型安全 DAO
├── api/                              # 实现 FileApi, ConfigApi, ApiAccessLogApi, ApiErrorLogApi
├── mq/                               # 文件主配置变更与动态参数变更广播消费者
├── job/                              # 访问日志与错误日志定期归档清理 Job
├── enums/                            # 模块错误码段 (1_001_000_000 起) 与存储/模板枚举
├── framework/
│   ├── file/                         # 内聚文件客户端：S3, Local, FTP, SFTP, DB 五种实现与工厂
│   └── codegen/                      # embed.FS 内嵌的 Go 后端与 Vue3 前端代码模板库
└── module.go                         # 导出 infra.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `infra_`）

| 序号 | 表名 | 租户隔离 | 核心字段说明 | 核心索引与约束 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | `infra_codegen_table` | 全局表 | `id`, `data_source_config_id`, `scene`(1管理端/2移动端), `table_name`, `table_comment`, `class_name`, `class_comment`, `author`, `template_type`(1单表/2树表/10主子表标准/11主子表ERP/12主子表内嵌), `front_type`(10 Vue2/20 Vue3/21 Vue3 Schema/30 Vben), `module_name`, `business_name`, `parent_menu_id`, `master_table_id`, `sub_join_column_id`, `sub_join_many`, `tree_parent_column_id`, `tree_name_column_id` | `idx_table_name` (`table_name`) |
| 02 | `infra_codegen_column` | 全局表 | `id`, `table_id`, `column_name`, `data_type`, `column_comment`, `nullable`, `primary_key`, `auto_increment`, `ordinal_position`, `java_type`/`go_type`, `java_field`/`go_field`, `dict_type`, `example`, `create_operation`, `update_operation`, `list_operation`, `list_operation_condition`, `list_operation_result`, `html_type` | `idx_table_id` (`table_id`) |
| 03 | `infra_file_config` | 全局表 | `id`, `name`, `storage`(1数据库/10本地磁盘/11 FTP/12 SFTP/20 S3对象存储), `remark`, `master`(bool 全表唯一主配置), `config`(JSON：存储 endpoint/bucket/accessKey/domain 或 basePath) | `idx_master` (`master`) |
| 04 | `infra_file` | 全局表 | `id`, `config_id`, `name`(原始文件名), `path`(相对路径), `url`(完整访问地址), `type`(MIME类型), `size`(字节数) | `idx_create_time` (`create_time`) |
| 05 | `infra_file_content` | 全局表 | `id`, `config_id`, `path`, `content`(`LONGBLOB` 二进制内容，仅当 `storage = 1` 时使用) | `idx_config_path` (`config_id`, `path`) |
| 06 | `infra_config` | 全局表 | `id`, `category`, `type`(1系统内置/2自定义), `name`, `config_key`, `value`, `visible`(前端是否可读), `remark` | `uk_config_key` (`config_key`) |
| 07 | `infra_job` | 全局表 | `id`, `name`, `status`(1开启/2暂停), `handler_name`(对应 Go `JobHandler` 名称), `handler_param`, `cron_expression`, `retry_count`, `retry_interval`, `monitor_timeout` | `uk_handler_name` (`handler_name`, `handler_param`) |
| 08 | `infra_job_log` | 全局表 | `id`, `job_id`, `handler_name`, `handler_param`, `execute_index`, `begin_time`, `end_time`, `duration`(ms), `status`(0运行中/1成功/2失败), `result` | `idx_job_id` (`job_id`), `idx_begin_time` |
| 09 | `infra_api_access_log` | **租户级 (`tenant_id`)** | `id`, `trace_id`, `user_id`, `user_type`, `application_name`, `request_method`, `request_url`, `request_params`, `response_body`, `user_ip`, `user_agent`, `operate_module`, `operate_name`, `operate_type`, `begin_time`, `end_time`, `duration`, `result_code`, `result_msg` | `idx_begin_time` (`begin_time`), `idx_user_id` |
| 10 | `infra_api_error_log` | **租户级 (`tenant_id`)** | `id`, `trace_id`, `user_id`, `user_type`, `application_name`, `request_method`, `request_url`, `request_params`, `user_ip`, `user_agent`, `exception_time`, `exception_name`, `exception_message`, `exception_root_cause_message`, `exception_stack_trace`, `process_status`(0未处理/1已处理/2已忽略), `process_time`, `process_user_id` | `idx_exception_time`, `idx_process_status` |
| 11 | `infra_data_source_config` | 全局表 | `id`, `name`, `url`(DSN 连接串), `username`, `password`(AES 加密存储) | 主键索引 `id` |
| 12 | `infra_demo01` ~ `03` | **租户级 (`tenant_id`)** | 代码生成器自带的单表（`demo01_contact`）、树表（`demo02_category`）、主子表（`demo03_student` + `course` + `grade`）示例表 | 主键索引 `id` |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **MQ 广播刷新** | `infra.file_config.refresh` | 当后台新增/修改文件配置或切换 `master` 主配置时广播，各实例清理并重建 `FileClientFactory` 中的客户端实例 |
| **MQ 广播刷新** | `infra.config.refresh` | 当后台修改 `infra_config` 动态参数时广播，各实例刷新进程内配置缓存 |
| **内置定时任务** | `accessLogCleanJob` / `errorLogCleanJob` / `jobLogCleanJob` | 每日凌晨定期清理超过保留天数（如 14 天）的 `infra_api_access_log`、`infra_api_error_log` 与 `infra_job_log`，防止日志表无限膨胀 |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **代码生成 (`codegen`)** | `GET` | `/admin-api/infra/codegen/db/table/list` | `infra:codegen:query` | 查询指定数据源下尚未导入代码生成器的物理表列表 |
| **代码生成 (`codegen`)** | `POST/PUT/DELETE` | `/admin-api/infra/codegen/{create-list,update,sync-from-db,delete}` | `infra:codegen:*` | 批量导入表结构、修改字段生成配置、同步最新数据库表结构变更 |
| **代码生成 (`codegen`)** | `GET` | `/admin-api/infra/codegen/{preview,download}` | `infra:codegen:{preview,download}` | 在线预览生成的 Go + Vue3 全套代码，或打包为 `.zip` 流式下载 |
| **文件服务 (`file`)** | `POST/PUT/DELETE/GET` | `/admin-api/infra/file-config/{create,update,update-master,delete,get,page,test}` | `infra:file-config:*` | 文件存储配置管理、设为默认主配置、测试上传连通性 |
| **文件服务 (`file`)** | `POST / GET` | `/admin-api/infra/file/{upload,presigned-url}` | 已登录即可 | 后端中转上传文件，或获取 S3 前端直传预签名 URL |
| **文件服务 (`file`)** | `GET` | `/admin-api/infra/file/{configId}/get/**` | `@PermitAll` | 本地磁盘/FTP/SFTP/DB 存储模式下的文件二进制流读取端点 |
| **定时任务 (`job`)** | `POST/PUT/DELETE/GET` | `/admin-api/infra/job/{create,update,update-status,trigger,delete,get,page}` | `infra:job:*` | 在线管理 Cron 定时任务、暂停/恢复、立即触发一次 |
| **日志与监控** | `GET / PUT` | `/admin-api/infra/api-access-log/page`, `/admin-api/infra/api-error-log/{page,update-status}` | `infra:api-*-log:*` | 查询 API 访问日志与异常堆栈日志，标记异常日志已处理 |
| **数据库文档** | `GET` | `/admin-api/infra/db-doc/export-{html,word,markdown}` | `infra:db-doc:export` | 实时扫描数据库 Schema 导出表结构设计文档 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package infra

import "context"

// FileApi 供全系统上传文件（头像、商品图、素材、附件）调用的契约
type FileApi interface {
	CreateFile(ctx context.Context, name string, path string, content []byte) (url string, err error)
}

// ConfigApi 供各模块按 Key 读取动态系统参数配置的契约
type ConfigApi interface {
	GetConfigValueByKey(ctx context.Context, key string) (string, error)
}

// ApiAccessLogApi & ApiErrorLogApi 供 pkg/web 中间件异步批量写入访问日志与异常日志
type ApiAccessLogApi interface {
	CreateApiAccessLog(ctx context.Context, req *ApiAccessLogCreateReqDTO) error
}

type ApiErrorLogApi interface {
	CreateApiErrorLog(ctx context.Context, req *ApiErrorLogCreateReqDTO) error
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **S3 预签名直传跨域与路径安全**：生成的对象存储 `path` 必须使用 `sha256(content) + ext` 或带日期前缀的安全文件名，严禁直接使用客户端传入的含 `../` 的路径，防止目录穿越漏洞。
- [x] **数据源密码加密存储**：`infra_data_source_config.password` 落库前必须经过 AES 对称加密，前端回显时脱敏或仅在连接测试时解密。
