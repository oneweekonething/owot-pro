package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ConfigBuiltin = 1
	ConfigCustom  = 2
)

type Config struct {
	ID                                 int64
	Category, Name, Key, Value, Remark string
	Type                               int
	Visible                            bool
	CreateTime, UpdateTime             time.Time
}

func ValidateConfigKey(key string) error {
	if key == "" || strings.TrimSpace(key) != key || !validText(key, 100) {
		return ErrInvalidArgument
	}
	return nil
}

func (c Config) Validate() error {
	if ValidateConfigKey(c.Key) != nil ||
		strings.TrimSpace(c.Name) == "" || !validText(c.Name, 100) ||
		strings.TrimSpace(c.Category) == "" || !validText(c.Category, 50) ||
		strings.TrimSpace(c.Value) == "" || !validText(c.Value, 500) ||
		!validText(c.Remark, 500) {
		return ErrInvalidArgument
	}
	return nil
}

func (c Config) CanDelete() error {
	if c.Type == ConfigBuiltin {
		return ErrBuiltinConfigDelete
	}
	return nil
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max
}
