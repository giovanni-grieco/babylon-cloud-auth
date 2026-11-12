package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"errors"
	"fmt"
	"log/slog"
	"slices"
)

type ControlPlaneService struct {
	userStore             UserStore
	realm2KeycloakClients map[string]*clients.KeycloakClient
}

func NewControlPlanceService(userStore UserStore, realm2KeyloakClients map[string]*clients.KeycloakClient) *ControlPlaneService {
	return &ControlPlaneService{userStore: userStore, realm2KeycloakClients: realm2KeyloakClients}
}

func (cps *ControlPlaneService) DiscoverRealm(ctx context.Context, userHint string) (string, error) {
	userHint = normalizeIdentifier(userHint)
	slog.InfoContext(ctx, fmt.Sprintf("discovering realm for user: %s", userHint))
	if user, err := cps.userStore.FindByUsername(ctx, userHint); err == nil {
		slog.InfoContext(ctx, fmt.Sprintf("found realm for user: %s - %s", userHint, user.RealmAlias))
		return user.RealmAlias, nil
	} else if user, err := cps.userStore.FindByEmail(ctx, userHint); err == nil {
		slog.InfoContext(ctx, fmt.Sprintf("found realm for user: %s - %s", userHint, user.RealmAlias))
		return user.RealmAlias, nil
	}
	slog.InfoContext(ctx, fmt.Sprintf("realm not found for user: %s", userHint))
	return "", fmt.Errorf("realm not found for user hint: %s", userHint)
}

// Realms returns the aliases of the realms users can be created in, sorted.
func (cps *ControlPlaneService) Realms() []string {
	realms := make([]string, 0, len(cps.realm2KeycloakClients))
	for alias := range cps.realm2KeycloakClients {
		realms = append(realms, alias)
	}
	slices.Sort(realms)
	return realms
}

// ListUsers returns one page of users ordered by realm, then username.
func (cps *ControlPlaneService) ListUsers(ctx context.Context, offset, limit int) ([]*domain.User, error) {
	return cps.userStore.List(ctx, offset, limit)
}

// GetUser returns the user, or an error wrapping ErrNotFound.
func (cps *ControlPlaneService) GetUser(ctx context.Context, id string) (*domain.User, error) {
	return cps.userStore.Find(ctx, id)
}

// RegistrationResult is what the operator gets back from RegisterUser.
// TemporaryPassword is empty when the creation was left to the reconciler:
// once the user is CREATED, the operator obtains one with ResetTemporaryPassword.
type RegistrationResult struct {
	User              *domain.User
	TemporaryPassword string
}

// RegisterUser reserves the identifiers, then creates the user in Keycloak.
//
//  1. INSERT the row as PENDING_CREATION. The UNIQUE constraints on username
//     and email make this a global reservation across realms: a taken
//     identifier is rejected here, before Keycloak is called.
//  2. Create the user in Keycloak with a fresh random temporary password,
//     which is never stored.
//  3. Mark the row CREATED.
//
// Failures:
//   - permanent Keycloak error (4xx): the reservation is released.
//   - transient Keycloak error (timeout, 5xx): the row stays PENDING_CREATION
//     and ErrOperationPending is returned; the reconciler finishes the job.
//   - crash or DB error after step 2: the row stays PENDING_CREATION; the
//     reconciler finds the user in Keycloak and marks it CREATED.
func (cps *ControlPlaneService) RegisterUser(ctx context.Context, username, email, realmAlias string) (*RegistrationResult, error) {
	username = normalizeIdentifier(username)
	email = normalizeIdentifier(email)
	if username == "" || email == "" {
		return nil, fmt.Errorf("username and email are required")
	}
	slog.InfoContext(ctx, "registering user", "username", username, "realm", realmAlias)

	keycloak, ok := cps.realm2KeycloakClients[realmAlias]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownRealm, realmAlias)
	}

	temporaryPassword, err := GenerateTemporaryPassword()
	if err != nil {
		return nil, err
	}

	// 1. Reserve.
	user, err := cps.userStore.Save(ctx, &domain.User{
		Username:   username,
		Email:      email,
		RealmAlias: realmAlias,
		State:      domain.PENDING_CREATION,
	})
	if err != nil {
		return nil, err
	}

	opCtx, cancel := keycloakOpContext(ctx)
	defer cancel()

	// 2. Execute.
	keycloakID, err := createOrAdoptWithPassword(opCtx, keycloak, user, temporaryPassword)
	if err != nil {
		if clients.IsTransient(err) {
			slog.WarnContext(ctx, "keycloak creation failed transiently, left to the reconciler", "user_id", user.ID, "error", err)
			return &RegistrationResult{User: user}, fmt.Errorf("%w: %w", ErrOperationPending, err)
		}
		if releaseErr := cps.userStore.Delete(opCtx, user.ID); releaseErr != nil {
			// The reconciler will retry the creation, hit the same error and mark it FAILED_CREATION.
			slog.ErrorContext(ctx, "could not release reservation", "user_id", user.ID, "error", releaseErr)
		}
		return nil, err
	}

	// 3. Confirm.
	user.RealmID = keycloakID
	user.State = domain.CREATED
	updated, err := cps.userStore.Update(opCtx, user)
	if err != nil {
		// The user exists in Keycloak with this password: hand it over anyway.
		// The reconciler adopts the user without touching the password.
		slog.WarnContext(ctx, "user created in keycloak but state update failed, left to the reconciler", "user_id", user.ID, "error", err)
		user.State = domain.PENDING_CREATION
		return &RegistrationResult{User: user, TemporaryPassword: temporaryPassword}, nil
	}

	slog.InfoContext(ctx, "user registered", "user_id", updated.ID, "realm", realmAlias)
	return &RegistrationResult{User: updated, TemporaryPassword: temporaryPassword}, nil
}

func (cps *ControlPlaneService) UpdateUser(ctx context.Context, username, email string) error {
	// TODO: Implement user update logic
	slog.InfoContext(ctx, fmt.Sprintf("updating user: %s - %s", username, email))
	return nil
}

// DeleteUser records the intent (PENDING_DELETION), deletes the user from
// Keycloak, then removes the row, which frees the identifiers. The row goes
// away only after Keycloak, so nobody can reserve the identifiers while the
// Keycloak user still exists.
//
// Failures:
//   - transient Keycloak error: the row stays PENDING_DELETION and
//     ErrOperationPending is returned; the reconciler finishes the job.
//   - permanent Keycloak error: the row goes to FAILED_DELETION.
//   - crash or DB error after Keycloak: the reconciler retries the (repeatable)
//     Keycloak delete and removes the row.
func (cps *ControlPlaneService) DeleteUser(ctx context.Context, id string) error {
	slog.InfoContext(ctx, "deleting user", "user_id", id)

	var user *domain.User
	err := cps.userStore.InTx(ctx, func(ctx context.Context, tx UserStore) error {
		current, err := tx.FindForUpdate(ctx, id)
		if err != nil {
			return err
		}
		switch current.State {
		case domain.CREATED, domain.FAILED_CREATION, domain.FAILED_DELETION:
		case domain.PENDING_DELETION:
			return fmt.Errorf("%w: deletion already in progress", ErrOperationPending)
		default:
			return fmt.Errorf("%w: STATE=%s", ErrInvalidState, domain.StateToString(current.State))
		}
		if _, ok := cps.realm2KeycloakClients[current.RealmAlias]; !ok {
			return fmt.Errorf("%w: %q", ErrUnknownRealm, current.RealmAlias)
		}

		current.State = domain.PENDING_DELETION
		user, err = tx.Update(ctx, current)
		return err
	})
	if err != nil {
		return err
	}

	opCtx, cancel := keycloakOpContext(ctx)
	defer cancel()

	if err := removeKeycloakUser(opCtx, cps.realm2KeycloakClients[user.RealmAlias], user); err != nil {
		if clients.IsTransient(err) {
			slog.WarnContext(ctx, "keycloak deletion failed transiently, left to the reconciler", "user_id", id, "error", err)
			return fmt.Errorf("%w: %w", ErrOperationPending, err)
		}
		user.State = domain.FAILED_DELETION
		if _, updateErr := cps.userStore.Update(opCtx, user); updateErr != nil {
			slog.ErrorContext(ctx, "could not mark user FAILED_DELETION", "user_id", id, "error", updateErr)
		}
		return err
	}

	if err := cps.userStore.Delete(opCtx, id); err != nil {
		slog.WarnContext(ctx, "user deleted in keycloak but row removal failed, left to the reconciler", "user_id", id, "error", err)
		return nil
	}
	slog.InfoContext(ctx, "user deleted", "user_id", id)
	return nil
}

// ResetTemporaryPassword gives a CREATED user a new random temporary password
// and returns it. Operators use it for users whose creation was completed by
// the reconciler (nobody saw their first password) or who lost it.
func (cps *ControlPlaneService) ResetTemporaryPassword(ctx context.Context, id string) (string, error) {
	user, err := cps.userStore.Find(ctx, id)
	if err != nil {
		return "", err
	}
	if user.State != domain.CREATED {
		return "", fmt.Errorf("%w: STATE=%s", ErrInvalidState, domain.StateToString(user.State))
	}
	keycloak, ok := cps.realm2KeycloakClients[user.RealmAlias]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownRealm, user.RealmAlias)
	}
	if user.RealmID == "" {
		return "", errors.New("user has no keycloak id yet: wait for the next reconciliation")
	}

	temporaryPassword, err := GenerateTemporaryPassword()
	if err != nil {
		return "", err
	}

	opCtx, cancel := keycloakOpContext(ctx)
	defer cancel()
	if err := keycloak.ResetTemporaryPassword(opCtx, user.RealmID, temporaryPassword); err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "temporary password reset", "user_id", id)
	return temporaryPassword, nil
}
