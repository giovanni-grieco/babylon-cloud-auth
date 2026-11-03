// Package app holds the provisioning use cases. It talks to the outside world
// only through the ports below; adapters implement them.
package app

import (
	"context"

	"control-plane-svc/internal/domain"
)

// IdentityStore is a DC realm that owns user accounts.
type IdentityStore interface {
	Groups(ctx context.Context, realm domain.RealmID) ([]string, error)
	// CreateUser creates an enabled user who must change tempPassword at first
	// login, and returns the user's ID. It returns domain.ErrConflict when the
	// username or email is taken.
	CreateUser(ctx context.Context, realm domain.RealmID, u domain.NewUser, tempPassword string) (string, error)
	ListUsers(ctx context.Context, realm domain.RealmID, search string, first, max int) ([]domain.User, error)
	// User returns domain.ErrNotFound when the user doesn't exist.
	User(ctx context.Context, realm domain.RealmID, id string) (domain.User, error)
	// DeleteUser returns domain.ErrNotFound when the user doesn't exist.
	DeleteUser(ctx context.Context, realm domain.RealmID, id string) error
}

// Broker is the central Keycloak, which routes logins to DC realms.
type Broker interface {
	// Domains returns the lowercased email domains that the broker routes to realm.
	Domains(ctx context.Context, realm domain.RealmID) ([]string, error)
	// RemoveBrokeredUser deletes the broker's copy of a DC user, which also ends
	// that user's sessions at the broker. A user who never signed in has no copy,
	// and that isn't an error.
	RemoveBrokeredUser(ctx context.Context, realm domain.RealmID, dcUserID string) error
}

// Passwords generates initial passwords.
type Passwords interface {
	New() (string, error)
}
