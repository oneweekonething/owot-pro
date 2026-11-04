package app

import (
	"context"
	"errors"

	"oneweekonething.com/owot-pro/internal/system/domain"
)

// SeedDefaults is explicitly invoked by the startup command, never by init().
// Existing types and values are preserved so rerunning it does not overwrite edits.
func (s *DictService) SeedDefaults(ctx context.Context) error {
	type seedItem struct{ label, value string }
	defaults := []struct {
		name, typ string
		items     []seedItem
	}{
		{"通用状态", "common_status", []seedItem{{"开启", "0"}, {"关闭", "1"}}},
		{"通知公告类型", "system_notice_type", []seedItem{{"通知", "1"}, {"公告", "2"}}},
		{"登录日志类型", "system_login_type", []seedItem{{"账号登录", "100"}, {"主动登出", "200"}, {"强制退出", "202"}}},
		{"登录结果", "system_login_result", []seedItem{{"成功", "0"}, {"账号或密码不正确", "10"}, {"用户被禁用", "20"}}},
		{"用户类型", "user_type", []seedItem{{"管理后台用户", "1"}, {"会员用户", "2"}}},
	}
	for _, def := range defaults {
		_, err := s.CreateType(ctx, domain.DictType{Name: def.name, Type: def.typ})
		if err != nil && !errors.Is(err, domain.ErrDictTypeExists) {
			return err
		}
		for i, item := range def.items {
			_, err := s.CreateData(ctx, domain.DictData{DictType: def.typ, Label: item.label, Value: item.value, Sort: i})
			if err != nil && !errors.Is(err, domain.ErrDictValueExists) {
				return wrap("seed dictionary item "+def.typ+"/"+item.value, err)
			}
		}
	}
	return nil
}
