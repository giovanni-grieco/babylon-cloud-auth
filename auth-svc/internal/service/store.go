package service

import (
	"common/domain"
	"context"
)

type UserStore interface {
	GetUser(ctx context.Context, id uint64) (domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateUser(ctx context.Context, user domain.User) error
	UpdateUser(ctx context.Context, user domain.User) error
	DeleteUser(ctx context.Context, username string) error
	// ForEachUsername calls fn for every stored username, stopping at the first error.
	ForEachUsername(ctx context.Context, fn func(username string) error) error
}

type SessionStore interface {
	CreateSession(ctx context.Context, session domain.Session) error
	GetSession(ctx context.Context, id uint64) (domain.Session, error)
	GetSessionByUserID(ctx context.Context, userID uint64) (domain.Session, error)
	GetSessionByRefreshToken(ctx context.Context, refreshToken string) (domain.Session, error)
	DeleteSession(ctx context.Context, id uint64) error
}
