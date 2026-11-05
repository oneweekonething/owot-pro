package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

const (
	requestBodyLimit = 4096
	resultCodeKey    = "owot:result_code"
	requestErrorKey  = "owot:request_error"
	requestRouteKey  = "owot:request_route"
)

type AccessLog struct {
	Method, URL, Params, UserIP, UserAgent string
	Module, Operation, ResultMessage       string
	ResultCode, OperationType              int
	BeginTime, EndTime                     time.Time
}

type ErrorLog struct {
	At                          time.Time
	Name, Stack, Function, File string
	Line                        int
}

type AccessLogWriter interface {
	WriteAccess(context.Context, AccessLog) error
}

type ErrorLogWriter interface {
	WriteError(context.Context, AccessLog, ErrorLog) error
}

type requestEntry struct {
	ctx    context.Context
	access AccessLog
	fault  *ErrorLog
}

type RequestLogs struct {
	access  AccessLogWriter
	errors  ErrorLogWriter
	logger  *slog.Logger
	queue   chan requestEntry
	dropped atomic.Uint64
}

func NewRequestLogs(access AccessLogWriter, errors ErrorLogWriter, logger *slog.Logger) *RequestLogs {
	return &RequestLogs{access: access, errors: errors, logger: logger, queue: make(chan requestEntry, 256)}
}

func (l *RequestLogs) Run(ctx context.Context) {
	const writeTimeout = 2 * time.Second
	for ctx.Err() == nil {
		select {
		case entry := <-l.queue:
			l.write(entry, writeTimeout)
		case <-ctx.Done():
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(l.queue) > 0 {
		timeout := min(time.Until(deadline), writeTimeout)
		if timeout <= 0 {
			l.logger.Warn("request logs dropped during shutdown", "count", len(l.queue))
			return
		}
		l.write(<-l.queue, timeout)
	}
}

func (l *RequestLogs) write(entry requestEntry, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(entry.ctx, timeout)
	defer cancel()
	if entry.fault != nil {
		if err := l.errors.WriteError(ctx, entry.access, *entry.fault); err != nil {
			l.logger.ErrorContext(ctx, "record API error log", "trace_id", ctxmeta.TraceID(ctx), "error", err)
		}
	}
	if err := l.access.WriteAccess(ctx, entry.access); err != nil {
		l.logger.ErrorContext(ctx, "record API access log", "trace_id", ctxmeta.TraceID(ctx), "error", err)
	}
}

func (l *RequestLogs) middleware(c *gin.Context) {
	if !strings.HasPrefix(c.Request.URL.Path, "/admin-api/") && !strings.HasPrefix(c.Request.URL.Path, "/app-api/") {
		c.Next()
		return
	}
	begin := time.Now()
	var body capturedBody
	contentType, _, _ := strings.Cut(c.GetHeader("Content-Type"), ";")
	if c.Request.Body != nil && (contentType == "application/json" || contentType == "application/x-www-form-urlencoded") {
		c.Request.Body = struct {
			io.Reader
			io.Closer
		}{io.TeeReader(c.Request.Body, &body), c.Request.Body}
	}
	c.Next()
	end := time.Now()

	ctx := c.Request.Context()
	route, ok := c.Get(requestRouteKey)
	if !ok || ctxmeta.IsIgnoreTenant(ctx) || ctxmeta.TenantID(ctx) <= 0 {
		return
	}
	meta := route.(Route)
	code, ok := c.Get(resultCodeKey)
	result := c.Writer.Status()
	if ok {
		result = code.(int)
	} else if result < 400 {
		result = 0
	}
	var message string
	if result != 0 {
		message = "请求失败"
		if result == 500 {
			message = "系统异常"
		}
	}
	entry := requestEntry{
		ctx: ctxmeta.CloneContext(ctx),
		access: AccessLog{Method: logText(c.Request.Method, 16), URL: logText(c.Request.URL.EscapedPath(), 512),
			Params: requestParams(c.Request.URL.RawQuery, contentType, &body), UserIP: logText(c.ClientIP(), 64),
			UserAgent: logText(c.Request.UserAgent(), 512), Module: meta.AuditType, Operation: meta.AuditSubType,
			OperationType: operationType(c.Request.Method), ResultCode: result, ResultMessage: message, BeginTime: begin, EndTime: end},
	}
	if fault, ok := c.Get(requestErrorKey); ok {
		data := fault.(ErrorLog)
		entry.fault = &data
	}
	select {
	case l.queue <- entry:
	default:
		count := l.dropped.Add(1)
		if count&(count-1) == 0 {
			l.logger.Warn("request log queue full", "dropped", count)
		}
	}
}

type capturedBody struct {
	bytes.Buffer
	truncated bool
}

func (b *capturedBody) Write(p []byte) (int, error) {
	remaining := requestBodyLimit - b.Len()
	if len(p) > remaining {
		b.truncated = true
	}
	_, _ = b.Buffer.Write(p[:min(len(p), remaining)])
	return len(p), nil
}

// Only names are recorded. Query credentials, form values, response payloads
// and free-form error messages may contain secrets or other tenants' data.
func requestParams(rawQuery, contentType string, body *capturedBody) string {
	values := map[string]any{}
	if len(rawQuery) <= requestBodyLimit {
		query, _ := url.ParseQuery(rawQuery)
		values["query"] = sortedNames(query)
	}
	if !body.truncated && body.Len() != 0 {
		switch contentType {
		case "application/json":
			var fields map[string]json.RawMessage
			if json.Unmarshal(body.Bytes(), &fields) == nil {
				values["body"] = sortedNames(fields)
			}
		case "application/x-www-form-urlencoded":
			form, _ := url.ParseQuery(body.String())
			values["body"] = sortedNames(form)
		}
	}
	result, _ := json.Marshal(values)
	return string(result)
}

func sortedNames[M ~map[string]V, V any](fields M) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		if !strings.EqualFold(name, "token") {
			names = append(names, logText(name, 100))
		}
	}
	sort.Strings(names)
	return names[:min(len(names), 32)]
}

func logText(value string, maxLength int) string {
	runes := []rune(value)
	return string(runes[:min(len(runes), maxLength)])
}

func operationType(method string) int {
	switch method {
	case "GET", "HEAD":
		return 1
	case "POST":
		return 2
	case "PUT", "PATCH":
		return 3
	case "DELETE":
		return 4
	default:
		return 0
	}
}

func captureRequestError(c *gin.Context, value any) {
	pcs := make([]uintptr, 24)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs)])
	fault := ErrorLog{At: time.Now().UTC(), Name: logText(fmt.Sprintf("%T", value), 128)}
	var stack strings.Builder
	for {
		frame, more := frames.Next()
		file := filepath.Base(frame.File)
		if fault.Function == "" && !strings.HasPrefix(frame.Function, "runtime.") {
			fault.Function, fault.File, fault.Line = logText(frame.Function, 255), logText(file, 255), frame.Line
		}
		stack.WriteString(frame.Function + " (" + file + ":" + strconv.Itoa(frame.Line) + ")\n")
		if !more {
			break
		}
	}
	fault.Stack = logText(stack.String(), 8192)
	c.Set(requestErrorKey, fault)
}
