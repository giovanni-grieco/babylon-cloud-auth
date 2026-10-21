package httpin

import (
	"context"
	"time"
)

type StateStore interface {
	SetState(ctx context.Context, key string, value string, expiration time.Duration) error
	GetState(ctx context.Context, key string) (string, error)
	DeleteState(ctx context.Context, key string) error
}
