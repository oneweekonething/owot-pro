package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	infradomain "oneweekonething.com/owot-pro/internal/infra/domain"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

func TestInfraOperationsHTTP(t *testing.T) {
	f := newIAMFixture(t)
	ctx, stop := context.WithCancel(t.Context())
	var workers sync.WaitGroup
	workers.Go(func() { f.operations.requestLogs.Run(ctx) })
	t.Cleanup(func() {
		stop()
		workers.Wait()
	})
	call := func(method, path string, body any, code int) json.RawMessage {
		t.Helper()
		return configCall(t, f, method, "/admin-api/infra/"+path, f.admin, 1, body, code)
	}
	permissionInfo := f.ok("GET", "auth/get-permission-info", nil, f.admin, 1)
	for _, permission := range []string{"infra:job:trigger", "infra:api-error-log:update-status", "infra:redis:get-monitor-info"} {
		if !bytes.Contains(permissionInfo, []byte(permission)) {
			t.Fatalf("missing frontend permission %s", permission)
		}
	}

	job := map[string]any{"name": "清理日志", "handlerName": "jobLogCleanJob", "handlerParam": "test-body-secret",
		"cronExpression": "0 0 2 L * ?", "retryCount": "2", "retryInterval": "5", "monitorTimeout": "1000"}
	job["cronExpression"] = "0 0 2 ? 2 6#5 2027"
	call("POST", "job/create", job, 1_001_001_005)
	job["cronExpression"] = "0 0 2 L * ?"
	data := call("POST", "job/create?secret=test-query-secret", job, 0)
	var id int64
	if err := json.Unmarshal(data, &id); err != nil || id <= 0 {
		t.Fatalf("create job: %s", data)
	}
	call("POST", "job/create", job, 1_001_001_001)
	job["id"], job["name"], job["monitorTimeout"] = id, "更新清理任务", ""
	call("PUT", "job/update", job, 0)
	data = call("GET", "job/page?name=更新&status=1", nil, 0)
	if !bytes.Contains(data, []byte(`"total":1`)) || !bytes.Contains(data, []byte(`"retryCount":2`)) {
		t.Fatalf("job page did not bind filters or quoted numbers: %s", data)
	}
	data = call("GET", fmt.Sprintf("job/get_next_times?id=%d&count=2", id), nil, 0)
	var times []int64
	if err := json.Unmarshal(data, &times); err != nil || len(times) != 2 || times[0] >= times[1] {
		t.Fatalf("job next times: %s", data)
	}

	call("POST", "job/sync", nil, 0)
	call("PUT", fmt.Sprintf("job/update-status?id=%d&status=2", id), nil, 0)
	call("PUT", fmt.Sprintf("job/trigger?id=%d", id), nil, 0)
	workers.Go(func() { f.operations.runner.Run(ctx) })
	type loggedRun struct {
		ID        int64  `json:"id"`
		BeginTime int64  `json:"beginTime"`
		Result    string `json:"result"`
	}
	run := waitForInfraLog[loggedRun](t, func() json.RawMessage {
		return call("GET", fmt.Sprintf("job-log/page?jobId=%d&status=1", id), nil, 0)
	})
	if run.ID == 0 || run.BeginTime <= 0 || !strings.Contains(run.Result, "清理") {
		t.Fatalf("unexpected job log: %+v", run)
	}
	call("GET", fmt.Sprintf("job-log/get?id=%d", run.ID), nil, 0)
	call("PUT", fmt.Sprintf("job/update-status?id=%d&status=1", id), nil, 0)
	for _, path := range []string{"job/export-excel", "job-log/export-excel", "api-access-log/export-excel?resultCode=0"} {
		assertInfraWorkbook(t, f, path)
	}
	call("DELETE", fmt.Sprintf("job/delete-list?ids=%d,%d", id, id+1), nil, 1_001_001_000)
	call("GET", fmt.Sprintf("job/get?id=%d", id), nil, 0)
	call("DELETE", fmt.Sprintf("job/delete-list?ids=%d", id), nil, 0)

	type accessLog struct {
		ID      int64  `json:"id"`
		TraceID string `json:"traceId"`
		Params  string `json:"requestParams"`
		Body    string `json:"responseBody"`
	}
	access := waitForInfraLog[accessLog](t, func() json.RawMessage {
		return call("GET", "api-access-log/page?requestUrl=/infra/job/create&userId=1&resultCode=0", nil, 0)
	})
	if access.ID <= 0 || len(access.TraceID) != 32 || access.Body != `{"code":0}` ||
		!strings.Contains(access.Params, "handlerParam") || strings.Contains(access.Params, "test-") {
		t.Fatalf("access log metadata or masking incorrect: %+v", access)
	}
	call("GET", fmt.Sprintf("api-access-log/get?id=%d", access.ID), nil, 0)

	// miniredis cannot serve INFO commandstats, which exercises the error recorder.
	call("GET", "redis/get-monitor-info", nil, 500)
	type errorLog struct {
		ID            int64  `json:"id"`
		Stack         string `json:"exceptionStackTrace"`
		TraceID       string `json:"traceId"`
		ProcessUserID int64  `json:"processUserId"`
		ProcessTime   int64  `json:"processTime"`
	}
	fault := waitForInfraLog[errorLog](t, func() json.RawMessage {
		return call("GET", "api-error-log/page?processStatus=0&requestUrl=/infra/redis/get-monitor-info", nil, 0)
	})
	if fault.ID <= 0 || fault.Stack == "" || len(fault.TraceID) != 32 {
		t.Fatalf("unexpected error log: %+v", fault)
	}
	call("PUT", fmt.Sprintf("api-error-log/update-status?id=%d&processStatus=2", fault.ID), nil, 0)
	call("PUT", fmt.Sprintf("api-error-log/update-status?id=%d&processStatus=1", fault.ID), nil, 1_001_004_001)
	data = call("GET", fmt.Sprintf("api-error-log/get?id=%d", fault.ID), nil, 0)
	if err := json.Unmarshal(data, &fault); err != nil || fault.ProcessUserID != 1 || fault.ProcessTime <= 0 {
		t.Fatalf("error log missing handler metadata: %s", data)
	}
	assertInfraWorkbook(t, f, "api-error-log/export-excel?processStatus=2")
}

func TestInfraLogExportLimitKeepsPagination(t *testing.T) {
	f := newIAMFixture(t)
	if err := f.db.Exec(`WITH RECURSIVE n(value) AS (
		SELECT 1 UNION ALL SELECT value + 1 FROM n WHERE value < ?
	)
	INSERT INTO infra_api_access_log (tenant_id, trace_id, user_id, user_type, application_name,
		request_method, request_url, request_params, user_ip, user_agent, result_code, response_body,
		result_msg, operate_module, operate_name, operate_type, begin_time, end_time, duration)
	SELECT 1, '', 1, 1, 'owot-pro', 'GET', CASE WHEN value = 1 THEN '/limit/selected' ELSE '/limit' END,
		'{}', '', '', 0, '{}', '', '', '', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0 FROM n`,
		infradomain.MaxLogExportRows+1).Error; err != nil {
		t.Fatal(err)
	}
	const prefix = "/admin-api/infra/api-access-log/"
	configCall(t, f, "GET", prefix+"export-excel?pageSize=1", f.admin, 1, nil, 400)
	page := configCall(t, f, "GET", prefix+"page?pageSize=1", f.admin, 1, nil, 0)
	var result pagination.PageResult[json.RawMessage]
	if err := json.Unmarshal(page, &result); err != nil || result.Total != infradomain.MaxLogExportRows+1 || len(result.List) != 1 {
		t.Fatalf("export limit affected pagination: %s, %v", page, err)
	}
	assertInfraWorkbook(t, f, "api-access-log/export-excel?requestUrl=/limit/selected")
}

func waitForInfraLog[T any](t *testing.T, query func() json.RawMessage) T {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		data := query()
		var page pagination.PageResult[T]
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatalf("invalid log page %s: %v", data, err)
		}
		if len(page.List) > 0 {
			return page.List[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no log written: %s", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertInfraWorkbook(t *testing.T, f *iamFixture, path string) {
	t.Helper()
	response := configRequest(t, f, "GET", "/admin-api/infra/"+path, f.admin, 1, nil)
	book, err := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("export %s: %v", path, err)
	}
	defer func() { _ = book.Close() }()
	if value, err := book.GetCellValue("Sheet1", "A1"); err != nil || value == "" {
		t.Fatalf("export %s missing header: %q, %v", path, value, err)
	}
}
