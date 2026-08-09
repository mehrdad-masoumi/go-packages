package sharederrors

import "errors"

// Common sentinel errors shared across microservices.
// Service-specific sentinels may live in the owning service.
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrInsufficient  = errors.New("insufficient balance")
	ErrConflict      = errors.New("conflict")
	ErrInvalidState  = errors.New("invalid state")
)
