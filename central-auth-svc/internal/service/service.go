package service

import "context"

type CentralAuthSvcConfig struct {
}

type CentralAuthSvc struct {
	RealmStore RealmStore
}

func (cas *CentralAuthSvc) NewUserRealmEvent(ctx context.Context, username, realm_id string) error {
	return cas.RealmStore.SetRealm(ctx, username, realm_id)
}

func (cas *CentralAuthSvc) FindRealm(ctx context.Context, username string) (string, error) {
	return cas.RealmStore.GetRealm(ctx, username)
}
