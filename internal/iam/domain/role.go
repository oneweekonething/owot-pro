package domain

import "time"

const (
	DataScopeAll             = 1
	DataScopeCustom          = 2
	DataScopeDept            = 3
	DataScopeDeptAndChildren = 4
	DataScopeSelf            = 5
)

type Role struct {
	ID, TenantID                  int64
	Name, Code, Remark            string
	Sort, Status, Type, DataScope int
	DataScopeDeptIDs              []int64
	CreateTime                    time.Time
}

func (r Role) BuiltIn() bool { return r.Type == 1 }
func (r Role) Validate() error {
	if !text(r.Name, 30) || !text(r.Code, 100) || !optionalText(r.Remark, 500) || !ValidStatus(r.Status) || r.Type < 1 || r.Type > 2 || r.DataScope < 1 || r.DataScope > 5 || !ValidIDs(r.DataScopeDeptIDs) {
		return ErrInvalidArgument
	}
	return nil
}
func (r Role) ValidateChange(next Role) error {
	if r.BuiltIn() && (r.Code != next.Code || r.Code == "super_admin" && next.Status != StatusEnabled) {
		return ErrProtected
	}
	if !r.BuiltIn() && (next.Code == "super_admin" || next.Code == "tenant_admin") {
		return ErrProtected
	}
	return next.Validate()
}
