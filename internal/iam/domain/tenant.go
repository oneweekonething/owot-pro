package domain

import "time"

type Tenant struct {
	ID, ContactUserID, PackageID     int64
	Name, ContactName, ContactMobile string
	Status, AccountCount             int
	Websites                         []string
	ExpireTime, CreateTime           time.Time
}

func (t Tenant) Validate() error {
	if !text(t.Name, 30) || !text(t.ContactName, 50) || !optionalText(t.ContactMobile, 30) || !ValidStatus(t.Status) || t.PackageID < 0 || t.AccountCount < 1 || t.ExpireTime.IsZero() {
		return ErrInvalidArgument
	}
	for _, w := range t.Websites {
		if !text(w, 255) {
			return ErrInvalidArgument
		}
	}
	return nil
}
func (t Tenant) Available(now time.Time) error {
	if t.Status != StatusEnabled || !t.ExpireTime.After(now) {
		return ErrTenantUnavailable
	}
	return nil
}
func (t Tenant) CheckQuota(count int64) error {
	if t.PackageID != 0 && count >= int64(t.AccountCount) {
		return ErrQuotaExceeded
	}
	return nil
}

type TenantPackage struct {
	ID           int64
	Name, Remark string
	Status       int
	MenuIDs      []int64
	CreateTime   time.Time
}

func (p TenantPackage) Validate() error {
	if !text(p.Name, 30) || !optionalText(p.Remark, 500) || !ValidStatus(p.Status) || !ValidIDs(p.MenuIDs) {
		return ErrInvalidArgument
	}
	return nil
}
