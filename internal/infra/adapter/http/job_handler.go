package http

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/infra/app"
	"oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/httpx"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

// The frontend edits retry values in text inputs, so it sends quoted integers.
type jobNumber int

func (n *jobNumber) UnmarshalJSON(data []byte) error {
	if string(data) == `""` {
		*n = 0
		return nil
	}
	var raw json.Number
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == "" {
		*n = 0
		return nil
	}
	value, err := strconv.Atoi(string(raw))
	*n = jobNumber(value)
	return err
}

type jobInput struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	HandlerName    string    `json:"handlerName"`
	HandlerParam   string    `json:"handlerParam"`
	CronExpression string    `json:"cronExpression"`
	RetryCount     jobNumber `json:"retryCount"`
	RetryInterval  jobNumber `json:"retryInterval"`
	MonitorTimeout jobNumber `json:"monitorTimeout"`
}

type jobDTO struct {
	jobInput
	Status     int                 `json:"status"`
	CreateTime timeutil.MillisTime `json:"createTime"`
}

func jobResponse(job domain.Job) jobDTO {
	return jobDTO{jobInput: jobInput{ID: job.ID, Name: job.Name, HandlerName: job.HandlerName, HandlerParam: job.HandlerParam,
		CronExpression: job.CronExpression, RetryCount: jobNumber(job.RetryCount), RetryInterval: jobNumber(job.RetryInterval), MonitorTimeout: jobNumber(job.MonitorTimeout)},
		Status: job.Status, CreateTime: timeutil.MillisTime{Time: job.CreateTime}}
}

type jobLogDTO struct {
	ID             int64               `json:"id"`
	JobID          int64               `json:"jobId"`
	HandlerName    string              `json:"handlerName"`
	HandlerParam   string              `json:"handlerParam"`
	CronExpression string              `json:"cronExpression"`
	ExecuteIndex   int                 `json:"executeIndex"`
	BeginTime      timeutil.MillisTime `json:"beginTime"`
	EndTime        timeutil.MillisTime `json:"endTime"`
	CreateTime     timeutil.MillisTime `json:"createTime"`
	Duration       int64               `json:"duration"`
	Status         int                 `json:"status"`
	Result         string              `json:"result"`
}

func jobLogResponse(entry domain.JobLog) jobLogDTO {
	return jobLogDTO{ID: entry.ID, JobID: entry.JobID, HandlerName: entry.HandlerName, HandlerParam: entry.HandlerParam,
		CronExpression: entry.CronExpression, ExecuteIndex: entry.ExecuteIndex, BeginTime: timeutil.MillisTime{Time: entry.BeginTime},
		EndTime: optionalMillis(entry.EndTime), CreateTime: timeutil.MillisTime{Time: entry.CreateTime}, Duration: entry.Duration,
		Status: entry.Status, Result: entry.Result}
}

func (h *OperationsHandler) saveJob(create bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input jobInput
		if c.ShouldBindJSON(&input) != nil || create && input.ID != 0 || !create && input.ID <= 0 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		id, err := h.jobs.Save(c.Request.Context(), domain.Job{ID: input.ID, Name: input.Name, HandlerName: input.HandlerName,
			HandlerParam: input.HandlerParam, CronExpression: input.CronExpression, RetryCount: int(input.RetryCount),
			RetryInterval: int(input.RetryInterval), MonitorTimeout: int(input.MonitorTimeout)})
		if err != nil {
			h.fail(c, err)
			return
		}
		httpx.SetAuditBizID(c, id)
		if create {
			httpx.Success(c, id)
			return
		}
		httpx.Success(c, true)
	}
}

func (h *OperationsHandler) setJobStatus(c *gin.Context) {
	var input struct {
		ID     int64 `form:"id" json:"id"`
		Status int   `form:"status" json:"status"`
	}
	err := c.ShouldBindQuery(&input)
	if err == nil && input.ID == 0 && c.ContentType() == "application/json" {
		err = c.ShouldBindJSON(&input)
	}
	if err != nil || input.ID <= 0 {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	if err := h.jobs.SetStatus(c.Request.Context(), input.ID, input.Status); err != nil {
		h.fail(c, err)
		return
	}
	httpx.SetAuditBizID(c, input.ID)
	httpx.Success(c, true)
}

func (h *OperationsHandler) triggerJob(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err == nil {
		err = h.jobs.Trigger(c.Request.Context(), id)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.SetAuditBizID(c, id)
	httpx.Success(c, true)
}

func (h *OperationsHandler) syncJobs(c *gin.Context) {
	if err := h.jobs.Sync(c.Request.Context()); err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, true)
}

func (h *OperationsHandler) deleteJobs(batch bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ids, err := requestIDs(c, batch)
		if err != nil || len(ids) == 0 || len(ids) > 200 {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		slices.Sort(ids)
		ids = slices.Compact(ids)
		if err := h.jobs.Delete(c.Request.Context(), ids); err != nil {
			h.fail(c, err)
			return
		}
		if len(ids) == 1 {
			httpx.SetAuditBizID(c, ids[0])
		}
		httpx.Success(c, true)
	}
}

func (h *OperationsHandler) getJob(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	job, err := h.jobs.Get(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, jobResponse(job))
}

func (h *OperationsHandler) jobPage(export bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			pagination.PageQuery
			Name        string `form:"name"`
			HandlerName string `form:"handlerName"`
			Status      *int   `form:"status"`
		}
		if c.ShouldBindQuery(&input) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		if export {
			input.PageSize = -1
		}
		page, err := h.jobs.Page(c.Request.Context(), app.JobFilter{PageQuery: input.PageQuery,
			Name: input.Name, HandlerName: input.HandlerName, Status: input.Status})
		if err != nil {
			h.fail(c, err)
			return
		}
		if !export {
			httpx.Success(c, pageResponse(page, jobResponse))
			return
		}
		rows := make([][]any, 0, len(page.List))
		for _, job := range page.List {
			rows = append(rows, []any{job.ID, job.Name, job.Status, job.HandlerName, job.HandlerParam,
				job.CronExpression, job.RetryCount, job.RetryInterval, job.MonitorTimeout, job.CreateTime.UnixMilli()})
		}
		h.export(c, "jobs.xlsx", []string{"任务编号", "任务名称", "任务状态", "处理器", "参数", "Cron 表达式", "重试次数", "重试间隔", "超时", "创建时间"}, rows)
	}
}

func (h *OperationsHandler) jobNextTimes(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	count, err := strconv.Atoi(c.DefaultQuery("count", "5"))
	if err != nil || count < 1 || count > 100 {
		h.fail(c, domain.ErrInvalidArgument)
		return
	}
	times, err := h.jobs.NextTimes(c.Request.Context(), id, count)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, times)
}

func (h *OperationsHandler) getJobLog(c *gin.Context) {
	id, err := configID(c.Query("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	entry, err := h.jobs.Log(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, jobLogResponse(entry))
}

func (h *OperationsHandler) jobLogPage(export bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			pagination.PageQuery
			JobID       int64  `form:"jobId"`
			HandlerName string `form:"handlerName"`
			Status      *int   `form:"status"`
		}
		if c.ShouldBindQuery(&input) != nil {
			h.fail(c, domain.ErrInvalidArgument)
			return
		}
		beginFrom, beginTo, err := queryTimeRange(c, "beginTime")
		if err != nil {
			h.fail(c, err)
			return
		}
		endFrom, endTo, err := queryTimeRange(c, "endTime")
		if err != nil {
			h.fail(c, err)
			return
		}
		if c.Query("endTime") != "" && endTo == nil {
			endTo, endFrom = endFrom, nil
		}
		page, err := h.jobs.LogPage(c.Request.Context(), app.JobLogFilter{PageQuery: input.PageQuery, Export: export, JobID: input.JobID,
			HandlerName: input.HandlerName, Status: input.Status, BeginFrom: beginFrom, BeginTo: beginTo, EndFrom: endFrom, EndTo: endTo})
		if err != nil {
			h.fail(c, err)
			return
		}
		if !export {
			httpx.Success(c, pageResponse(page, jobLogResponse))
			return
		}
		rows := make([][]any, 0, len(page.List))
		for _, entry := range page.List {
			var end any
			if entry.EndTime != nil {
				end = entry.EndTime.UnixMilli()
			}
			rows = append(rows, []any{entry.ID, entry.JobID, entry.HandlerName, entry.ExecuteIndex,
				entry.BeginTime.UnixMilli(), end, entry.Duration, entry.Status, entry.Result})
		}
		h.export(c, "job-logs.xlsx", []string{"日志编号", "任务编号", "处理器", "执行序号", "开始时间", "结束时间", "耗时", "状态", "结果"}, rows)
	}
}
