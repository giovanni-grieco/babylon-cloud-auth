package domain

import "errors"

// Errors that adapters return so the application can react without knowing
// about HTTP status codes.
var (
	ErrUnknownRealm = errors.New("unknown realm")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("a user with this username or email already exists")
)
