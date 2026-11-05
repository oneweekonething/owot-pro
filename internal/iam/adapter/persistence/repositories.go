package persistence

import (
	"context"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/iam/app"
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

func NewUserRepository(db *gorm.DB) app.Repository[domain.AdminUser] {
	r := &repository[domain.AdminUser, userPO]{db: db, tenantScoped: true, missing: domain.ErrUserNotFound, toPO: func(v domain.AdminUser) userPO {
		return userPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, TenantID: v.TenantID, Username: v.Username, Nickname: v.Nickname, PasswordHash: v.PasswordHash, DeptID: v.DeptID, PostIDs: v.PostIDs, Email: v.Email, Mobile: v.Mobile, Sex: v.Sex, Status: v.Status, Avatar: v.Avatar, Remark: v.Remark, LoginIP: v.LoginIP, LoginDate: nullableTime(v.LoginDate)}
	}, toDomain: func(p userPO) domain.AdminUser {
		return domain.AdminUser{ID: p.ID, CreateTime: p.CreateTime, TenantID: p.TenantID, Username: p.Username, Nickname: p.Nickname, PasswordHash: p.PasswordHash, DeptID: p.DeptID, PostIDs: p.PostIDs, Email: p.Email, Mobile: p.Mobile, Sex: p.Sex, Status: p.Status, Avatar: p.Avatar, Remark: p.Remark, LoginIP: p.LoginIP, LoginDate: timeValue(p.LoginDate)}
	}, base: func(p *userPO) *dbx.BasePO { return &p.BasePO }, active: func(p *userPO) {
		p.ActiveUsername = active(p.Username)
		p.ActiveMobile = active(p.Mobile)
		p.ActiveEmail = active(p.Email)
	}, clearActive: func(p *userPO) { p.ActiveUsername = nil; p.ActiveMobile = nil; p.ActiveEmail = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("nickname LIKE ?", "%"+q.Name+"%")
		}
		if q.Username != "" {
			db = db.Where("username LIKE ?", "%"+q.Username+"%")
		}
		if q.ExactUsername != "" {
			db = db.Where("username = ?", q.ExactUsername)
		}
		if q.Mobile != "" {
			db = db.Where("mobile LIKE ?", "%"+q.Mobile+"%")
		}
		if q.DeptIDs != nil {
			db = db.Where("dept_id IN ?", q.DeptIDs)
		}
		return db
	}}
	r.afterSave = func(ctx context.Context, p *userPO) error { return syncUserPosts(ctx, db, p.ID, p.PostIDs) }
	return r
}
func NewDeptRepository(db *gorm.DB) app.Repository[domain.Dept] {
	r := &repository[domain.Dept, deptPO]{db: db, tenantScoped: true, missing: domain.ErrDeptNotFound, order: "sort ASC, id ASC", toPO: func(v domain.Dept) deptPO {
		return deptPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, TenantID: v.TenantID, ParentID: v.ParentID, LeaderUserID: v.LeaderUserID, Name: v.Name, Phone: v.Phone, Email: v.Email, Sort: v.Sort, Status: v.Status}
	}, toDomain: func(p deptPO) domain.Dept {
		return domain.Dept{ID: p.ID, CreateTime: p.CreateTime, TenantID: p.TenantID, ParentID: p.ParentID, LeaderUserID: p.LeaderUserID, Name: p.Name, Phone: p.Phone, Email: p.Email, Sort: p.Sort, Status: p.Status}
	}, base: func(p *deptPO) *dbx.BasePO { return &p.BasePO }, active: func(p *deptPO) {}, clearActive: func(p *deptPO) {}, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		return db
	}}
	return r
}
func NewPostRepository(db *gorm.DB) app.Repository[domain.Post] {
	r := &repository[domain.Post, postPO]{db: db, tenantScoped: true, missing: domain.ErrPostNotFound, order: "sort ASC, id ASC", toPO: func(v domain.Post) postPO {
		return postPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, TenantID: v.TenantID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status}
	}, toDomain: func(p postPO) domain.Post {
		return domain.Post{ID: p.ID, CreateTime: p.CreateTime, TenantID: p.TenantID, Name: p.Name, Code: p.Code, Remark: p.Remark, Sort: p.Sort, Status: p.Status}
	}, base: func(p *postPO) *dbx.BasePO { return &p.BasePO }, active: func(p *postPO) { p.ActiveCode = active(p.Code); p.ActiveName = active(p.Name) }, clearActive: func(p *postPO) { p.ActiveCode = nil; p.ActiveName = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		if q.Code != "" {
			db = db.Where("code LIKE ?", "%"+q.Code+"%")
		}
		return db
	}}
	return r
}
func NewRoleRepository(db *gorm.DB) app.Repository[domain.Role] {
	r := &repository[domain.Role, rolePO]{db: db, tenantScoped: true, missing: domain.ErrRoleNotFound, order: "sort ASC, id ASC", toPO: func(v domain.Role) rolePO {
		return rolePO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, TenantID: v.TenantID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status, Type: v.Type, DataScope: v.DataScope, DataScopeDeptIDs: v.DataScopeDeptIDs}
	}, toDomain: func(p rolePO) domain.Role {
		return domain.Role{ID: p.ID, CreateTime: p.CreateTime, TenantID: p.TenantID, Name: p.Name, Code: p.Code, Remark: p.Remark, Sort: p.Sort, Status: p.Status, Type: p.Type, DataScope: p.DataScope, DataScopeDeptIDs: p.DataScopeDeptIDs}
	}, base: func(p *rolePO) *dbx.BasePO { return &p.BasePO }, active: func(p *rolePO) { p.ActiveCode = active(p.Code) }, clearActive: func(p *rolePO) { p.ActiveCode = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		if q.Code != "" {
			db = db.Where("code LIKE ?", "%"+q.Code+"%")
		}
		return db
	}}
	return r
}
func NewMenuRepository(db *gorm.DB) app.Repository[domain.Menu] {
	r := &repository[domain.Menu, menuPO]{db: db, tenantScoped: false, missing: domain.ErrMenuNotFound, order: "sort ASC, id ASC", toPO: func(v domain.Menu) menuPO {
		return menuPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, ParentID: v.ParentID, Name: v.Name, Permission: v.Permission, Icon: v.Icon, Path: v.Path, Component: v.Component, ComponentName: v.ComponentName, Type: v.Type, Sort: v.Sort, Status: v.Status, Visible: v.Visible, KeepAlive: v.KeepAlive, AlwaysShow: v.AlwaysShow}
	}, toDomain: func(p menuPO) domain.Menu {
		return domain.Menu{ID: p.ID, CreateTime: p.CreateTime, ParentID: p.ParentID, Name: p.Name, Permission: p.Permission, Icon: p.Icon, Path: p.Path, Component: p.Component, ComponentName: p.ComponentName, Type: p.Type, Sort: p.Sort, Status: p.Status, Visible: p.Visible, KeepAlive: p.KeepAlive, AlwaysShow: p.AlwaysShow}
	}, base: func(p *menuPO) *dbx.BasePO { return &p.BasePO }, active: func(p *menuPO) {}, clearActive: func(p *menuPO) {}, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		return db
	}}
	return r
}
func NewTenantRepository(db *gorm.DB) app.Repository[domain.Tenant] {
	r := &repository[domain.Tenant, tenantPO]{db: db, tenantScoped: false, missing: domain.ErrTenantNotFound, toPO: func(v domain.Tenant) tenantPO {
		return tenantPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, ContactUserID: v.ContactUserID, PackageID: v.PackageID, Name: v.Name, ContactName: v.ContactName, ContactMobile: v.ContactMobile, Status: v.Status, AccountCount: v.AccountCount, Websites: v.Websites, ExpireTime: v.ExpireTime}
	}, toDomain: func(p tenantPO) domain.Tenant {
		return domain.Tenant{ID: p.ID, CreateTime: p.CreateTime, ContactUserID: p.ContactUserID, PackageID: p.PackageID, Name: p.Name, ContactName: p.ContactName, ContactMobile: p.ContactMobile, Status: p.Status, AccountCount: p.AccountCount, Websites: p.Websites, ExpireTime: p.ExpireTime}
	}, base: func(p *tenantPO) *dbx.BasePO { return &p.BasePO }, active: func(p *tenantPO) { p.ActiveName = active(p.Name) }, clearActive: func(p *tenantPO) { p.ActiveName = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		if q.PackageID != nil {
			db = db.Where("package_id = ?", *q.PackageID)
		}
		return db
	}}
	return r
}
func NewPackageRepository(db *gorm.DB) app.Repository[domain.TenantPackage] {
	r := &repository[domain.TenantPackage, packagePO]{db: db, tenantScoped: false, missing: domain.ErrPackageNotFound, toPO: func(v domain.TenantPackage) packagePO {
		return packagePO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, Name: v.Name, Remark: v.Remark, Status: v.Status, MenuIDs: v.MenuIDs}
	}, toDomain: func(p packagePO) domain.TenantPackage {
		return domain.TenantPackage{ID: p.ID, CreateTime: p.CreateTime, Name: p.Name, Remark: p.Remark, Status: p.Status, MenuIDs: p.MenuIDs}
	}, base: func(p *packagePO) *dbx.BasePO { return &p.BasePO }, active: func(p *packagePO) { p.ActiveName = active(p.Name) }, clearActive: func(p *packagePO) { p.ActiveName = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		return db
	}}
	return r
}
func NewClientRepository(db *gorm.DB) app.Repository[domain.OAuth2Client] {
	r := &repository[domain.OAuth2Client, clientPO]{db: db, tenantScoped: false, missing: domain.ErrClientNotFound, toPO: func(v domain.OAuth2Client) clientPO {
		return clientPO{BasePO: dbx.BasePO{ID: v.ID, CreateTime: v.CreateTime}, ClientID: v.ClientID, Secret: v.Secret, Name: v.Name, Logo: v.Logo, Description: v.Description, Status: v.Status, AccessTokenValiditySeconds: v.AccessTokenValiditySeconds, RefreshTokenValiditySeconds: v.RefreshTokenValiditySeconds, RedirectURIs: v.RedirectURIs, AuthorizedGrantTypes: v.AuthorizedGrantTypes, Scopes: v.Scopes, AutoApproveScopes: v.AutoApproveScopes, Authorities: v.Authorities, AdditionalInformation: v.AdditionalInformation}
	}, toDomain: func(p clientPO) domain.OAuth2Client {
		return domain.OAuth2Client{ID: p.ID, CreateTime: p.CreateTime, ClientID: p.ClientID, Secret: p.Secret, Name: p.Name, Logo: p.Logo, Description: p.Description, Status: p.Status, AccessTokenValiditySeconds: p.AccessTokenValiditySeconds, RefreshTokenValiditySeconds: p.RefreshTokenValiditySeconds, RedirectURIs: p.RedirectURIs, AuthorizedGrantTypes: p.AuthorizedGrantTypes, Scopes: p.Scopes, AutoApproveScopes: p.AutoApproveScopes, Authorities: p.Authorities, AdditionalInformation: p.AdditionalInformation}
	}, base: func(p *clientPO) *dbx.BasePO { return &p.BasePO }, active: func(p *clientPO) { p.ActiveClientID = active(p.ClientID) }, clearActive: func(p *clientPO) { p.ActiveClientID = nil }, filter: func(db *gorm.DB, q app.Query) *gorm.DB {
		if q.Name != "" {
			db = db.Where("name LIKE ?", "%"+q.Name+"%")
		}
		if q.ClientID != "" {
			db = db.Where("client_id = ?", q.ClientID)
		}
		return db
	}}
	return r
}
