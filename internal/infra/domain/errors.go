package domain

import "errors"

var (
	ErrInvalidArgument     = errors.New("infra: invalid argument")
	ErrConfigNotFound      = errors.New("infra: configuration not found")
	ErrConfigKeyExists     = errors.New("infra: configuration key already exists")
	ErrBuiltinConfigDelete = errors.New("infra: cannot delete a built-in configuration")
	ErrConfigHidden        = errors.New("infra: configuration is not visible")
)

var (
	ErrFileNotFound       = errors.New("infra: file not found")
	ErrFileConfigNotFound = errors.New("infra: file configuration not found")
	ErrFileConfigInUse    = errors.New("infra: file configuration in use")
	ErrFileMasterDelete   = errors.New("infra: cannot delete master file configuration")
	ErrFileTooLarge       = errors.New("infra: file too large")
	ErrFileDirectUpload   = errors.New("infra: direct upload requires S3 storage")
)
