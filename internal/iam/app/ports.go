// Package app orchestrates IAM use cases through consumer-defined ports.
package app

import (
	"context"
	"time"

	"oneweekonething.com/owot-pro/internal/iam/domain"
	"oneweekonething.com/owot-pro/internal/platform/pagination"
)

type TxRunner interface {
	WithinTx(context.Context, func(context.Context) error) error
}
type OutboxPublisher interface {
	Publish(context.Context, string, []byte) error
}
type PasswordHasher interface {
	Hash(string) (string, error)
	Compare(string, string) error
}

// Query contains the filters shared by the IAM management screens. Repositories
// only apply the fields belonging to their aggregate; exact matches are explicit.
type Query struct {
	pagination.PageQuery
	Name, Code, Username, ExactUsername, Mobile, ClientID string
	Status                                                *int
	DeptIDs, IDs                                          []int64
	PackageID                                             *int64
	UserID                                                *int64
	UserType                                              *int
	From, To                                              *time.Time
}

type Repository[T any] interface {
	Get(context.Context, int64) (T, error)
	List(context.Context, Query) (pagination.PageResult[T], error)
	Create(context.Context, *T) error
	Save(context.Context, *T) error
	Delete(context.Context, int64) error
}

type MutationLocker interface {
	LockTenant(context.Context, int64) error
	LockGlobal(context.Context) error
}

type AssociationRepository interface {
	UserRoles(context.Context, int64) ([]int64, error)
	RoleMenus(context.Context, int64) ([]int64, error)
	AssignUserRoles(context.Context, int64, []int64) error
	AssignRoleMenus(context.Context, int64, []int64) error
	RemoveReferences(context.Context, string, int64) error
}

type TokenSelection struct {
	UserID                    int64
	AccessToken, RefreshToken string
}
type AccessSelector struct {
	ID    int64
	Token string
}
type OAuth2Repository interface {
	FindAccess(context.Context, AccessSelector) (domain.AccessToken, error)
	FindRefresh(context.Context, string) (domain.RefreshToken, error)
	SaveTokens(context.Context, *domain.AccessToken, *domain.RefreshToken) error
	Revoke(context.Context, TokenSelection) ([]domain.AccessToken, error)
	PageTokens(context.Context, Query) (pagination.PageResult[domain.AccessToken], error)
}
type TokenCache interface {
	Get(context.Context, string) (domain.AccessToken, error)
	Set(context.Context, domain.AccessToken) error
	Delete(context.Context, []string) error
	// DeleteAfterCommit synchronously evicts sessions only if the SQL transaction commits.
	DeleteAfterCommit(context.Context, []string) error
}

type PermissionSnapshot struct {
	Roles []domain.Role
	Menus []domain.Menu
	Depts []domain.Dept
	User  domain.AdminUser
}
type PermissionCache interface {
	Load(context.Context, int64, func(context.Context) (PermissionSnapshot, error)) (PermissionSnapshot, error)
}

type LoginLog struct {
	UserID                      int64
	Username, UserIP, UserAgent string
	LogType, Result             int
}
type LoginLogRecorder interface {
	RecordLogin(context.Context, LoginLog) error
}
type AvatarUploader interface {
	Upload(context.Context, string, []byte) (string, error)
}
type DictTranslator interface {
	Value(context.Context, string, string) (string, error)
	Label(context.Context, string, string) (string, error)
}

type Repositories struct {
	Users    Repository[domain.AdminUser]
	Depts    Repository[domain.Dept]
	Posts    Repository[domain.Post]
	Roles    Repository[domain.Role]
	Menus    Repository[domain.Menu]
	Tenants  Repository[domain.Tenant]
	Packages Repository[domain.TenantPackage]
	Clients  Repository[domain.OAuth2Client]
	Links    AssociationRepository
	Tokens   OAuth2Repository
	Locks    MutationLocker
}

type Dependencies struct {
	Repositories
	Tx          TxRunner
	Outbox      OutboxPublisher
	Hasher      PasswordHasher
	TokenCache  TokenCache
	Permissions PermissionCache
	LoginLogs   LoginLogRecorder
	Avatars     AvatarUploader
}
