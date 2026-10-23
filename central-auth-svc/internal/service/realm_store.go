package service

import "context"

type RealmStore interface {
	GetRealm(ctx context.Context, username string) (string, error)
	SetRealm(ctx context.Context, username, realm_id string) error
}
