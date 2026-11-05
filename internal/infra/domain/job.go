package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	JobEnabled = 1
	JobPaused  = 2

	JobRunning   = 0
	JobSucceeded = 1
	JobFailed    = 2
)

type Job struct {
	ID                                int64
	Name, HandlerName, HandlerParam   string
	CronExpression                    string
	Status, RetryCount, RetryInterval int
	MonitorTimeout                    int
	CreateTime                        time.Time
	NextRun                           *time.Time
	ManualPending                     bool
	Revision                          int64
}

func (j Job) Validate() error {
	if strings.TrimSpace(j.Name) == "" || utf8.RuneCountInString(j.Name) > 64 ||
		j.HandlerName == "" || len(j.HandlerName) > 64 ||
		utf8.RuneCountInString(j.HandlerParam) > 255 || j.CronExpression == "" || len(j.CronExpression) > 128 ||
		j.RetryCount < 0 || j.RetryCount > 10 || j.RetryInterval < 0 || j.RetryInterval > 3600000 ||
		j.MonitorTimeout < 0 || j.MonitorTimeout > 3600000 {
		return ErrInvalidArgument
	}
	return nil
}

func (j Job) Timeout() time.Duration {
	if j.MonitorTimeout == 0 {
		return 5 * time.Minute
	}
	return time.Duration(j.MonitorTimeout) * time.Millisecond
}

type JobLog struct {
	ID, JobID                                 int64
	HandlerName, HandlerParam, CronExpression string
	ExecuteIndex, Status                      int
	BeginTime, CreateTime                     time.Time
	EndTime                                   *time.Time
	Duration                                  int64
	Result                                    string
}
