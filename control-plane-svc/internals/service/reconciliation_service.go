package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"log/slog"
	"time"
)

type ReconciliationService struct {
	ReconcilationPeriod time.Duration
	KeycloakClients     []*clients.KeycloakClient
	UserStore           UserStore
}

func NewReconciliationService(reconciliationPeriod time.Duration, userStore UserStore, keycloakClients []*clients.KeycloakClient) *ReconciliationService {
	return &ReconciliationService{
		ReconcilationPeriod: reconciliationPeriod,
		KeycloakClients:     keycloakClients,
		UserStore:           userStore,
	}
}

func (rs *ReconciliationService) Reconcile(ctx context.Context) error {
	slog.InfoContext(ctx, "starting reconciliation")
	for _, keycloakClient := range rs.KeycloakClients {
		slog.InfoContext(ctx, "reconciling realm users", "realm", keycloakClient.RealmAlias)
		realmUsers, err := keycloakClient.GetAllUsers()
		if err != nil {
			return err
		}

		// Get the users from the control plane for this realm
		err = rs.UserStore.InTx(ctx, func(ctx context.Context, txUserStore UserStore) error {
			controlPlaneRealmUsers, err := txUserStore.FindByRealmAlias(ctx, keycloakClient.RealmAlias)
			if err != nil {
				return err
			}

			// Create a map for quick lookup of control plane users by username
			controlPlaneUserToRemove := make(map[string]*domain.User)
			for _, user := range controlPlaneRealmUsers {
				controlPlaneUserToRemove[user.Username] = user
			}

			for _, user := range realmUsers {
				// Check if the user exists in the control plane
				if _, exists := controlPlaneUserToRemove[user.Username]; exists {
					// User exists, remove from the map to indicate it's still present
					delete(controlPlaneUserToRemove, user.Username)
				} else {
					// User does not exist in the control plane, add it
					err := txUserStore.Save(ctx, &user)
					if err != nil {
						return err
					}
				}
			}

			// Any remaining users in controlPlaneUserToRemove are no longer present in Keycloak and should be removed from the control plane
			for _, userToRemove := range controlPlaneUserToRemove {
				err := txUserStore.Delete(ctx, userToRemove)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		slog.InfoContext(ctx, "realm reconciliation completed", "realm", keycloakClient.RealmAlias)
	}
	slog.InfoContext(ctx, "reconciliation completed")
	return nil
}

func (rs *ReconciliationService) StartLoop(ctx context.Context) {

	go func() {
		ticker := time.NewTicker(rs.ReconcilationPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := rs.Reconcile(ctx)
				if err != nil {
					slog.ErrorContext(ctx, "reconciliation failed", "error", err)
				}
				slog.InfoContext(ctx, "next reconciliation scheduled", "in", rs.ReconcilationPeriod)
			case <-ctx.Done():
				slog.InfoContext(ctx, "stopping reconciliation loop")
				return
			}
		}
	}()
}
