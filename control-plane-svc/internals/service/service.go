package service

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"fmt"
	"log/slog"
)

type ControlPlaneService struct {
	userStore             UserStore
	realm2KeycloakClients map[string]*clients.KeycloakClient
}

func NewControlPlanceService(userStore UserStore, realm2KeyloakClients map[string]*clients.KeycloakClient) *ControlPlaneService {
	return &ControlPlaneService{userStore: userStore, realm2KeycloakClients: realm2KeyloakClients}
}

func (cps *ControlPlaneService) DiscoverRealm(ctx context.Context, userHint string) (string, error) {
	// TODO: Implement realm discovery logic
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

func (cps *ControlPlaneService) RegisterUser(ctx context.Context, username, email, temporaryPassword, realmAlias string) error {
	// TODO: Implement user registration logic
	slog.InfoContext(ctx, fmt.Sprintf("registering user: %s - %s - %s", username, email, realmAlias))
	err := cps.userStore.InTx(ctx, func(ctx context.Context, userStore UserStore) error {
		user := &domain.User{
			Username:   username,
			Email:      email,
			RealmAlias: realmAlias,
			State:      domain.CREATED, // creiamo subito la riga per dire che qualcosa va creato. Se avremo successo, verrà cambiato.
		}

		user, err := cps.userStore.Save(ctx, user)
		if err != nil {
			return err
		}

		// Issue a creation on keycloak
		keycloak, ok := cps.realm2KeycloakClients[realmAlias]
		if !ok {
			return fmt.Errorf("Realm Alias \"%s\" not mapped to an existing Keycloak client", realmAlias)
		}

		err = keycloak.RegisterUser(user)
		if err != nil {
			return fmt.Errorf("Keycloak client err: %v", err)
		}
		// change the user status to created
		user.State = domain.CREATED
		user, err = cps.userStore.Update(ctx, user)
		if err != nil {
			return fmt.Errorf("Failure to update user: %v", err)
		}

		return err
	})
	return err
}

func (cps *ControlPlaneService) UpdateUser(ctx context.Context, username, email string) error {
	// TODO: Implement user update logic
	slog.InfoContext(ctx, fmt.Sprintf("updating user: %s - %s", username, email))
	return nil
}

func (cps *ControlPlaneService) DeleteUser(ctx context.Context, id string) error {
	// TODO: Implement user delete logic
	slog.InfoContext(ctx, fmt.Sprintf("deleting user: %s", id))

	err := cps.userStore.InTx(ctx, func(ctx context.Context, userStore UserStore) error {
		user, err := userStore.Find(ctx, id)
		if err != nil {
			return err
		}

		if user.State != domain.CREATED {
			return fmt.Errorf("Can't delete user that is still in STATE=%s", domain.StateToString(user.State))
		}

		user.State = domain.PENDING_DELETION
		user, err = userStore.Update(ctx, user)
		if err != nil {
			return err
		}

		keycloak, ok := cps.realm2KeycloakClients[user.RealmAlias]
		if !ok {
			return fmt.Errorf("Realm Alias \"%s\" not mapped to an existing Keycloak client", user.RealmAlias)
		}

		err = keycloak.DeleteUser(user)
		if err != nil {
			return err
		}

		err = userStore.Delete(ctx, user.ID)
		if err != nil {
			return err
		}
		return nil
	})
	return err
}
