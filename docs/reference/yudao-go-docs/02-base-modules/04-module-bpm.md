# DOC-04：yudao-module-bpm 工作流程模块（通用基础支撑模块）

## 一、模块概述与架构 Review 评估

- **模块定位**：企业级工作流与审批引擎模块（参考 [工作流演示与开发指南](https://doc.iocoder.cn/bpm-preview/)），支撑全系统所有的业务审批诉求（OA 请假/报销/公文、CRM 合同与回款审批、ERP 采购/销售审核、HRM 员工转正/调岗/离职审批）。
- **Go 模块路径**：`internal/modules/bpm`（对外跨模块契约：`internal/api/bpm`）
- **架构 Review 核心结论**：
  1. **双设计器统一 AST 模型**：原项目前端同时包含 **BPMN 标准设计器**（输出 XML）和 **SIMPLE 仿钉钉/飞书设计器**（输出树形 JSON）。在 Go 端通过 `parser_bpmn.go` 和 `parser_simple.go` 将两种格式统一编译为内存中的标准有向图结构 `ProcessGraph`（存入 `bpm_engine_deployment.compiled_graph_json`），使运行时引擎完全无需关心设计器来源。
  2. **中国式复杂审批动作原生支持**：通过 5 张轻量级 Go 引擎表（`deployment`、`instance`、`execution`、`task`、`activity_log`）直接内置支持**会签（按人数或比例）、或签、依次审批、向前加签、向后加签、减签、自由驳回至任意历史节点、转办、委派、抄送、父子流程、延迟节点、HTTP 触发器与超时自动处理**。

## 二、Go 模块标准包目录结构

```text
internal/modules/bpm/
├── controller/
│   ├── admin/                        # 管理后台与 PC 审批中心接口 (/admin-api/bpm/*)
│   │   ├── definition/               # 流程分类、动态表单、流程模型(BPMN/Simple)、流程定义、用户组、表达式、监听器
│   │   ├── task/                     # 流程实例(发起/取消/详情/预测节点)、待办/已办/抄送任务、通过/拒绝/驳回/加减签/转办/委派
│   │   └── oa/                       # 内置 OA 请假审批接入示例 (BpmOALeaveController)
│   └── app/                          # 移动端审批中心接口 (/app-api/bpm/*)
│       └── task/                     # 移动端我的待办、已办、发起流程与审批操作
├── service/
│   ├── definition/                   # 模型版本管理、动态表单校验、审批候选人解析器
│   ├── task/                         # 实例与任务状态流转、抄送记录、超时提醒与事件发布
│   └── oa/
├── dal/
│   ├── dataobject/                   # 7 张业务扩展表 + 5 张 Go DAG 审批引擎核心表 DO
│   ├── mysql/                        # GORM Gen 类型安全 DAO
│   └── redis/                        # ProcessIdRedisDAO (自定义流程编号原子自增序列)
├── api/                              # 实现 BpmProcessInstanceApi（供 OA/CRM/ERP 代码发起流程）
├── mq/                               # 流程实例状态变更事件发布 (BpmProcessInstanceStatusEvent)
├── job/                              # 任务超时自动审批/自动驳回/重复催办提醒定时任务 (TaskTimeoutJob)
├── enums/                            # 模块错误码段 (1_009_000_000 起) 与节点/任务状态枚举
├── framework/
│   └── flowengine/                   # Go 原生 DAG + Token 状态机引擎核心实现
│       ├── graph_model.go            # 统一有向图 AST 模型定义
│       ├── parser_bpmn.go            # BPMN 2.0 XML -> ProcessGraph 编译器
│       ├── parser_simple.go          # 仿钉钉/飞书 Simple JSON -> ProcessGraph 编译器
│       ├── runtime_engine.go         # 状态机执行器（Token 分裂、汇聚、跳转、回退、加签）
│       └── candidate_strategy.go     # 9 种审批人分配策略执行器
└── module.go                         # 导出 bpm.ProviderSet 与路由注册函数
```

## 三、全量数据库表结构清单（12 张表，前缀 `bpm_`）

| 序号 | 表分类 | 表名 | 租户隔离 | 核心字段与设计说明 |
| :--- | :--- | :--- | :--- | :--- |
| 01 | 流程定义扩展 | `bpm_category` | **租户级 (`tenant_id`)** | `id`, `name`, `code`, `description`, `status`, `sort`（流程分类，如人事、财务、行政） |
| 02 | 流程定义扩展 | `bpm_form` | **租户级 (`tenant_id`)** | `id`, `name`, `status`, `conf`(表单全局配置JSON), `fields`(form-create 拖拽组件 JSON 数组), `remark` |
| 03 | 流程定义扩展 | `bpm_process_definition_info` | **租户级 (`tenant_id`)** | `id`, `process_definition_id`, `model_id`, `model_type`(10 BPMN / 20 Simple), `category`, `icon`, `description`, `form_type`(10流程动态表单/20业务自定义表单), `form_id`, `form_conf`, `form_fields`, `form_custom_create_path`, `form_custom_view_path`, `simple_model`(Simple设计器JSON), `visible`, `start_user_ids`, `manager_user_ids`, `process_id_rule`, `auto_approval_type`, `title_setting`, `summary_setting` |
| 04 | 流程辅助配置 | `bpm_user_group` | **租户级 (`tenant_id`)** | `id`, `name`, `description`, `user_ids`(JSON数组), `status`（自定义审批人分组） |
| 05 | 流程辅助配置 | `bpm_process_expression` & `bpm_process_listener` | **租户级 (`tenant_id`)** | 常用条件表达式库（`name`, `status`, `expression`）与实例/任务级监听器配置（`name`, `type`, `status`, `event`, `value_type`, `value`） |
| 06 | 业务流转记录 | `bpm_process_instance_copy` | **租户级 (`tenant_id`)** | `id`, `user_id`(被抄送人), `start_user_id`, `process_instance_id`, `process_instance_name`, `category`, `activity_id`, `activity_name`, `task_id`, `reason` |
| 07 | 接入示例表 | `bpm_oa_leave` | **租户级 (`tenant_id`)** | `id`, `user_id`, `type`, `reason`, `start_time`, `end_time`, `day`, `status`, `process_instance_id` |
| 08 | **Go 引擎核心表** | `bpm_engine_deployment` | **租户级 (`tenant_id`)** | `id`(string), `model_id`, `process_key`, `name`, `category`, `version`(递增版本号), `model_type`, `bpmn_xml`, `simple_json`, `compiled_graph_json`(编译后的标准 DAG 节点图), `suspension_state`(1激活/2挂起) |
| 09 | **Go 引擎核心表** | `bpm_engine_instance` | **租户级 (`tenant_id`)** | `id`(自定义编号或UUID), `deployment_id`, `process_key`, `name`, `category`, `business_key`(业务单据ID), `start_user_id`, `status`(1审批中/2通过/3不通过/4已取消), `form_variables`(JSON), `parent_instance_id`, `start_time`, `end_time`, `duration` |
| 10 | **Go 引擎核心表** | `bpm_engine_execution` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `parent_id`, `current_node_id`, `active`(bool), `scope_variables`(JSON)（用于并行网关多分支令牌跟踪与汇聚计数） |
| 11 | **Go 引擎核心表** | `bpm_engine_task` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `execution_id`, `node_id`, `node_name`, `assignee_user_id`(处理人), `owner_user_id`(委派原持有人), `status`(1待办/2通过/3不通过/4取消/5退回/6委派中/7待加签完成), `reason`, `sign_pic_url`, `parent_task_id`, `approve_method`(1依次/2会签/3或签), `due_time`(超时时间), `timeout_handler_type`, `end_time` |
| 12 | **Go 引擎核心表** | `bpm_engine_activity_log` | **租户级 (`tenant_id`)** | `id`, `instance_id`, `execution_id`, `node_id`, `node_name`, `node_type`, `task_id`, `assignee_id`, `status`, `start_time`, `end_time`（供前端流程图高亮轨迹） |

## 四、RedisDAO、MQ 消息队列与 Job 定时任务设计

| 组件类别 | 标识名称 / Topic / Key 模板 | 触发时机与业务处理逻辑 |
| :--- | :--- | :--- |
| **RedisDAO** | `bpm:process_id_seq:%d:%s` | 当流程配置开启 `process_id_rule`（自定义流程编号前缀+日期+自增序列）时，使用 Redis `INCR` 原子生成唯一流程实例 ID |
| **MQ 事件广播** | `bpm.process_instance.status` | 当流程实例最终状态变为“审批通过(2)”、“审批不通过(3)”或“已取消(4)”时发布事件，供 OA/CRM/ERP 业务模块监听并更新业务单据状态 |
| **内置定时任务** | `bpmTaskTimeoutJob` | 每分钟扫描一次 `status = 1 AND due_time <= NOW()` 的待办任务，按节点配置的超时策略自动执行“自动提醒（发送站内信/短信）”、“自动通过”或“自动驳回” |

## 五、全量 RESTful API 路由契约表

| 子域 | HTTP 方法 | 路由路径 | 权限标识 | 功能描述 |
| :--- | :--- | :--- | :--- | :--- |
| **模型与定义** | `POST/PUT/DELETE/GET` | `/admin-api/bpm/model/{create,update,update-bpmn,delete,get,page,deploy,update-state}` | `bpm:model:*` | 流程模型 CRUD、保存 BPMN XML、发布部署新版本、激活或挂起流程定义 |
| **Simple 设计器** | `GET / POST` | `/admin-api/bpm/model/simple/{get,update}` | `bpm:model:*` | 读取与保存仿钉钉/飞书 Simple 设计器的树形 JSON 节点配置 |
| **流程定义查询** | `GET` | `/admin-api/bpm/process-definition/{list,page,get}` | `bpm:process-definition:query` | 发起流程页获取当前用户可见的激活流程定义列表及历史部署版本分页 |
| **流程实例** | `POST / DELETE` | `/admin-api/bpm/process-instance/{create,cancel-by-start-user,cancel-by-admin}` | `bpm:process-instance:*` | 发起流程实例、发起人主动撤回、管理员强制终止流程 |
| **流程实例** | `GET` | `/admin-api/bpm/process-instance/{my-page,manager-page,get-approval-detail}` | `bpm:process-instance:query` | 我的发起列表、管理后台全量实例列表、获取审批详情页完整数据（含预测节点时间线与流程图高亮） |
| **审批任务** | `GET` | `/admin-api/bpm/task/{todo-page,done-page,manager-page}` | `bpm:task:query` | 我的待办、我的已办、管理员任务监管列表 |
| **审批操作** | `PUT` | `/admin-api/bpm/task/{approve,reject,return,delegate,transfer,create-sign,delete-sign}` | `bpm:task:update` | 审批通过、审批拒绝、自由退回（配合 `get-return-list`）、委派、转办、向前/向后加签、减签 |
| **抄送列表** | `GET` | `/admin-api/bpm/process-instance/copy/page` | `bpm:process-instance-copy:query` | 分页查询抄送给我的流程记录 |

## 六、跨模块 API 契约与核心 Go 源码实现

```go
package bpm

import "context"

// BpmProcessInstanceApi 供 OA / CRM / ERP / HRM 业务模块在代码中直接发起审批流的契约
type BpmProcessInstanceApi interface {
	CreateProcessInstance(ctx context.Context, userID int64, req *BpmProcessInstanceCreateReqDTO) (instanceID string, err error)
}

type BpmProcessInstanceCreateReqDTO struct {
	ProcessDefinitionKey     string                 // 流程定义标识（如 "oa_leave", "crm_contract"）
	BusinessKey              string                 // 业务表主键 ID（如合同 ID）
	Variables                map[string]interface{} // 业务表单变量（用于条件分支判断与摘要展示）
	StartUserSelectAssignees map[string][]int64     // 发起人自选审批人映射：nodeId -> userIds
}

// BpmProcessInstanceStatusEvent 流程实例终态事件载荷（各业务模块通过 MQ 订阅或注册 Listener 接收）
type BpmProcessInstanceStatusEvent struct {
	InstanceID           string `json:"id"`
	ProcessDefinitionKey string `json:"processDefinitionKey"`
	BusinessKey          string `json:"businessKey"` // 对应业务单据 ID
	Status               int    `json:"status"`      // 2-审批通过 3-审批不通过 4-已取消
}
```

## 七、模块 Review 验收与单测 Checklist

- [x] **审批人自动去重（AutoApprovalType）**：当流程开启“发起人自动通过”或“相邻节点/全局同一审批人自动去重通过”时，`createUserTasks` 检测到候选人与发起人或历史已通过人一致，应直接将该任务标记为自动通过并递归推进下一节点。
- [x] **向前加签 vs 向后加签状态机**：向前加签（`before`）时，原任务状态置为 `7（待前加签完成）`，待所有前加签子任务审批通过后，原任务重新恢复为 `1（待办）`；向后加签（`after`）时，原任务直接标记为 `2（通过）`，待所有后加签子任务通过后直接推进至流程图下一节点。
- [x] **自由退回（Return）分支清理**：当在并行分支之后执行自由退回至汇聚前的节点时，必须清理或取消当前活跃的所有兄弟分支待办任务与 `execution` 令牌，防止残留脏待办。
