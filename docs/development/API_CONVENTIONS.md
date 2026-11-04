# HTTP API 契约规范（兼容 `yudao-ui-admin-vue3`）

## 一、统一响应包裹
所有接口 HTTP 状态码固定返回 `200 OK`，业务状态通过 JSON Body 表达：
```json
{
  "code": 0,
  "data": {},
  "msg": ""
}
```
- `code == 0` 表示成功；`code != 0` 表示业务或系统异常（如 `401` 未登录、`403` 无权限、`1002003000` 用户不存在）。

## 二、分页与列表契约
- **请求参数**：`pageNo`（默认 `1`，`>=1`）、`pageSize`（默认 `10`，`1~200`；传入 `-1` 表示不分页查全部）。
- **响应结构**：
  ```json
  {
    "code": 0,
    "data": {
      "list": [],
      "total": 0
    },
    "msg": ""
  }
  ```
- **强制要求**：当查询结果为空时，`list` 必须序列化为 `[]`，**严禁输出 `"list": null`**。

## 三、时间与数组参数规范
1. **响应时间**：所有 `createTime`、`updateTime`、`loginDate`、`expiresTime` 等时间字段在 JSON 响应中统一序列化为 **Int64 毫秒时间戳**（如 `1730736000000`）。
2. **请求时间区间**：支持前端 Query 传入 `createTime[0]=2026-01-01 00:00:00&createTime[1]=2026-01-31 23:59:59` 或毫秒时间戳数组。
3. **批量删除与 ID 列表**：兼容 `?ids=1,2,3` 与 `?ids=1&ids=2` 两种传参格式。

## 四、请求头与状态枚举约定
- **认证头**：`Authorization: Bearer <accessToken>`（同时兼容 Query `?token=<accessToken>`）。
- **租户头**：`tenant-id: <tenantId>`；超级管理员切换租户头：`visit-tenant-id: <tenantId>`。
- **链路追踪响应头**：`trace-id: <32位Hex>`。
- **通用状态（`CommonStatus`）**：`0` 开启（ENABLE），`1` 关闭（DISABLE）。
- **用户类型（`UserType`）**：`1` 管理后台用户（`/admin-api/**`），`2` C 端会员用户（`/app-api/**`）。
