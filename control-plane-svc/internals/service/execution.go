package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Steps that touch Keycloak, shared by the synchronous API path
// (ControlPlaneService) and the crash-recovery path (ReconciliationService).
// Every step is safe to repeat.

// keycloakOpTimeout bounds every sequence of Keycloak calls. It must stay well
// below the reconciler's staleAfter, so the reconciler never picks up a row
// whose synchronous request is still talking to Keycloak.
const keycloakOpTimeout = 30 * time.Second

var (
	ErrNotFound         = errors.New("user not found")
	ErrIdentifierTaken  = errors.New("username or email already registered")
	ErrUnknownRealm     = errors.New("realm alias not mapped to a keycloak client")
	ErrOperationPending = errors.New("operation not completed yet: the reconciler will finish it")
	ErrInvalidState     = errors.New("operation not allowed in the user's current state")
)

func normalizeIdentifier(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}

// keycloakOpContext detaches the Keycloak calls from the caller's cancellation
// (an HTTP client disconnecting must not interrupt a half-done operation) and
// bounds them with keycloakOpTimeout.
func keycloakOpContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), keycloakOpTimeout)
}

// createOrAdoptWithPassword makes sure the user exists in Keycloak with
// temporaryPassword as its password, and returns the Keycloak id.
// On 409 it adopts the existing user only if username and email both match
// (an earlier attempt got halfway) and resets its password, so the password
// handed to the operator is always valid. Otherwise the identifier belongs to
// someone else in that realm.
func createOrAdoptWithPassword(ctx context.Context, kc *clients.KeycloakClient, user *domain.User, temporaryPassword string) (string, error) {
	keycloakID, err := kc.CreateUser(ctx, user.Username, user.Email, temporaryPassword)
	if !errors.Is(err, clients.ErrConflict) {
		return keycloakID, err
	}

	existing, err := kc.FindUserByUsername(ctx, user.Username)
	if err != nil {
		return "", err
	}
	if existing == nil || !strings.EqualFold(existing.Email, user.Email) {
		return "", fmt.Errorf("%w in realm %s: %w", ErrIdentifierTaken, user.RealmAlias, clients.ErrConflict)
	}
	if err := kc.ResetTemporaryPassword(ctx, existing.RealmID, temporaryPassword); err != nil {
		return "", err
	}
	return existing.RealmID, nil
}

// removeKeycloakUser deletes the user from Keycloak; a user that is already
// gone counts as deleted. Without a stored Keycloak id (the creation never
// completed) it looks the user up by username, and never deletes a Keycloak
// user whose email differs: that one is not ours.
func removeKeycloakUser(ctx context.Context, kc *clients.KeycloakClient, user *domain.User) error {
	keycloakID := user.RealmID
	if keycloakID == "" {
		existing, err := kc.FindUserByUsername(ctx, user.Username)
		if err != nil {
			return err
		}
		if existing == nil || !strings.EqualFold(existing.Email, user.Email) {
			return nil
		}
		keycloakID = existing.RealmID
	}
	return kc.DeleteUser(ctx, keycloakID)
}
