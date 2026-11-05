package http

import (
	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/timeutil"
)

type userDTO struct {
	ID         int64               `json:"id"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	TenantID   int64               `json:"tenantId"`
	Username   string              `json:"username"`
	Nickname   string              `json:"nickname"`
	DeptID     int64               `json:"deptId"`
	PostIDs    []int64             `json:"postIds"`
	Email      string              `json:"email"`
	Mobile     string              `json:"mobile"`
	Sex        int                 `json:"sex"`
	Status     int                 `json:"status"`
	Avatar     string              `json:"avatar"`
	Remark     string              `json:"remark"`
	LoginIP    string              `json:"loginIp"`
	LoginDate  timeutil.MillisTime `json:"loginDate"`
}

func userResponse(v domain.AdminUser) userDTO {
	return userDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, TenantID: v.TenantID, Username: v.Username, Nickname: v.Nickname, DeptID: v.DeptID, PostIDs: nonNil(v.PostIDs), Email: v.Email, Mobile: v.Mobile, Sex: v.Sex, Status: v.Status, Avatar: v.Avatar, Remark: v.Remark, LoginIP: v.LoginIP, LoginDate: timeutil.MillisTime{Time: v.LoginDate}}
}
func (v userDTO) entity() domain.AdminUser {
	return domain.AdminUser{ID: v.ID, Username: v.Username, Nickname: v.Nickname, DeptID: v.DeptID, PostIDs: v.PostIDs, Email: v.Email, Mobile: v.Mobile, Sex: v.Sex, Status: v.Status, Avatar: v.Avatar, Remark: v.Remark, LoginIP: v.LoginIP, LoginDate: v.LoginDate.Time}
}

type deptDTO struct {
	ID           int64               `json:"id"`
	CreateTime   timeutil.MillisTime `json:"createTime"`
	TenantID     int64               `json:"tenantId"`
	ParentID     int64               `json:"parentId"`
	LeaderUserID int64               `json:"leaderUserId"`
	Name         string              `json:"name"`
	Phone        string              `json:"phone"`
	Email        string              `json:"email"`
	Sort         int                 `json:"sort"`
	Status       int                 `json:"status"`
}

func deptResponse(v domain.Dept) deptDTO {
	return deptDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, TenantID: v.TenantID, ParentID: v.ParentID, LeaderUserID: v.LeaderUserID, Name: v.Name, Phone: v.Phone, Email: v.Email, Sort: v.Sort, Status: v.Status}
}
func (v deptDTO) entity() domain.Dept {
	return domain.Dept{ID: v.ID, ParentID: v.ParentID, LeaderUserID: v.LeaderUserID, Name: v.Name, Phone: v.Phone, Email: v.Email, Sort: v.Sort, Status: v.Status}
}

type postDTO struct {
	ID         int64               `json:"id"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	TenantID   int64               `json:"tenantId"`
	Name       string              `json:"name"`
	Code       string              `json:"code"`
	Remark     string              `json:"remark"`
	Sort       int                 `json:"sort"`
	Status     int                 `json:"status"`
}

func postResponse(v domain.Post) postDTO {
	return postDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, TenantID: v.TenantID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status}
}
func (v postDTO) entity() domain.Post {
	return domain.Post{ID: v.ID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status}
}

type roleDTO struct {
	ID               int64               `json:"id"`
	CreateTime       timeutil.MillisTime `json:"createTime"`
	TenantID         int64               `json:"tenantId"`
	Name             string              `json:"name"`
	Code             string              `json:"code"`
	Remark           string              `json:"remark"`
	Sort             int                 `json:"sort"`
	Status           int                 `json:"status"`
	Type             int                 `json:"type"`
	DataScope        int                 `json:"dataScope"`
	DataScopeDeptIDs []int64             `json:"dataScopeDeptIds"`
}

func roleResponse(v domain.Role) roleDTO {
	return roleDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, TenantID: v.TenantID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status, Type: v.Type, DataScope: v.DataScope, DataScopeDeptIDs: nonNil(v.DataScopeDeptIDs)}
}
func (v roleDTO) entity() domain.Role {
	return domain.Role{ID: v.ID, Name: v.Name, Code: v.Code, Remark: v.Remark, Sort: v.Sort, Status: v.Status, Type: v.Type, DataScope: v.DataScope, DataScopeDeptIDs: v.DataScopeDeptIDs}
}

type menuDTO struct {
	ID            int64               `json:"id"`
	CreateTime    timeutil.MillisTime `json:"createTime"`
	ParentID      int64               `json:"parentId"`
	Name          string              `json:"name"`
	Permission    string              `json:"permission"`
	Icon          string              `json:"icon"`
	Path          string              `json:"path"`
	Component     string              `json:"component"`
	ComponentName string              `json:"componentName"`
	Type          int                 `json:"type"`
	Sort          int                 `json:"sort"`
	Status        int                 `json:"status"`
	Visible       bool                `json:"visible"`
	KeepAlive     bool                `json:"keepAlive"`
	AlwaysShow    bool                `json:"alwaysShow"`
}

func menuResponse(v domain.Menu) menuDTO {
	return menuDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, ParentID: v.ParentID, Name: v.Name, Permission: v.Permission, Icon: v.Icon, Path: v.Path, Component: v.Component, ComponentName: v.ComponentName, Type: v.Type, Sort: v.Sort, Status: v.Status, Visible: v.Visible, KeepAlive: v.KeepAlive, AlwaysShow: v.AlwaysShow}
}
func (v menuDTO) entity() domain.Menu {
	return domain.Menu{ID: v.ID, ParentID: v.ParentID, Name: v.Name, Permission: v.Permission, Icon: v.Icon, Path: v.Path, Component: v.Component, ComponentName: v.ComponentName, Type: v.Type, Sort: v.Sort, Status: v.Status, Visible: v.Visible, KeepAlive: v.KeepAlive, AlwaysShow: v.AlwaysShow}
}

type tenantDTO struct {
	ID            int64               `json:"id"`
	CreateTime    timeutil.MillisTime `json:"createTime"`
	ContactUserID int64               `json:"contactUserId"`
	PackageID     int64               `json:"packageId"`
	Name          string              `json:"name"`
	ContactName   string              `json:"contactName"`
	ContactMobile string              `json:"contactMobile"`
	Status        int                 `json:"status"`
	AccountCount  int                 `json:"accountCount"`
	Websites      []string            `json:"websites"`
	ExpireTime    timeutil.MillisTime `json:"expireTime"`
}

func tenantResponse(v domain.Tenant) tenantDTO {
	return tenantDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, ContactUserID: v.ContactUserID, PackageID: v.PackageID, Name: v.Name, ContactName: v.ContactName, ContactMobile: v.ContactMobile, Status: v.Status, AccountCount: v.AccountCount, Websites: nonNil(v.Websites), ExpireTime: timeutil.MillisTime{Time: v.ExpireTime}}
}
func (v tenantDTO) entity() domain.Tenant {
	return domain.Tenant{ID: v.ID, ContactUserID: v.ContactUserID, PackageID: v.PackageID, Name: v.Name, ContactName: v.ContactName, ContactMobile: v.ContactMobile, Status: v.Status, AccountCount: v.AccountCount, Websites: v.Websites, ExpireTime: v.ExpireTime.Time}
}

type packageDTO struct {
	ID         int64               `json:"id"`
	CreateTime timeutil.MillisTime `json:"createTime"`
	Name       string              `json:"name"`
	Remark     string              `json:"remark"`
	Status     int                 `json:"status"`
	MenuIDs    []int64             `json:"menuIds"`
}

func packageResponse(v domain.TenantPackage) packageDTO {
	return packageDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, Name: v.Name, Remark: v.Remark, Status: v.Status, MenuIDs: nonNil(v.MenuIDs)}
}
func (v packageDTO) entity() domain.TenantPackage {
	return domain.TenantPackage{ID: v.ID, Name: v.Name, Remark: v.Remark, Status: v.Status, MenuIDs: v.MenuIDs}
}

type clientDTO struct {
	ID                          int64               `json:"id"`
	CreateTime                  timeutil.MillisTime `json:"createTime"`
	ClientID                    string              `json:"clientId"`
	Secret                      string              `json:"secret"`
	Name                        string              `json:"name"`
	Logo                        string              `json:"logo"`
	Description                 string              `json:"description"`
	Status                      int                 `json:"status"`
	AccessTokenValiditySeconds  int                 `json:"accessTokenValiditySeconds"`
	RefreshTokenValiditySeconds int                 `json:"refreshTokenValiditySeconds"`
	RedirectURIs                []string            `json:"redirectUris"`
	AuthorizedGrantTypes        []string            `json:"authorizedGrantTypes"`
	Scopes                      []string            `json:"scopes"`
	AutoApproveScopes           []string            `json:"autoApproveScopes"`
	Authorities                 []string            `json:"authorities"`
	AdditionalInformation       string              `json:"additionalInformation"`
}

func clientResponse(v domain.OAuth2Client) clientDTO {
	return clientDTO{ID: v.ID, CreateTime: timeutil.MillisTime{Time: v.CreateTime}, ClientID: v.ClientID, Secret: v.Secret, Name: v.Name, Logo: v.Logo, Description: v.Description, Status: v.Status, AccessTokenValiditySeconds: v.AccessTokenValiditySeconds, RefreshTokenValiditySeconds: v.RefreshTokenValiditySeconds, RedirectURIs: nonNil(v.RedirectURIs), AuthorizedGrantTypes: nonNil(v.AuthorizedGrantTypes), Scopes: nonNil(v.Scopes), AutoApproveScopes: nonNil(v.AutoApproveScopes), Authorities: nonNil(v.Authorities), AdditionalInformation: v.AdditionalInformation}
}
func (v clientDTO) entity() domain.OAuth2Client {
	return domain.OAuth2Client{ID: v.ID, ClientID: v.ClientID, Secret: v.Secret, Name: v.Name, Logo: v.Logo, Description: v.Description, Status: v.Status, AccessTokenValiditySeconds: v.AccessTokenValiditySeconds, RefreshTokenValiditySeconds: v.RefreshTokenValiditySeconds, RedirectURIs: v.RedirectURIs, AuthorizedGrantTypes: v.AuthorizedGrantTypes, Scopes: v.Scopes, AutoApproveScopes: v.AutoApproveScopes, Authorities: v.Authorities, AdditionalInformation: v.AdditionalInformation}
}
func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
