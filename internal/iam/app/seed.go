package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/ctxmeta"
)

// SeedDefaults only creates missing objects; rerunning never resets credentials.
func (s *Service) SeedDefaults(ctx context.Context) error {
	ctx = ctxmeta.WithTenantID(ctx, 1)
	return s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.Tenants.Get(ctx, 1)
		if errors.Is(err, domain.ErrTenantNotFound) {
			t = domain.Tenant{ID: 1, Name: "芋道源码", ContactName: "管理员", PackageID: 0, AccountCount: 999999, ExpireTime: time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), Websites: []string{}}
			if err := s.Tenants.Create(ctx, &t); err != nil {
				return err
			}
			if t.ID != 1 {
				return domain.ErrProtected
			}
		} else if err != nil {
			return err
		}
		if t.PackageID != 0 {
			return domain.ErrProtected
		}
		if err := s.Locks.LockGlobal(ctx); err != nil {
			return err
		}
		q := all()
		q.ClientID = "default"
		clients, err := s.Clients.List(ctx, q)
		if err != nil {
			return err
		}
		if len(clients.List) == 0 {
			client := domain.OAuth2Client{ClientID: "default", Secret: "default", Name: "默认客户端", AccessTokenValiditySeconds: 1800, RefreshTokenValiditySeconds: 2592000, AuthorizedGrantTypes: []string{"password", "refresh_token"}, RedirectURIs: []string{}, Scopes: []string{}, AutoApproveScopes: []string{}, Authorities: []string{}}
			if err := s.Clients.Save(ctx, &client); err != nil {
				return err
			}
		}
		roles, err := s.Roles.List(ctx, all())
		if err != nil {
			return err
		}
		var role domain.Role
		for _, r := range roles.List {
			if r.Code == "super_admin" {
				role = r
			}
		}
		if role.ID == 0 {
			role = domain.Role{Name: "超级管理员", Code: "super_admin", Type: 1, DataScope: domain.DataScopeAll, DataScopeDeptIDs: []int64{}}
			if err := s.Roles.Save(ctx, &role); err != nil {
				return err
			}
		}
		q = all()
		q.ExactUsername = "admin"
		users, err := s.Users.List(ctx, q)
		if err != nil {
			return err
		}
		if len(users.List) == 0 {
			dept := domain.Dept{Name: "芋道源码"}
			if err := s.Depts.Save(ctx, &dept); err != nil {
				return err
			}
			hash, err := s.Hasher.Hash("admin123")
			if err != nil {
				return err
			}
			user := domain.AdminUser{ID: 1, Username: "admin", Nickname: "管理员", PasswordHash: hash, DeptID: dept.ID, PostIDs: []int64{}}
			if err := s.Users.Create(ctx, &user); err != nil {
				return err
			}
			if user.ID != 1 {
				return domain.ErrProtected
			}
			if err := s.Links.AssignUserRoles(ctx, user.ID, []int64{role.ID}); err != nil {
				return err
			}
			t.ContactUserID = user.ID
			if err := s.Tenants.Save(ctx, &t); err != nil {
				return err
			}
		}
		menus, err := s.Menus.List(ctx, all())
		if err != nil {
			return err
		}
		if len(menus.List) == 0 {
			pageNames := map[string]string{"user": "用户管理", "dept": "部门管理", "post": "岗位管理", "role": "角色管理", "menu": "菜单管理", "tenant": "租户管理", "tenant-package": "租户套餐", "oauth2-client": "OAuth2客户端", "oauth2-token": "在线令牌", "dict": "字典管理", "notice": "通知公告", "operate-log": "操作日志", "login-log": "登录日志", "config": "参数配置", "file": "文件管理", "file-config": "文件配置", "job": "定时任务", "job-log": "任务日志", "api-access-log": "API访问日志", "api-error-log": "API错误日志", "redis": "Redis监控"}
			components := map[string]string{"tenant-package": "system/tenant/package/index", "oauth2-client": "system/oauth2/client/index", "oauth2-token": "system/oauth2/token/index", "operate-log": "system/operatelog/index", "login-log": "system/loginlog/index", "file-config": "infra/fileConfig/index", "job-log": "infra/job/logger/index", "api-access-log": "infra/apiAccessLog/index", "api-error-log": "infra/apiErrorLog/index"}
			actionNames := map[string]string{"query": "查询", "create": "新增", "update": "修改", "delete": "删除", "export": "导出", "update-password": "重置密码", "import": "导入", "assign-menu": "分配菜单", "assign-data-scope": "分配数据权限", "page": "查询在线令牌"}
			pageIDs := map[string]int64{}
			for _, group := range []struct {
				Name, Path string
				Items      []string
			}{
				{"系统管理", "system", []string{"user", "dept", "post", "role", "menu", "tenant", "tenant-package", "oauth2-client", "oauth2-token", "dict", "notice", "operate-log", "login-log"}},
				{"基础设施", "infra", []string{"config", "file", "file-config", "job", "job-log", "api-access-log", "api-error-log", "redis"}},
			} {
				root := domain.Menu{Name: group.Name, Path: "/" + group.Path, Type: 1, Visible: true, AlwaysShow: true}
				if err := s.Menus.Save(ctx, &root); err != nil {
					return err
				}
				for i, item := range group.Items {
					component := components[item]
					if component == "" {
						component = group.Path + "/" + item + "/index"
					}
					parts := strings.Split(group.Path+"-"+item, "-")
					routeName := ""
					for _, part := range parts {
						routeName += strings.ToUpper(part[:1]) + part[1:]
					}
					menu := domain.Menu{Name: pageNames[item], Path: item, Component: component, ComponentName: routeName, ParentID: root.ID, Type: 2, Sort: i, Visible: true, KeepAlive: true}
					if err := s.Menus.Save(ctx, &menu); err != nil {
						return err
					}
					pageIDs[item] = menu.ID
					actions := []string{"query", "create", "update", "delete", "export"}
					if item == "user" {
						actions = append(actions, "update-password", "import")
					}
					if item == "role" {
						actions = append(actions, "assign-menu", "assign-data-scope")
					}
					if item == "oauth2-token" {
						actions = append(actions, "page")
					}
					for j, action := range actions {
						button := domain.Menu{Name: actionNames[action], ParentID: menu.ID, Type: 3, Sort: j, Permission: group.Path + ":" + item + ":" + action}
						if err := s.Menus.Save(ctx, &button); err != nil {
							return err
						}
					}
				}
			}
			// The permission screen uses dedicated identifiers in addition to role/user actions.
			for _, action := range []string{"assign-role-menu", "assign-role-data-scope", "assign-user-role"} {
				parent := pageIDs["role"]
				if action == "assign-user-role" {
					parent = pageIDs["user"]
				}
				m := domain.Menu{Name: action, ParentID: parent, Type: 3, Permission: "system:permission:" + action}
				if err := s.Menus.Save(ctx, &m); err != nil {
					return err
				}
			}
			menus, err = s.Menus.List(ctx, all())
			if err != nil {
				return err
			}
			ids := []int64{}
			for _, menu := range menus.List {
				ids = append(ids, menu.ID)
			}
			if err := s.Links.AssignRoleMenus(ctx, role.ID, ids); err != nil {
				return err
			}
		}
		if err := s.seedInfraOperationMenus(ctx, menus.List); err != nil {
			return err
		}
		return s.invalidate(ctxmeta.WithTenantID(ctx, 0), "seed")
	})
}

func (s *Service) seedInfraOperationMenus(ctx context.Context, menus []domain.Menu) error {
	var rootID int64
	permissions := make(map[string]bool)
	for _, menu := range menus {
		if menu.ParentID == 0 && menu.Type == 1 && menu.Path == "/infra" {
			rootID = menu.ID
		}
		permissions[menu.Permission] = true
	}
	if rootID == 0 {
		return nil
	}
	pages := make(map[string]int64)
	for _, menu := range menus {
		if menu.ParentID != rootID || menu.Type != 2 {
			continue
		}
		pages[menu.Path] = menu.ID
		if menu.Path == "job-log" && menu.Component == "infra/job/logger/index" && menu.ComponentName == "InfraJobLog" {
			// The upstream UI already registers /job/job-log with this route name.
			menu.ComponentName = "InfraJobLogMenu"
			if err := s.Menus.Save(ctx, &menu); err != nil {
				return err
			}
		}
	}
	for _, item := range []struct{ page, action, name string }{
		{"job", "trigger", "执行任务"},
		{"api-error-log", "update-status", "处理错误"},
		{"redis", "get-monitor-info", "查看 Redis 监控"},
	} {
		permission := "infra:" + item.page + ":" + item.action
		if parent := pages[item.page]; parent != 0 && !permissions[permission] {
			button := domain.Menu{Name: item.name, ParentID: parent, Type: 3, Permission: permission}
			if err := s.Menus.Save(ctx, &button); err != nil {
				return err
			}
		}
	}
	return nil
}
