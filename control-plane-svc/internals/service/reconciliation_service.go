package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// reconcileBatchSize caps how many stuck rows of each state one tick handles.
const reconcileBatchSize = 50

type ReconciliationService struct {
	ReconcilationPeriod time.Duration
	// StaleAfter is how long a row must sit in a PENDING_* state before the
	// reconciler takes it over. It must exceed keycloakOpTimeout, so the
	// reconciler never races a request that is still talking to Keycloak.
	StaleAfter            time.Duration
	KeycloakClients       []*clients.KeycloakClient
	UserStore             UserStore
	realm2KeycloakClients map[string]*clients.KeycloakClient
}

func NewReconciliationService(reconciliationPeriod, staleAfter time.Duration, userStore UserStore, keycloakClients []*clients.KeycloakClient) *ReconciliationService {
	realm2KeycloakClients := make(map[string]*clients.KeycloakClient, len(keycloakClients))
	for _, kc := range keycloakClients {
		realm2KeycloakClients[kc.RealmAlias] = kc
	}
	return &ReconciliationService{
		ReconcilationPeriod:   reconciliationPeriod,
		StaleAfter:            staleAfter,
		KeycloakClients:       keycloakClients,
		UserStore:             userStore,
		realm2KeycloakClients: realm2KeycloakClients,
	}
}

// BootstrapReconciliation imports the Keycloak users the control plane does
// not know yet (e.g. created before the control plane existed) as CREATED.
// It never removes rows. A user whose username or email is already owned by
// another realm is not imported: it is logged as a conflict.
func (rs *ReconciliationService) BootstrapReconciliation(ctx context.Context) error {
	slog.InfoContext(ctx, "Executing bootstrap reconciliation")
	var errs []error
	for _, keycloakClient := range rs.KeycloakClients {
		if err := rs.importUnknownUsers(ctx, keycloakClient); err != nil {
			errs = append(errs, fmt.Errorf("realm %s: %w", keycloakClient.RealmAlias, err))
			continue
		}
		slog.InfoContext(ctx, "Bootstrap realm reconciliation completed", "realm", keycloakClient.RealmAlias)
	}
	slog.InfoContext(ctx, "Bootstrap reconciliation completed")
	return errors.Join(errs...)
}

func (rs *ReconciliationService) importUnknownUsers(ctx context.Context, kc *clients.KeycloakClient) error {
	slog.InfoContext(ctx, "reconciling realm users", "realm", kc.RealmAlias)
	realmUsers, err := kc.GetAllUsers()
	if err != nil {
		return err
	}

	controlPlaneRealmUsers, err := rs.UserStore.FindByRealmAlias(ctx, kc.RealmAlias)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(controlPlaneRealmUsers))
	for _, user := range controlPlaneRealmUsers {
		known[normalizeIdentifier(user.Username)] = struct{}{}
	}

	for _, realmUser := range realmUsers {
		if _, exists := known[normalizeIdentifier(realmUser.Username)]; exists {
			continue
		}

		// The list may be stale: the user could have been deleted (e.g. by
		// DeleteUser) after GetAllUsers. Re-check before importing it, or we
		// would put back a row for a user that no longer exists.
		opCtx, cancel := keycloakOpContext(ctx)
		current, err := kc.FindUserByUsername(opCtx, realmUser.Username)
		cancel()
		if err != nil {
			return err
		}
		if current == nil {
			continue
		}

		user := &domain.User{
			RealmID:    current.RealmID,
			Username:   normalizeIdentifier(current.Username),
			Email:      normalizeIdentifier(current.Email),
			RealmAlias: kc.RealmAlias,
			State:      domain.CREATED,
		}
		if _, err := rs.UserStore.Save(ctx, user); err != nil {
			if errors.Is(err, ErrIdentifierTaken) {
				slog.WarnContext(ctx, "keycloak user not imported: username or email already owned by another user",
					"realm", kc.RealmAlias, "username", user.Username)
				continue
			}
			return err
		}
		slog.InfoContext(ctx, "imported keycloak user", "realm", kc.RealmAlias, "username", user.Username)
	}
	return nil
}

// reconcileUsersPendingCreation finishes creations interrupted by a crash or a
// transient Keycloak error. Each user is handled in its own transaction:
//
//	BEGIN; lock row (SKIP LOCKED); ask Keycloak; create if missing; mark CREATED; COMMIT
//
// If anything fails the transaction rolls back, the row stays
// PENDING_CREATION, and the next tick tries again; every step is repeatable.
func (rs *ReconciliationService) reconcileUsersPendingCreation(ctx context.Context) error {
	return rs.reconcilePending(ctx, domain.PENDING_CREATION, domain.FAILED_CREATION, rs.completeCreation)
}

// reconcileUsersPendingDeletion finishes deletions the same way:
//
//	BEGIN; lock row (SKIP LOCKED); delete in Keycloak (404 = done); delete row; COMMIT
func (rs *ReconciliationService) reconcileUsersPendingDeletion(ctx context.Context) error {
	return rs.reconcilePending(ctx, domain.PENDING_DELETION, domain.FAILED_DELETION, rs.completeDeletion)
}

type reconcileStep func(ctx context.Context, tx UserStore, kc *clients.KeycloakClient, user *domain.User) error

func (rs *ReconciliationService) reconcilePending(ctx context.Context, pendingState, failedState int, step reconcileStep) error {

	// it takes a bit for the reconciler to actually enact the pending because it takes "old" enough records
	candidates, err := rs.UserStore.FindStale(ctx, pendingState, rs.StaleAfter, reconcileBatchSize)
	if err != nil {
		return err
	}

	var errs []error
	for _, candidate := range candidates {
		err := rs.UserStore.InTx(ctx, func(ctx context.Context, tx UserStore) error {
			user, err := tx.FindForUpdateSkipLocked(ctx, candidate.ID)
			if err != nil {
				return err
			}
			if user == nil || user.State != pendingState {
				return nil // someone else holds it, or it already moved on
			}

			kc, ok := rs.realm2KeycloakClients[user.RealmAlias]
			if !ok {
				return markFailed(ctx, tx, user, failedState, fmt.Errorf("%w: %q", ErrUnknownRealm, user.RealmAlias))
			}

			opCtx, cancel := keycloakOpContext(ctx)
			defer cancel()
			return step(opCtx, tx, kc, user)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("user %s (%s): %w", candidate.ID, domain.StateToString(pendingState), err))
		}
	}
	return errors.Join(errs...)
}

func (rs *ReconciliationService) completeCreation(ctx context.Context, tx UserStore, kc *clients.KeycloakClient, user *domain.User) error {
	existing, err := kc.FindUserByUsername(ctx, user.Username)
	if err != nil {
		return err
	}

	if existing != nil {
		if !strings.EqualFold(existing.Email, user.Email) {
			return markFailed(ctx, tx, user, domain.FAILED_CREATION,
				fmt.Errorf("%w: username belongs to another keycloak user", ErrIdentifierTaken))
		}
		user.RealmID = existing.RealmID
		slog.InfoContext(ctx, "reconciler: adopted user already present in keycloak", "user_id", user.ID, "realm", user.RealmAlias)
	} else {
		temporaryPassword, err := GenerateTemporaryPassword()
		if err != nil {
			return err
		}
		keycloakID, err := kc.CreateUser(ctx, user.Username, user.Email, temporaryPassword)
		if err != nil {
			if clients.IsTransient(err) {
				return err
			}
			return markFailed(ctx, tx, user, domain.FAILED_CREATION, err)
		}
		user.RealmID = keycloakID
		slog.WarnContext(ctx, "reconciler: created user with a temporary password nobody has seen; reset it before handing the account over",
			"user_id", user.ID, "realm", user.RealmAlias)
	}

	user.State = domain.CREATED
	_, err = tx.Update(ctx, user)
	return err
}

func (rs *ReconciliationService) completeDeletion(ctx context.Context, tx UserStore, kc *clients.KeycloakClient, user *domain.User) error {
	if err := removeKeycloakUser(ctx, kc, user); err != nil {
		if clients.IsTransient(err) {
			return err
		}
		return markFailed(ctx, tx, user, domain.FAILED_DELETION, err)
	}
	if err := tx.Delete(ctx, user.ID); err != nil {
		return err
	}
	slog.InfoContext(ctx, "reconciler: completed deletion", "user_id", user.ID, "realm", user.RealmAlias)
	return nil
}

func markFailed(ctx context.Context, tx UserStore, user *domain.User, failedState int, cause error) error {
	slog.ErrorContext(ctx, "reconciler: permanent failure", "user_id", user.ID, "realm", user.RealmAlias,
		"state", domain.StateToString(failedState), "error", cause)
	user.State = failedState
	_, err := tx.Update(ctx, user)
	return err
}

func (rs *ReconciliationService) Reconcile(ctx context.Context) error {
	return errors.Join(
		rs.reconcileUsersPendingCreation(ctx),
		rs.reconcileUsersPendingDeletion(ctx),
	)
}

func (rs *ReconciliationService) StartLoop(ctx context.Context) {

	go func() {
		ticker := time.NewTicker(rs.ReconcilationPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				slog.InfoContext(ctx, "execute reconciliation loop")
				if err := rs.Reconcile(ctx); err != nil {
					slog.ErrorContext(ctx, "pending-state reconciliation failed", "error", err)
				}
				if err := rs.BootstrapReconciliation(ctx); err != nil {
					slog.ErrorContext(ctx, "keycloak import failed", "error", err)
				}
				slog.InfoContext(ctx, "next reconciliation scheduled", "in", rs.ReconcilationPeriod)
			case <-ctx.Done():
				slog.InfoContext(ctx, "stopping reconciliation loop")
				return
			}
		}
	}()
}
