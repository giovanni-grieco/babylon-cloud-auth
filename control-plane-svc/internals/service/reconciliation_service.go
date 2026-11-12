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

func (rs *ReconciliationService) BootstrapReconciliation(ctx context.Context) error {
	slog.InfoContext(ctx, "Executing bootstrap reconciliation")
	for _, keycloakClient := range rs.KeycloakClients {
		slog.InfoContext(ctx, "reconciling realm users", "realm", keycloakClient.RealmAlias)
		realmUsers, err := keycloakClient.GetAllUsers()
		if err != nil {
			return err
		}
		err = rs.UserStore.InTx(ctx, func(ctx context.Context, txUserStore UserStore) error {
			controlPlaneRealmUsers, err := txUserStore.FindByRealmAlias(ctx, keycloakClient.RealmAlias)
			if err != nil {
				return err
			}
			controlPlaneUserToRemove := make(map[string]*domain.User)
			for _, user := range controlPlaneRealmUsers {
				controlPlaneUserToRemove[user.Username] = user
			}

			for _, user := range realmUsers {
				if _, exists := controlPlaneUserToRemove[user.Username]; exists {
					delete(controlPlaneUserToRemove, user.Username)
				} else {
					user.State = domain.CREATED // WE ASSUME THE USER IS CREATED
					_, err := txUserStore.Save(ctx, &user)
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		slog.InfoContext(ctx, "Bootstrap realm reconciliation completed", "realm", keycloakClient.RealmAlias)
	}
	slog.InfoContext(ctx, "Bootstrap reconciliation completed")
	return nil
}

func (rs *ReconciliationService) reconcileUsersPendingCreation(ctx context.Context) error {
	return nil
}

func (rs *ReconciliationService) reconcileUsersPendingDeletion(ctx context.Context) error {
	return nil
}

// Si assicura di trovare le utenze che stanno in stati transienti e si assicura che viene applicata
func (rs *ReconciliationService) Reconcile(ctx context.Context) error {
	err := rs.reconcileUsersPendingCreation(ctx)
	if err != nil {
		return err
	}
	err = rs.reconcileUsersPendingDeletion(ctx)
	return err
}

func (rs *ReconciliationService) StartLoop(ctx context.Context) {

	go func() {
		ticker := time.NewTicker(rs.ReconcilationPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := rs.BootstrapReconciliation(ctx)
				slog.InfoContext(ctx, "execute reconciliation loop")
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
