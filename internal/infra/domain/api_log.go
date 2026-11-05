package domain

import "time"

const MaxLogExportRows = 5000

const (
	ErrorUnhandled = 0
	ErrorProcessed = 1
	ErrorIgnored   = 2
)

type RequestLog struct {
	TraceID, ApplicationName, RequestMethod, RequestURL string
	RequestParams, UserIP, UserAgent                    string
	UserID                                              int64
	UserType, ResultCode                                int
}

type APIAccessLog struct {
	RequestLog
	ID                             int64
	ResponseBody, ResultMsg        string
	OperateModule, OperateName     string
	OperateType                    int
	BeginTime, EndTime, CreateTime time.Time
	Duration                       int64
}

type APIErrorLog struct {
	RequestLog
	ID, ProcessUserID                                          int64
	ProcessStatus, ExceptionLineNumber                         int
	ExceptionName, ExceptionMessage                            string
	ExceptionRootCauseMessage, ExceptionStackTrace             string
	ExceptionClassName, ExceptionFileName, ExceptionMethodName string
	ExceptionTime, CreateTime                                  time.Time
	ProcessTime                                                *time.Time
}
