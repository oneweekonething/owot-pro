package http

import (
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
	"oneweekonething.com/owot-pro/internal/system/domain"
)

type dictTypeDTO struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Type       string              `json:"type"`
	Status     int                 `json:"status"`
	Remark     string              `json:"remark"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	UpdateTime timeutil.MillisTime `json:"updateTime"`
}

func typeDTO(v domain.DictType) dictTypeDTO {
	return dictTypeDTO{v.ID, v.Name, v.Type, v.Status, v.Remark, timeutil.MillisTime{Time: v.CreateTime}, timeutil.MillisTime{Time: v.UpdateTime}}
}

type dictDataDTO struct {
	ID         int64               `json:"id"`
	Sort       int                 `json:"sort"`
	Label      string              `json:"label"`
	Value      string              `json:"value"`
	DictType   string              `json:"dictType"`
	Status     int                 `json:"status"`
	ColorType  string              `json:"colorType"`
	CSSClass   string              `json:"cssClass"`
	Remark     string              `json:"remark"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	UpdateTime timeutil.MillisTime `json:"updateTime"`
}

func dataDTO(v domain.DictData) dictDataDTO {
	return dictDataDTO{v.ID, v.Sort, v.Label, v.Value, v.DictType, v.Status, v.ColorType, v.CSSClass, v.Remark,
		timeutil.MillisTime{Time: v.CreateTime}, timeutil.MillisTime{Time: v.UpdateTime}}
}

type noticeDTO struct {
	ID         int64               `json:"id"`
	Title      string              `json:"title"`
	Type       int                 `json:"type"`
	Content    string              `json:"content"`
	Status     int                 `json:"status"`
	Creator    string              `json:"creator"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	UpdateTime timeutil.MillisTime `json:"updateTime"`
}

func toNoticeDTO(v domain.Notice) noticeDTO {
	return noticeDTO{v.ID, v.Title, v.Type, v.Content, v.Status, v.Creator, timeutil.MillisTime{Time: v.CreateTime}, timeutil.MillisTime{Time: v.UpdateTime}}
}

type operateLogDTO struct {
	ID            int64               `json:"id"`
	TraceID       string              `json:"traceId"`
	UserID        int64               `json:"userId"`
	UserType      int                 `json:"userType"`
	UserNickname  string              `json:"userName"`
	Type          string              `json:"type"`
	SubType       string              `json:"subType"`
	BizID         int64               `json:"bizId"`
	Action        string              `json:"action"`
	Extra         string              `json:"extra"`
	RequestMethod string              `json:"requestMethod"`
	RequestURL    string              `json:"requestUrl"`
	UserIP        string              `json:"userIp"`
	UserAgent     string              `json:"userAgent"`
	CreateTime    timeutil.MillisTime `json:"createTime"`
}

func toOperateDTO(v domain.OperateLog) operateLogDTO {
	return operateLogDTO{v.ID, v.TraceID, v.UserID, v.UserType, v.UserNickname, v.Type, v.SubType, v.BizID,
		v.Action, v.Extra, v.RequestMethod, v.RequestURL, v.UserIP, v.UserAgent, timeutil.MillisTime{Time: v.CreateTime}}
}

type loginLogDTO struct {
	ID         int64               `json:"id"`
	LogType    int                 `json:"logType"`
	TraceID    string              `json:"traceId"`
	UserID     int64               `json:"userId"`
	UserType   int                 `json:"userType"`
	Username   string              `json:"username"`
	Result     int                 `json:"result"`
	UserIP     string              `json:"userIp"`
	UserAgent  string              `json:"userAgent"`
	CreateTime timeutil.MillisTime `json:"createTime"`
}

func toLoginDTO(v domain.LoginLog) loginLogDTO {
	return loginLogDTO{v.ID, v.LogType, v.TraceID, v.UserID, v.UserType, v.Username, v.Result, v.UserIP, v.UserAgent, timeutil.MillisTime{Time: v.CreateTime}}
}

func convert[A, B any](values []A, fn func(A) B) []B {
	result := make([]B, 0, len(values))
	for _, value := range values {
		result = append(result, fn(value))
	}
	return result
}
