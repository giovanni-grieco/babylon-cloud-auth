package keycloak

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"control-plane-svc/internal/domain"
)

// Broker implements app.Broker over the central Keycloak.
//
// It relies on two choices in the central realm: each DC's identity provider
// alias equals its realm ID, and the "opaque-username" mapper names brokered
// users "<alias>.<DC user ID>".
type Broker struct {
	central *Client
}

func NewBroker(central *Client) *Broker {
	return &Broker{central: central}
}

// domainsKey is where the home-idp-discovery plugin keeps an identity
// provider's domains, separated by "##".
const domainsKey = "home.idp.discovery.domains"

func (b *Broker) Domains(ctx context.Context, realm domain.RealmID) ([]string, error) {
	var idp struct {
		Config map[string]string `json:"config"`
	}
	path := "/identity-provider/instances/" + url.PathEscape(string(realm))
	if _, err := b.central.do(ctx, http.MethodGet, path, nil, nil, &idp); err != nil {
		return nil, err
	}
	var domains []string
	for _, d := range strings.Split(idp.Config[domainsKey], "##") {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			domains = append(domains, d)
		}
	}
	return domains, nil
}

func (b *Broker) RemoveBrokeredUser(ctx context.Context, realm domain.RealmID, dcUserID string) error {
	username := strings.ToLower(string(realm) + "." + dcUserID)
	var users []userRep
	q := url.Values{"username": {username}, "exact": {"true"}, "briefRepresentation": {"true"}}
	if _, err := b.central.do(ctx, http.MethodGet, "/users", q, nil, &users); err != nil {
		return err
	}
	for _, u := range users {
		if u.Username != username {
			continue // exact=true should already guarantee this
		}
		_, err := b.central.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(u.ID), nil, nil, nil)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
	}
	return nil
}
