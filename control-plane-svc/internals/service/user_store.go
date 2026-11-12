package service

import (
	"context"
	"control-plane-svc/internals/domain"
	"time"
)

type UserStore interface {
	// Find returns an error wrapping ErrNotFound when the user does not exist.
	Find(ctx context.Context, id string) (*domain.User, error)
	// List returns one page of users ordered by realm, then username.
	List(ctx context.Context, offset, limit int) ([]*domain.User, error)
	// FindForUpdate locks the row until the surrounding transaction ends (use inside InTx).
	FindForUpdate(ctx context.Context, id string) (*domain.User, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByRealmAlias(ctx context.Context, realmAlias string) ([]*domain.User, error)
	// FindStale returns users in the given state untouched for olderThan (no lock).
	FindStale(ctx context.Context, state int, olderThan time.Duration, limit int) ([]*domain.User, error)
	// FindForUpdateSkipLocked locks the row (use inside InTx); nil, nil if gone or locked by someone else.
	FindForUpdateSkipLocked(ctx context.Context, id string) (*domain.User, error)
	InTx(ctx context.Context, fn func(ctx context.Context, store UserStore) error) error
	// Save returns an error wrapping ErrIdentifierTaken when username or email already exist.
	Save(ctx context.Context, user *domain.User) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) (*domain.User, error)
	Delete(ctx context.Context, id string) error
}
