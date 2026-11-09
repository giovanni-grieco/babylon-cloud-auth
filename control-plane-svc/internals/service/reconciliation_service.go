package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"fmt"
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
	fmt.Println("Starting reconciliation process...")
	for _, keycloakClient := range rs.KeycloakClients {
		fmt.Println("Reconciling users for realm: ", keycloakClient.RealmAlias)
		users, err := keycloakClient.GetAllUsers()
		if err != nil {
			return err
		}

		for _, user := range users {
			err = rs.UserStore.Save(ctx, &user)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (rs *ReconciliationService) StartLoop(ctx context.Context) {

}
