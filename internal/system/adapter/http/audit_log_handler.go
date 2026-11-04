package http

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/system/app"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

func (h *Handler) auditAction(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, err := pageQuery(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		if action == "export" {
			query.PageSize = -1
		}
		from, to, err := dateRange(c)
		if err != nil {
			h.fail(c, err)
			return
		}
		if resource == "operate-log" {
			filter := app.OperateLogFilter{PageQuery: query, Type: c.Query("type"), SubType: c.Query("subType"),
				UserNickname: c.Query("userNickname"), Action: c.Query("action"), From: from, To: to}
			filter.UserID, err = optionalInt64(c, "userId")
			if err == nil {
				filter.BizID, err = optionalInt64(c, "bizId")
			}
			if err != nil {
				h.fail(c, err)
				return
			}
			result, err := h.audit.PageOperateLogs(c.Request.Context(), filter)
			if err != nil {
				h.fail(c, err)
				return
			}
			if action == "export" {
				rows := make([][]any, 0, len(result.List))
				for _, v := range result.List {
					rows = append(rows, []any{v.ID, v.UserID, v.UserNickname, v.Type, v.SubType, v.BizID, v.Action, v.TraceID, v.CreateTime.UnixMilli()})
				}
				h.export(c, "operate-logs.xlsx", []string{"日志编号", "用户编号", "用户昵称", "操作模块", "操作名", "业务编号", "操作内容", "链路编号", "创建时间"}, rows)
				return
			}
			httpx.Success(c, pagination.PageResult[operateLogDTO]{List: convert(result.List, toOperateDTO), Total: result.Total})
			return
		}
		filter := app.LoginLogFilter{PageQuery: query, Username: c.Query("username"), UserIP: c.Query("userIp"), From: from, To: to}
		if raw := c.Query("status"); raw != "" {
			status, err := strconv.ParseBool(raw)
			if err != nil {
				h.fail(c, domain.ErrInvalidArgument)
				return
			}
			filter.Status = &status
		}
		result, err := h.audit.PageLoginLogs(c.Request.Context(), filter)
		if err != nil {
			h.fail(c, err)
			return
		}
		if action == "export" {
			rows := make([][]any, 0, len(result.List))
			for _, v := range result.List {
				rows = append(rows, []any{v.ID, v.LogType, v.UserID, v.Username, v.Result, v.UserIP, v.TraceID, v.CreateTime.UnixMilli()})
			}
			h.export(c, "login-logs.xlsx", []string{"日志编号", "日志类型", "用户编号", "用户账号", "登录结果", "用户IP", "链路编号", "创建时间"}, rows)
			return
		}
		httpx.Success(c, pagination.PageResult[loginLogDTO]{List: convert(result.List, toLoginDTO), Total: result.Total})
	}
}
