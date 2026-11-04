package domain

import (
	"time"
	"unicode/utf8"
)

const (
	UserTypeAdmin  = 1
	UserTypeMember = 2

	LoginTypeLogin  = 100
	LoginTypeLogout = 200
	LoginTypeKick   = 202

	LoginResultSuccess     = 0
	LoginResultBadPassword = 10
	LoginResultDisabled    = 20

	MaxUserAgentLength = 1024
)

type OperateLog struct {
	ID            int64
	TenantID      int64
	TraceID       string
	UserID        int64
	UserType      int
	Type          string
	SubType       string
	BizID         int64
	Action        string
	Extra         string
	RequestMethod string
	RequestURL    string
	UserIP        string
	UserAgent     string
	CreateTime    time.Time
	UserNickname  string
}

func (l OperateLog) Validate() error {
	if l.UserID <= 0 || !validUserType(l.UserType) || !validText(l.Type, 50) ||
		!validText(l.SubType, 50) || utf8.RuneCountInString(l.TraceID) > 64 ||
		utf8.RuneCountInString(l.Action) > 2000 || utf8.RuneCountInString(l.RequestMethod) > 16 ||
		utf8.RuneCountInString(l.RequestURL) > 2048 || utf8.RuneCountInString(l.UserIP) > 64 ||
		utf8.RuneCountInString(l.UserAgent) > MaxUserAgentLength {
		return ErrInvalidArgument
	}
	return nil
}

type LoginLog struct {
	ID         int64
	TenantID   int64
	LogType    int
	TraceID    string
	UserID     int64
	UserType   int
	Username   string
	Result     int
	UserIP     string
	UserAgent  string
	CreateTime time.Time
}

func (l LoginLog) Validate() error {
	if !validUserType(l.UserType) || !validText(l.Username, 100) || l.UserID < 0 ||
		(l.LogType != LoginTypeLogin && l.LogType != LoginTypeLogout && l.LogType != LoginTypeKick) ||
		(l.Result != LoginResultSuccess && l.Result != LoginResultBadPassword && l.Result != LoginResultDisabled) ||
		utf8.RuneCountInString(l.TraceID) > 64 || utf8.RuneCountInString(l.UserIP) > 64 ||
		utf8.RuneCountInString(l.UserAgent) > MaxUserAgentLength {
		return ErrInvalidArgument
	}
	return nil
}

func validUserType(t int) bool { return t == UserTypeAdmin || t == UserTypeMember }
