package domain

import "time"

type Post struct {
	ID, TenantID       int64
	Name, Code, Remark string
	Sort, Status       int
	CreateTime         time.Time
}

func (p Post) Validate() error {
	if !text(p.Name, 50) || !text(p.Code, 100) || !optionalText(p.Remark, 500) || !ValidStatus(p.Status) {
		return ErrInvalidArgument
	}
	return nil
}
