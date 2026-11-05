package domain

import "errors"

var (
	ErrInvalidArgument     = errors.New("infra: invalid argument")
	ErrConfigNotFound      = errors.New("infra: configuration not found")
	ErrConfigKeyExists     = errors.New("infra: configuration key already exists")
	ErrBuiltinConfigDelete = errors.New("infra: cannot delete a built-in configuration")
	ErrConfigHidden        = errors.New("infra: configuration is not visible")
)
