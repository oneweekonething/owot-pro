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
		{"用户性别", "system_user_sex", []seedItem{{"未知", "0"}, {"男", "1"}, {"女", "2"}}},
		{"数据权限范围", "system_data_scope", []seedItem{{"全部数据", "1"}, {"自定义部门", "2"}, {"本部门", "3"}, {"本部门及以下", "4"}, {"仅本人", "5"}}},
		{"角色类型", "system_role_type", []seedItem{{"内置", "1"}, {"自定义", "2"}}},
		{"菜单类型", "system_menu_type", []seedItem{{"目录", "1"}, {"菜单", "2"}, {"按钮", "3"}}},
		{"通知公告类型", "system_notice_type", []seedItem{{"通知", "1"}, {"公告", "2"}}},
		{"登录日志类型", "system_login_type", []seedItem{{"账号登录", "100"}, {"主动登出", "200"}, {"强制退出", "202"}}},
		{"登录结果", "system_login_result", []seedItem{{"成功", "0"}, {"账号或密码不正确", "10"}, {"用户被禁用", "20"}}},
		{"用户类型", "user_type", []seedItem{{"管理后台用户", "1"}, {"会员用户", "2"}}},
		{"参数配置类型", "infra_config_type", []seedItem{{"系统内置", "1"}, {"自定义", "2"}}},
		{"是否可见", "infra_boolean_string", []seedItem{{"可见", "true"}, {"不可见", "false"}}},
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
