// Package domain contains System business rules and framework-free entities.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	StatusEnabled  = 0
	StatusDisabled = 1
)

type DictType struct {
	ID         int64
	Name       string
	Type       string
	Status     int
	Remark     string
	CreateTime time.Time
	UpdateTime time.Time
}

func (d DictType) Validate() error {
	if !validText(d.Name, 100) || !validText(d.Type, 100) ||
		!ValidStatus(d.Status) || utf8.RuneCountInString(d.Remark) > 500 {
		return ErrInvalidArgument
	}
	return nil
}

type DictData struct {
	ID         int64
	Sort       int
	Label      string
	Value      string
	DictType   string
	Status     int
	ColorType  string
	CSSClass   string
	Remark     string
	CreateTime time.Time
	UpdateTime time.Time
}

func (d DictData) Validate() error {
	if !validText(d.Label, 100) || !validText(d.Value, 100) || !validText(d.DictType, 100) ||
		!ValidStatus(d.Status) || d.Sort < 0 || utf8.RuneCountInString(d.ColorType) > 100 ||
		utf8.RuneCountInString(d.CSSClass) > 100 || utf8.RuneCountInString(d.Remark) > 500 {
		return ErrInvalidArgument
	}
	return nil
}

func ValidStatus(status int) bool { return status == StatusEnabled || status == StatusDisabled }

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max
}
