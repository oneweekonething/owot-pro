// Package domain contains IAM entities and rules using only the standard library.
package domain

import "errors"

var (
	ErrInvalidArgument   = errors.New("iam: invalid argument")
	ErrUserNotFound      = errors.New("iam: user not found")
	ErrDeptNotFound      = errors.New("iam: department not found")
	ErrPostNotFound      = errors.New("iam: post not found")
	ErrRoleNotFound      = errors.New("iam: role not found")
	ErrMenuNotFound      = errors.New("iam: menu not found")
	ErrTenantNotFound    = errors.New("iam: tenant not found")
	ErrPackageNotFound   = errors.New("iam: tenant package not found")
	ErrClientNotFound    = errors.New("iam: OAuth2 client not found")
	ErrTokenInvalid      = errors.New("iam: token invalid or expired")
	ErrCredentials       = errors.New("iam: invalid credentials")
	ErrDisabled          = errors.New("iam: account disabled")
	ErrTenantUnavailable = errors.New("iam: tenant disabled or expired")
	ErrQuotaExceeded     = errors.New("iam: tenant account quota exceeded")
	ErrDuplicate         = errors.New("iam: duplicate business identifier")
	ErrProtected         = errors.New("iam: built-in object protected")
	ErrForbidden         = errors.New("iam: forbidden")
	ErrTreeCycle         = errors.New("iam: tree cycle")
	ErrHasChildren       = errors.New("iam: object still referenced")
	ErrPassword          = errors.New("iam: password must contain 4 to 16 non-space characters")
)
