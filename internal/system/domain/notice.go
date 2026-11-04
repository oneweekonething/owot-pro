package domain

import "time"

const (
	NoticeTypeNotification = 1
	NoticeTypeAnnouncement = 2
)

type Notice struct {
	ID         int64
	TenantID   int64
	Title      string
	Type       int
	Content    string
	Status     int
	Creator    string
	CreateTime time.Time
	UpdateTime time.Time
}

func (n Notice) Validate() error {
	if !validText(n.Title, 50) || !validText(n.Content, 65535) ||
		!ValidStatus(n.Status) ||
		(n.Type != NoticeTypeNotification && n.Type != NoticeTypeAnnouncement) {
		return ErrInvalidArgument
	}
	return nil
}
