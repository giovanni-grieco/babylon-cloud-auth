package service

import (
	"auth-svc/internal/domain"
	"context"
)

type UserStore interface {
	ListUsernames(ctx context.Context) ([]string, error)
	GetUser(ctx context.Context, username string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateUser(ctx context.Context, user domain.User) error
	UpdateUser(ctx context.Context, user domain.User) error
	DeleteUser(ctx context.Context, username string) error
}

type SessionStore interface {
	CreateSession(ctx context.Context, sessionID string, userID uint64) error
	GetSession(ctx context.Context, sessionID string) (uint64, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

type BloomPublisher interface {
	Publish(ctx context.Context) error
}
