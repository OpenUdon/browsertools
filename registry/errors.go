package registry

import "errors"

// Public registry error categories support stable caller and CLI handling.
var (
	ErrValidation = errors.New("registry validation")
	ErrExpired    = errors.New("registry expiry")
	ErrPolicy     = errors.New("registry policy")
	ErrIntegrity  = errors.New("registry integrity")
	ErrLimit      = errors.New("registry limit")
	ErrConflict   = errors.New("registry conflict")
)
