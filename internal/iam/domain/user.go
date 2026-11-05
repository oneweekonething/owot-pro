package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	StatusEnabled  = 0
	StatusDisabled = 1
)

type AdminUser struct {
	ID, TenantID                     int64
	Username, Nickname, PasswordHash string
	DeptID                           int64
	PostIDs                          []int64
	Email, Mobile                    string
	Sex, Status                      int
	Avatar, Remark, LoginIP          string
	LoginDate, CreateTime            time.Time
}

func ValidStatus(status int) bool { return status == StatusEnabled || status == StatusDisabled }

func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < 4 || utf8.RuneCountInString(plain) > 16 || len(plain) > 72 || strings.IndexFunc(plain, unicode.IsSpace) >= 0 {
		return ErrPassword
	}
	return nil
}

func (u AdminUser) Validate() error {
	if len(u.Username) < 4 || len(u.Username) > 30 || !text(u.Nickname, 30) || !ValidStatus(u.Status) || u.DeptID < 0 || u.Sex < 0 || u.Sex > 2 || !optionalText(u.Remark, 500) || !optionalText(u.Avatar, 1024) || !ValidIDs(u.PostIDs) {
		return ErrInvalidArgument
	}
	for _, c := range u.Username {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !valid {
			return ErrInvalidArgument
		}
	}
	if u.Email != "" {
		address, err := mail.ParseAddress(u.Email)
		if err != nil || address.Address != u.Email || len(u.Email) > 100 {
			return ErrInvalidArgument
		}
	}
	if u.Mobile != "" {
		if len(u.Mobile) != 11 {
			return ErrInvalidArgument
		}
		for _, c := range u.Mobile {
			if c < '0' || c > '9' {
				return ErrInvalidArgument
			}
		}
	}
	return nil
}

func (u AdminUser) IsInitialAdmin() bool { return u.TenantID == 1 && u.Username == "admin" }

func text(value string, max int) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= max
}
func optionalText(value string, max int) bool { return utf8.RuneCountInString(value) <= max }
func ValidIDs(ids []int64) bool {
	for _, id := range ids {
		if id <= 0 {
			return false
		}
	}
	return true
}

func UniqueIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}
