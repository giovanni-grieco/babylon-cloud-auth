package service

import (
	"context"
	"control-plane-svc/internals/domain"
)

type UserStore interface {
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByEmail(ctx context.Context, username string) (*domain.User, error)
}
