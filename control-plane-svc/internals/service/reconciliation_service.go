package service

import (
	"control-plane-svc/internals/clients"
	"time"
)

type ReconciliationService struct {
	ReconcilationPeriod time.Duration
	KeycloakClients     []clients.KeycloakClient
	UserStore           UserStore
}

func NewReconciliationService() *ReconciliationService {
	return &ReconciliationService{}
}

func (rs *ReconciliationService) Reconcile() error {
	return nil
}
