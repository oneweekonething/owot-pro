package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestEntityValidation(t *testing.T) {
	tests := []struct {
		name  string
		value interface{ Validate() error }
		valid bool
	}{
		{"type enabled", DictType{Name: "通用状态", Type: "common_status", Status: 0}, true},
		{"type disabled", DictType{Name: "通用状态", Type: "common_status", Status: 1}, true},
		{"type blank", DictType{Name: "  ", Type: "common_status"}, false},
		{"type long", DictType{Name: strings.Repeat("字", 101), Type: "common_status"}, false},
		{"type invalid status", DictType{Name: "状态", Type: "status", Status: 2}, false},
		{"data zero value", DictData{Label: "开启", Value: "0", DictType: "status"}, true},
		{"data negative sort", DictData{Label: "开启", Value: "0", DictType: "status", Sort: -1}, false},
		{"data blank value", DictData{Label: "开启", Value: " ", DictType: "status"}, false},
		{"notice valid", Notice{Title: "公告", Content: "内容", Type: 2}, true},
		{"notice invalid type", Notice{Title: "公告", Content: "内容", Type: 3}, false},
		{"notice long title", Notice{Title: strings.Repeat("字", 51), Content: "内容", Type: 1}, false},
		{"operate valid", OperateLog{UserID: 1, UserType: 1, Type: "字典", SubType: "创建"}, true},
		{"operate invalid user", OperateLog{UserType: 1, Type: "字典", SubType: "创建"}, false},
		{"login unknown username", LoginLog{Username: "unknown", UserType: 1, LogType: 100, Result: 10}, true},
		{"login disabled", LoginLog{Username: "user", UserType: 1, LogType: 100, Result: 20}, true},
		{"login invalid result", LoginLog{Username: "user", UserType: 1, LogType: 100, Result: 99}, false},
		{"login wrong user type", LoginLog{Username: "user", UserType: 3, LogType: 100}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if tt.valid && err != nil || !tt.valid && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Validate() = %v; valid = %v", err, tt.valid)
			}
		})
	}
}
