package service

import (
	"context"
	"control-plane-svc/internals/domain"
)

type UserStore interface {
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByRealmAlias(ctx context.Context, realmAlias string) ([]*domain.User, error)
	InTx(ctx context.Context, fn func(ctx context.Context, store UserStore) error) error
	Save(ctx context.Context, user *domain.User) error //create and update
	Delete(ctx context.Context, user *domain.User) error
}
