package http

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
	"oneweekonething.com/owot-pro/internal/platform/excelx"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

type OperationsHandler struct {
	jobs   *app.JobService
	logs   *app.APILogService
	redis  *app.RedisMonitorService
	logger *slog.Logger
}

func NewOperationsHandler(jobs *app.JobService, logs *app.APILogService, redis *app.RedisMonitorService, logger *slog.Logger) *OperationsHandler {
	return &OperationsHandler{jobs: jobs, logs: logs, redis: redis, logger: logger}
}

func (h *OperationsHandler) Register(router *httpx.Router) {
	add := func(method, resource, action, permission string, global bool, handler gin.HandlerFunc) {
		route := httpx.Route{Permission: "infra:" + permission}
		if method != http.MethodGet {
			route.AuditType, route.AuditSubType = "infra-"+resource, action
		}
		router.Handle(method, "/admin-api/infra/"+resource+"/"+action, route, func(c *gin.Context) {
			user, _ := ctxmeta.User(c.Request.Context())
			if global && user.TenantID != 1 {
				httpx.Failure(c, 403, "无权访问全局基础设施")
				return
			}
			c.Header("Cache-Control", "no-store")
			handler(c)
		})
	}

	add(http.MethodPost, "job", "create", "job:create", true, h.saveJob(true))
	add(http.MethodPut, "job", "update", "job:update", true, h.saveJob(false))
	add(http.MethodPut, "job", "update-status", "job:update", true, h.setJobStatus)
	add(http.MethodPut, "job", "trigger", "job:trigger", true, h.triggerJob)
	add(http.MethodPost, "job", "sync", "job:create", true, h.syncJobs)
	add(http.MethodDelete, "job", "delete", "job:delete", true, h.deleteJobs(false))
	add(http.MethodDelete, "job", "delete-list", "job:delete", true, h.deleteJobs(true))
	add(http.MethodGet, "job", "get", "job:query", true, h.getJob)
	add(http.MethodGet, "job", "page", "job:query", true, h.jobPage(false))
	add(http.MethodGet, "job", "export-excel", "job:export", true, h.jobPage(true))
	add(http.MethodGet, "job", "get_next_times", "job:query", true, h.jobNextTimes)
	add(http.MethodGet, "job-log", "get", "job:query", true, h.getJobLog)
	add(http.MethodGet, "job-log", "page", "job:query", true, h.jobLogPage(false))
	add(http.MethodGet, "job-log", "export-excel", "job:export", true, h.jobLogPage(true))

	add(http.MethodGet, "api-access-log", "get", "api-access-log:query", false, h.getAccessLog)
	add(http.MethodGet, "api-access-log", "page", "api-access-log:query", false, h.accessLogPage(false))
	add(http.MethodGet, "api-access-log", "export-excel", "api-access-log:export", false, h.accessLogPage(true))
	add(http.MethodGet, "api-error-log", "get", "api-error-log:query", false, h.getErrorLog)
	add(http.MethodGet, "api-error-log", "page", "api-error-log:query", false, h.errorLogPage(false))
	add(http.MethodGet, "api-error-log", "export-excel", "api-error-log:export", false, h.errorLogPage(true))
	add(http.MethodPut, "api-error-log", "update-status", "api-error-log:update-status", false, h.processErrorLog)
	add(http.MethodGet, "redis", "get-monitor-info", "redis:get-monitor-info", true, h.redisInfo)
}

func (h *OperationsHandler) redisInfo(c *gin.Context) {
	info, err := h.redis.Monitor(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, info)
}

func (h *OperationsHandler) export(c *gin.Context, filename string, headers []string, rows [][]any) {
	var output bytes.Buffer
	if err := excelx.Write(c.Request.Context(), &output, headers, rows); err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", output.Bytes())
}

func pageResponse[T, DTO any](result pagination.PageResult[T], convert func(T) DTO) pagination.PageResult[DTO] {
	list := make([]DTO, 0, len(result.List))
	for _, entry := range result.List {
		list = append(list, convert(entry))
	}
	return pagination.PageResult[DTO]{List: list, Total: result.Total}
}

func queryTimeRange(c *gin.Context, name string) (*time.Time, *time.Time, error) {
	values := c.QueryArray(name)
	if len(values) == 0 {
		values = c.QueryArray(name + "[]")
	}
	parse := func(index int, key string) (*time.Time, error) {
		raw := c.Query(name + key)
		if raw == "" && index < len(values) {
			raw = values[index]
		}
		if raw == "" {
			return nil, nil
		}
		value, err := timeutil.Parse(raw)
		if err != nil {
			return nil, domain.ErrInvalidArgument
		}
		return &value, nil
	}
	from, err := parse(0, "[0]")
	if err != nil {
		return nil, nil, err
	}
	to, err := parse(1, "[1]")
	if err != nil || len(values) > 2 || from != nil && to != nil && from.After(*to) {
		return nil, nil, domain.ErrInvalidArgument
	}
	return from, to, nil
}

func optionalMillis(value *time.Time) timeutil.MillisTime {
	if value == nil {
		return timeutil.MillisTime{}
	}
	return timeutil.MillisTime{Time: *value}
}
