package domain

import "errors"

var (
	ErrInvalidArgument   = errors.New("system: invalid argument")
	ErrDictTypeNotFound  = errors.New("system: dictionary type not found")
	ErrDictTypeExists    = errors.New("system: dictionary type already exists")
	ErrDictTypeImmutable = errors.New("system: dictionary type cannot be changed")
	ErrDictTypeHasData   = errors.New("system: dictionary type contains data")
	ErrDictDataNotFound  = errors.New("system: dictionary data not found")
	ErrDictValueExists   = errors.New("system: dictionary value already exists")
	ErrDictLabelNotFound = errors.New("system: dictionary label not found")
	ErrNoticeNotFound    = errors.New("system: notice not found")
	ErrTenantRequired    = errors.New("system: tenant context required")
)
