// Package persistence implements System repositories with private GORM records.
package persistence

import (
	"fmt"
	"gorm.io/gorm"

	"oneweekonething.com/owot-pro/internal/platform/dbx"
)

type dictTypePO struct {
	dbx.BasePO
	Name       string  `gorm:"size:100;not null"`
	Type       string  `gorm:"size:100;not null;index"`
	ActiveType *string `gorm:"size:100;uniqueIndex:uk_dict_type_active"`
	Status     int     `gorm:"not null;default:0"`
	Remark     string  `gorm:"size:500;not null;default:''"`
}

func (dictTypePO) TableName() string { return "system_dict_type" }

type dictDataPO struct {
	dbx.BasePO
	Sort        int     `gorm:"not null;default:0"`
	Label       string  `gorm:"size:100;not null"`
	Value       string  `gorm:"size:100;not null"`
	DictType    string  `gorm:"size:100;not null;uniqueIndex:uk_dict_data_active,priority:1;index"`
	ActiveValue *string `gorm:"size:100;uniqueIndex:uk_dict_data_active,priority:2"`
	Status      int     `gorm:"not null;default:0"`
	ColorType   string  `gorm:"size:100;not null;default:''"`
	CSSClass    string  `gorm:"column:css_class;size:100;not null;default:''"`
	Remark      string  `gorm:"size:500;not null;default:''"`
}

func (dictDataPO) TableName() string { return "system_dict_data" }

type noticePO struct {
	dbx.TenantBasePO
	Title   string `gorm:"size:50;not null"`
	Type    int    `gorm:"not null"`
	Content string `gorm:"type:longtext;not null"`
	Status  int    `gorm:"not null;default:0"`
}

func (noticePO) TableName() string { return "system_notice" }

type operateLogPO struct {
	dbx.TenantBasePO
	TraceID       string `gorm:"size:64;not null;default:''"`
	UserID        int64  `gorm:"not null;index"`
	UserType      int    `gorm:"not null"`
	Type          string `gorm:"size:50;not null"`
	SubType       string `gorm:"size:50;not null"`
	BizID         int64  `gorm:"not null;default:0"`
	Action        string `gorm:"size:2000;not null;default:''"`
	Extra         string `gorm:"type:longtext"`
	RequestMethod string `gorm:"size:16;not null;default:''"`
	RequestURL    string `gorm:"size:2048;not null;default:''"`
	UserIP        string `gorm:"size:64;not null;default:''"`
	UserAgent     string `gorm:"size:1024;not null;default:''"`
}

func (operateLogPO) TableName() string { return "system_operate_log" }

type loginLogPO struct {
	dbx.TenantBasePO
	LogType   int    `gorm:"not null"`
	TraceID   string `gorm:"size:64;not null;default:''"`
	UserID    int64  `gorm:"not null;default:0;index"`
	UserType  int    `gorm:"not null"`
	Username  string `gorm:"size:100;not null"`
	Result    int    `gorm:"not null"`
	UserIP    string `gorm:"size:64;not null;default:''"`
	UserAgent string `gorm:"size:1024;not null;default:''"`
}

func (loginLogPO) TableName() string { return "system_login_log" }

// Migrate is for explicitly requested development/test initialization.
// Production schema changes are applied from migrations/mysql.
func Migrate(db *gorm.DB) error {
	if db.Name() == "mysql" {
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin")
	}
	return db.AutoMigrate(&dictTypePO{}, &dictDataPO{}, &noticePO{}, &operateLogPO{}, &loginLogPO{})
}

func CheckSchema(db *gorm.DB) error {
	for _, table := range []string{"system_dict_type", "system_dict_data", "system_notice", "system_operate_log", "system_login_log", "sys_outbox_event"} {
		if !db.Migrator().HasTable(table) {
			return fmt.Errorf("system schema table %s is missing; apply migrations/mysql/001_system.sql", table)
		}
	}
	return nil
}
