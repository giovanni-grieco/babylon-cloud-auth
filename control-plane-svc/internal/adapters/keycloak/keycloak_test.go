package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"control-plane-svc/internal/domain"
)

// fakeKeycloak answers the few endpoints the adapter uses.
func fakeKeycloak(t *testing.T, tokens *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/babylon/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_secret") != "s3cret" {
			http.Error(w, `{"error":"unauthorized_client"}`, http.StatusUnauthorized)
			return
		}
		tokens.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
	})
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("POST /admin/realms/babylon/users", auth(func(w http.ResponseWriter, r *http.Request) {
		var rep userRep
		json.NewDecoder(r.Body).Decode(&rep)
		if rep.Username == "taken" {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"errorMessage": "User exists with same username"})
			return
		}
		if !slices.Equal(rep.Groups, []string{"/service-a"}) || len(rep.Credentials) != 1 || !rep.Credentials[0].Temporary {
			http.Error(w, `{"errorMessage":"bad representation"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Location", "http://kc/admin/realms/babylon/users/0b7c-uuid")
		w.WriteHeader(http.StatusCreated)
	}))
	mux.HandleFunc("GET /admin/realms/babylon/users", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("exact") == "true" {
			json.NewEncoder(w).Encode([]userRep{{ID: "central-1", Username: r.URL.Query().Get("username")}})
			return
		}
		json.NewEncoder(w).Encode([]userRep{
			{ID: "1", Username: "mrossi", Enabled: true, CreatedTimestamp: 1700000000000},
			{ID: "2", Username: "service-account-control-plane"},
		})
	}))
	mux.HandleFunc("DELETE /admin/realms/babylon/users/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /admin/realms/babylon/identity-provider/instances/dc-1", auth(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{domainsKey: "Acme.test## other.test ##"}})
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(srv *httptest.Server, secret string) *Client {
	return NewClient(Connection{BaseURL: srv.URL + "/", Realm: "babylon", ClientID: "control-plane", ClientSecret: secret}, srv.Client())
}

func TestStore(t *testing.T) {
	var tokens atomic.Int32
	srv := fakeKeycloak(t, &tokens)
	store := NewStore(map[domain.RealmID]*Client{"dc-1": newTestClient(srv, "s3cret")})
	ctx := context.Background()

	id, err := store.CreateUser(ctx, "dc-1", domain.NewUser{Username: "mrossi", Email: "m@acme.test", Groups: []string{"service-a"}}, "pw")
	if err != nil || id != "0b7c-uuid" {
		t.Fatalf("CreateUser = %q, %v", id, err)
	}
	if _, err := store.CreateUser(ctx, "dc-1", domain.NewUser{Username: "taken"}, "pw"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	users, err := store.ListUsers(ctx, "dc-1", "", 0, 10)
	if err != nil || len(users) != 1 || users[0].Username != "mrossi" || users[0].CreatedAt.IsZero() {
		t.Fatalf("ListUsers = %+v, %v (service accounts must be hidden)", users, err)
	}

	if err := store.DeleteUser(ctx, "dc-1", "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete missing = %v", err)
	}
	if _, err := store.Groups(ctx, "dc-9"); !errors.Is(err, domain.ErrUnknownRealm) {
		t.Fatalf("unknown realm = %v", err)
	}
	if n := tokens.Load(); n != 1 {
		t.Fatalf("fetched %d tokens, want 1 cached token", n)
	}
}

func TestBadSecret(t *testing.T) {
	var tokens atomic.Int32
	srv := fakeKeycloak(t, &tokens)
	store := NewStore(map[domain.RealmID]*Client{"dc-1": newTestClient(srv, "wrong")})
	_, err := store.Groups(context.Background(), "dc-1")
	if err == nil || !strings.Contains(err.Error(), "unauthorized_client") {
		t.Fatalf("error = %v", err)
	}
}

func TestBroker(t *testing.T) {
	var tokens atomic.Int32
	srv := fakeKeycloak(t, &tokens)
	broker := NewBroker(newTestClient(srv, "s3cret"))
	ctx := context.Background()

	domains, err := broker.Domains(ctx, "dc-1")
	if err != nil || !slices.Equal(domains, []string{"acme.test", "other.test"}) {
		t.Fatalf("Domains = %v, %v", domains, err)
	}
	if err := broker.RemoveBrokeredUser(ctx, "dc-1", "ABC"); err != nil {
		t.Fatalf("RemoveBrokeredUser: %v", err)
	}
}
