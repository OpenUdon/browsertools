package cache

import "errors"

var (
	ErrValidation = errors.New("cache validation failure")
	ErrExpired    = errors.New("cache expiry failure")
	ErrPolicy     = errors.New("cache policy failure")
	ErrIntegrity  = errors.New("cache integrity failure")
	ErrLimit      = errors.New("cache limit failure")
	ErrConflict   = errors.New("cache conflict")
)
