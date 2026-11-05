package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

type requestLogDTO struct {
	TraceID         string `json:"traceId"`
	ApplicationName string `json:"applicationName"`
	RequestMethod   string `json:"requestMethod"`
	RequestURL      string `json:"requestUrl"`
	RequestParams   string `json:"requestParams"`
	UserIP          string `json:"userIp"`
	UserAgent       string `json:"userAgent"`
	UserID          int64  `json:"userId"`
	UserType        int    `json:"userType"`
	ResultCode      int    `json:"resultCode"`
}

type accessLogDTO struct {
	requestLogDTO
	ID            int64               `json:"id"`
	ResponseBody  string              `json:"responseBody"`
	ResultMsg     string              `json:"resultMsg"`
	OperateModule string              `json:"operateModule"`
	OperateName   string              `json:"operateName"`
	OperateType   int                 `json:"operateType"`
	BeginTime     timeutil.MillisTime `json:"beginTime"`
	EndTime       timeutil.MillisTime `json:"endTime"`
	CreateTime    timeutil.MillisTime `json:"createTime"`
	Duration      int64               `json:"duration"`
}

func accessLogResponse(log domain.APIAccessLog) accessLogDTO {
	return accessLogDTO{requestLogDTO: requestLogDTO(log.RequestLog), ID: log.ID, ResponseBody: log.ResponseBody,
		ResultMsg: log.ResultMsg, OperateModule: log.OperateModule, OperateName: log.OperateName, OperateType: log.OperateType,
		BeginTime: timeutil.MillisTime{Time: log.BeginTime}, EndTime: timeutil.MillisTime{Time: log.EndTime},
		CreateTime: timeutil.MillisTime{Time: log.CreateTime}, Duration: log.Duration}
}

type errorLogDTO struct {
	requestLogDTO
	ID                        int64               `json:"id"`
	ExceptionTime             timeutil.MillisTime `json:"exceptionTime"`
	ExceptionName             string              `json:"exceptionName"`
	ExceptionMessage          string              `json:"exceptionMessage"`
	ExceptionRootCauseMessage string              `json:"exceptionRootCauseMessage"`
	ExceptionStackTrace       string              `json:"exceptionStackTrace"`
	ExceptionClassName        string              `json:"exceptionClassName"`
	ExceptionFileName         string              `json:"exceptionFileName"`
	ExceptionMethodName       string              `json:"exceptionMethodName"`
	ExceptionLineNumber       int                 `json:"exceptionLineNumber"`
	ProcessTime               timeutil.MillisTime `json:"processTime"`
	ProcessStatus             int                 `json:"processStatus"`
	ProcessUserID             int64               `json:"processUserId"`
	CreateTime                timeutil.MillisTime `json:"createTime"`
}

func errorLogResponse(log domain.APIErrorLog) errorLogDTO {
	return errorLogDTO{requestLogDTO: requestLogDTO(log.RequestLog), ID: log.ID, ExceptionTime: timeutil.MillisTime{Time: log.ExceptionTime},
		ExceptionName: log.ExceptionName, ExceptionMessage: log.ExceptionMessage, ExceptionRootCauseMessage: log.ExceptionRootCauseMessage,
		ExceptionStackTrace: log.ExceptionStackTrace, ExceptionClassName: log.ExceptionClassName, ExceptionFileName: log.ExceptionFileName,
		ExceptionMethodName: log.ExceptionMethodName, ExceptionLineNumber: log.ExceptionLineNumber,
		ProcessTime: optionalMillis(log.ProcessTime), ProcessStatus: log.ProcessStatus, ProcessUserID: log.ProcessUserID,
		CreateTime: timeutil.MillisTime{Time: log.CreateTime}}
}

func (h *OperationsHandler) getAccessLog(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	entry, err := h.logs.Access(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, accessLogResponse(entry))
}

func (h *OperationsHandler) getErrorLog(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	entry, err := h.logs.Error(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, errorLogResponse(entry))
}

func (h *OperationsHandler) processErrorLog(c *gin.Context) {
	var input struct {
		ID     int64 `form:"id" json:"id"`
		Status int   `form:"processStatus" json:"processStatus"`
	}
	err := c.ShouldBindQuery(&input)
	if err == nil && input.ID == 0 && c.ContentType() == "application/json" {
		err = c.ShouldBindJSON(&input)
	}
	if err != nil {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	if err := h.logs.ProcessError(c.Request.Context(), input.ID, input.Status); err != nil {
		h.fail(c, err)
		return
	}
	httpx.SetAuditBizID(c, input.ID)
	httpx.Success(c, true)
}

type logQuery struct {
	pagination.PageQuery
	UserID          *int64 `form:"userId"`
	UserType        *int   `form:"userType"`
	ApplicationName string `form:"applicationName"`
	RequestURL      string `form:"requestUrl"`
}

func (q logQuery) filter(from, to *time.Time, export bool) app.RequestLogFilter {
	return app.RequestLogFilter{PageQuery: q.PageQuery, Export: export, UserID: q.UserID, UserType: q.UserType,
		ApplicationName: q.ApplicationName, RequestURL: q.RequestURL, From: from, To: to}
}

func (h *OperationsHandler) accessLogPage(export bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			logQuery
			Duration   *int64 `form:"duration"`
			ResultCode *int   `form:"resultCode"`
		}
		if c.ShouldBindQuery(&input) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		from, to, err := queryTimeRange(c, "beginTime")
		if err != nil {
			h.fail(c, err)
			return
		}
		page, err := h.logs.AccessPage(c.Request.Context(), app.AccessLogFilter{RequestLogFilter: input.filter(from, to, export),
			Duration: input.Duration, ResultCode: input.ResultCode})
		if err != nil {
			h.fail(c, err)
			return
		}
		if !export {
			httpx.Success(c, pageResponse(page, accessLogResponse))
			return
		}
		rows := make([][]any, 0, len(page.List))
		for _, log := range page.List {
			rows = append(rows, []any{log.ID, log.TraceID, log.UserID, log.UserType, log.ApplicationName, log.RequestMethod, log.RequestURL,
				log.RequestParams, log.ResponseBody, log.UserIP, log.BeginTime.UnixMilli(), log.Duration, log.ResultCode, log.ResultMsg})
		}
		h.export(c, "api-access-logs.xlsx", []string{"日志编号", "链路编号", "用户编号", "用户类型", "应用名", "方法", "地址",
			"请求参数", "响应摘要", "IP", "请求时间", "耗时", "结果码", "结果信息"}, rows)
	}
}

func (h *OperationsHandler) errorLogPage(export bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			logQuery
			ProcessStatus *int `form:"processStatus"`
		}
		if c.ShouldBindQuery(&input) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		from, to, err := queryTimeRange(c, "exceptionTime")
		if err != nil {
			h.fail(c, err)
			return
		}
		page, err := h.logs.ErrorPage(c.Request.Context(), app.ErrorLogFilter{RequestLogFilter: input.filter(from, to, export),
			ProcessStatus: input.ProcessStatus})
		if err != nil {
			h.fail(c, err)
			return
		}
		if !export {
			httpx.Success(c, pageResponse(page, errorLogResponse))
			return
		}
		rows := make([][]any, 0, len(page.List))
		for _, log := range page.List {
			var handledAt any
			if log.ProcessTime != nil {
				handledAt = log.ProcessTime.UnixMilli()
			}
			rows = append(rows, []any{log.ID, log.TraceID, log.UserID, log.UserType, log.ApplicationName, log.RequestMethod, log.RequestURL,
				log.RequestParams, log.ExceptionTime.UnixMilli(), log.ExceptionName, log.ExceptionMessage, log.ExceptionStackTrace,
				log.ProcessStatus, log.ProcessUserID, handledAt})
		}
		h.export(c, "api-error-logs.xlsx", []string{"日志编号", "链路编号", "用户编号", "用户类型", "应用名", "方法", "地址",
			"请求参数", "异常时间", "异常类型", "异常信息", "堆栈", "处理状态", "处理人", "处理时间"}, rows)
	}
}
