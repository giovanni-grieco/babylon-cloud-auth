package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"control-plane-svc/internal/domain"
)

// Store implements app.IdentityStore over the DC Keycloaks, one client per realm.
type Store struct {
	clients map[domain.RealmID]*Client
}

func NewStore(clients map[domain.RealmID]*Client) *Store {
	return &Store{clients: clients}
}

func (s *Store) client(realm domain.RealmID) (*Client, error) {
	c, ok := s.clients[realm]
	if !ok {
		return nil, fmt.Errorf("%w: %q", domain.ErrUnknownRealm, realm)
	}
	return c, nil
}

// Groups lists top-level groups by name. Users are only assigned to top-level
// groups, which is all the DC realms use.
func (s *Store) Groups(ctx context.Context, realm domain.RealmID) ([]string, error) {
	c, err := s.client(realm)
	if err != nil {
		return nil, err
	}
	var groups []struct {
		Name string `json:"name"`
	}
	q := url.Values{"briefRepresentation": {"true"}, "max": {"1000"}}
	if _, err := c.do(ctx, http.MethodGet, "/groups", q, nil, &groups); err != nil {
		return nil, err
	}
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	return names, nil
}

type credential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type userRep struct {
	ID               string       `json:"id,omitempty"`
	Username         string       `json:"username"`
	Email            string       `json:"email,omitempty"`
	Enabled          bool         `json:"enabled"`
	CreatedTimestamp int64        `json:"createdTimestamp,omitempty"`
	Groups           []string     `json:"groups,omitempty"`
	Credentials      []credential `json:"credentials,omitempty"`
	RequiredActions  []string     `json:"requiredActions,omitempty"`
}

func (r userRep) toDomain() domain.User {
	u := domain.User{ID: r.ID, Username: r.Username, Email: r.Email, Enabled: r.Enabled}
	if r.CreatedTimestamp > 0 {
		u.CreatedAt = time.UnixMilli(r.CreatedTimestamp)
	}
	return u
}

// CreateUser creates the user, its group memberships and its temporary password
// in one request, so a failure can't leave a half-configured account.
func (s *Store) CreateUser(ctx context.Context, realm domain.RealmID, u domain.NewUser, tempPassword string) (string, error) {
	c, err := s.client(realm)
	if err != nil {
		return "", err
	}
	groups := make([]string, len(u.Groups))
	for i, g := range u.Groups {
		groups[i] = "/" + g // Keycloak takes group paths here
	}
	rep := userRep{
		Username:    u.Username,
		Email:       u.Email,
		Enabled:     true,
		Groups:      groups,
		Credentials: []credential{{Type: "password", Value: tempPassword, Temporary: true}},
	}
	resp, err := c.do(ctx, http.MethodPost, "/users", nil, rep, nil)
	if err != nil {
		return "", err
	}
	// Keycloak answers 201 with the new user's URL in Location.
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("keycloak created %q but returned no Location", u.Username)
	}
	return path.Base(loc), nil
}

func (s *Store) ListUsers(ctx context.Context, realm domain.RealmID, search string, first, max int) ([]domain.User, error) {
	c, err := s.client(realm)
	if err != nil {
		return nil, err
	}
	q := url.Values{
		"briefRepresentation": {"true"},
		"first":               {strconv.Itoa(first)},
		"max":                 {strconv.Itoa(max)},
	}
	if search != "" {
		q.Set("search", search)
	}
	var reps []userRep
	if _, err := c.do(ctx, http.MethodGet, "/users", q, nil, &reps); err != nil {
		return nil, err
	}
	users := make([]domain.User, 0, len(reps))
	for _, r := range reps {
		// Service accounts, including this service's own, aren't people to manage here.
		if strings.HasPrefix(r.Username, "service-account-") {
			continue
		}
		users = append(users, r.toDomain())
	}
	return users, nil
}

func (s *Store) User(ctx context.Context, realm domain.RealmID, id string) (domain.User, error) {
	c, err := s.client(realm)
	if err != nil {
		return domain.User{}, err
	}
	var rep userRep
	if _, err := c.do(ctx, http.MethodGet, "/users/"+url.PathEscape(id), nil, nil, &rep); err != nil {
		return domain.User{}, err
	}
	return rep.toDomain(), nil
}

func (s *Store) DeleteUser(ctx context.Context, realm domain.RealmID, id string) error {
	c, err := s.client(realm)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodDelete, "/users/"+url.PathEscape(id), nil, nil, nil)
	return err
}
