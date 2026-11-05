// Package persistence implements IAM repositories with private GORM records.
package persistence

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

type userPO struct {
	dbx.BasePO
	TenantID       int64      `gorm:"not null;index;uniqueIndex:uk_iam_user_username,priority:1;uniqueIndex:uk_iam_user_mobile,priority:1;uniqueIndex:uk_iam_user_email,priority:1"`
	Username       string     `gorm:"size:30;not null"`
	Nickname       string     `gorm:"size:30;not null"`
	PasswordHash   string     `gorm:"column:password;size:100;not null"`
	DeptID         int64      `gorm:"not null;default:0;index"`
	PostIDs        []int64    `gorm:"serializer:json;type:json"`
	Email          string     `gorm:"size:100;not null;default:''"`
	Mobile         string     `gorm:"size:30;not null;default:''"`
	Sex            int        `gorm:"not null;default:0"`
	Status         int        `gorm:"not null;default:0"`
	Avatar         string     `gorm:"size:1024;not null;default:''"`
	Remark         string     `gorm:"size:500;not null;default:''"`
	LoginIP        string     `gorm:"size:50;not null;default:''"`
	LoginDate      *time.Time `gorm:""`
	ActiveUsername *string    `gorm:"size:30;uniqueIndex:uk_iam_user_username,priority:2"`
	ActiveMobile   *string    `gorm:"size:30;uniqueIndex:uk_iam_user_mobile,priority:2"`
	ActiveEmail    *string    `gorm:"size:100;uniqueIndex:uk_iam_user_email,priority:2"`
}

func (userPO) TableName() string { return "system_users" }

type deptPO struct {
	dbx.BasePO
	TenantID     int64  `gorm:"not null;index"`
	ParentID     int64  `gorm:"not null;default:0;index"`
	LeaderUserID int64  `gorm:"not null;default:0"`
	Name         string `gorm:"size:30;not null"`
	Phone        string `gorm:"size:30;not null;default:''"`
	Email        string `gorm:"size:100;not null;default:''"`
	Sort         int    `gorm:"not null;default:0"`
	Status       int    `gorm:"not null;default:0"`
}

func (deptPO) TableName() string { return "system_dept" }

type postPO struct {
	dbx.BasePO
	TenantID   int64   `gorm:"not null;index;uniqueIndex:uk_iam_post_code,priority:1;uniqueIndex:uk_iam_post_name,priority:1"`
	Name       string  `gorm:"size:50;not null"`
	Code       string  `gorm:"size:100;not null"`
	Remark     string  `gorm:"size:500;not null;default:''"`
	Sort       int     `gorm:"not null;default:0"`
	Status     int     `gorm:"not null;default:0"`
	ActiveCode *string `gorm:"size:100;uniqueIndex:uk_iam_post_code,priority:2"`
	ActiveName *string `gorm:"size:50;uniqueIndex:uk_iam_post_name,priority:2"`
}

func (postPO) TableName() string { return "system_post" }

type rolePO struct {
	dbx.BasePO
	TenantID         int64   `gorm:"not null;index;uniqueIndex:uk_iam_role_code,priority:1"`
	Name             string  `gorm:"size:30;not null"`
	Code             string  `gorm:"size:100;not null"`
	Remark           string  `gorm:"size:500;not null;default:''"`
	Sort             int     `gorm:"not null;default:0"`
	Status           int     `gorm:"not null;default:0"`
	Type             int     `gorm:"not null;default:0"`
	DataScope        int     `gorm:"not null;default:0"`
	DataScopeDeptIDs []int64 `gorm:"serializer:json;type:json"`
	ActiveCode       *string `gorm:"size:100;uniqueIndex:uk_iam_role_code,priority:2"`
}

func (rolePO) TableName() string { return "system_role" }

type menuPO struct {
	dbx.BasePO
	ParentID      int64  `gorm:"not null;default:0;index"`
	Name          string `gorm:"size:50;not null"`
	Permission    string `gorm:"size:100;index;not null;default:''"`
	Icon          string `gorm:"size:100;not null;default:''"`
	Path          string `gorm:"size:255;not null;default:''"`
	Component     string `gorm:"size:255;not null;default:''"`
	ComponentName string `gorm:"size:100;not null;default:''"`
	Type          int    `gorm:"not null;default:0"`
	Sort          int    `gorm:"not null;default:0"`
	Status        int    `gorm:"not null;default:0"`
	Visible       bool   `gorm:"not null;default:0"`
	KeepAlive     bool   `gorm:"not null;default:0"`
	AlwaysShow    bool   `gorm:"not null;default:0"`
}

func (menuPO) TableName() string { return "system_menu" }

type tenantPO struct {
	dbx.BasePO
	ContactUserID int64     `gorm:"not null;default:0"`
	PackageID     int64     `gorm:"not null;default:0;index"`
	Name          string    `gorm:"size:30;not null"`
	ContactName   string    `gorm:"size:50;not null;default:''"`
	ContactMobile string    `gorm:"size:30;not null;default:''"`
	Status        int       `gorm:"not null;default:0"`
	AccountCount  int       `gorm:"not null;default:0"`
	Websites      []string  `gorm:"serializer:json;type:json"`
	ExpireTime    time.Time `gorm:"not null"`
	ActiveName    *string   `gorm:"size:30;uniqueIndex:uk_iam_tenant_name,priority:2"`
}

func (tenantPO) TableName() string { return "system_tenant" }

type packagePO struct {
	dbx.BasePO
	Name       string  `gorm:"size:30;not null"`
	Remark     string  `gorm:"size:500;not null;default:''"`
	Status     int     `gorm:"not null;default:0"`
	MenuIDs    []int64 `gorm:"serializer:json;type:json"`
	ActiveName *string `gorm:"size:30;uniqueIndex:uk_iam_package_name,priority:2"`
}

func (packagePO) TableName() string { return "system_tenant_package" }

type clientPO struct {
	dbx.BasePO
	ClientID                    string   `gorm:"size:255;not null"`
	Secret                      string   `gorm:"size:255;not null"`
	Name                        string   `gorm:"size:255;not null"`
	Logo                        string   `gorm:"size:255;not null;default:''"`
	Description                 string   `gorm:"size:500;not null;default:''"`
	Status                      int      `gorm:"not null;default:0"`
	AccessTokenValiditySeconds  int      `gorm:"not null;default:0"`
	RefreshTokenValiditySeconds int      `gorm:"not null;default:0"`
	RedirectURIs                []string `gorm:"serializer:json;type:json"`
	AuthorizedGrantTypes        []string `gorm:"serializer:json;type:json"`
	Scopes                      []string `gorm:"serializer:json;type:json"`
	AutoApproveScopes           []string `gorm:"serializer:json;type:json"`
	Authorities                 []string `gorm:"serializer:json;type:json"`
	AdditionalInformation       string   `gorm:"type:text"`
	ActiveClientID              *string  `gorm:"size:255;uniqueIndex:uk_iam_client_clientid,priority:2"`
}

func (clientPO) TableName() string { return "system_oauth2_client" }
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
func timeValue(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
func modelList() []any {
	return []any{&userPO{}, &deptPO{}, &postPO{}, &rolePO{}, &menuPO{}, &tenantPO{}, &packagePO{}, &clientPO{}, &userRolePO{}, &roleMenuPO{}, &userPostPO{}, &accessTokenPO{}, &refreshTokenPO{}}
}
func Migrate(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(modelList()...)
}
func CheckSchema(db *gorm.DB) error {
	for _, m := range modelList() {
		if !db.Migrator().HasTable(m) {
			return fmt.Errorf("IAM schema missing; run migrations/mysql/002_iam.sql or -migrate")
		}
	}
	return nil
}
