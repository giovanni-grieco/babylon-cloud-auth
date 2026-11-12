package web

import (
	"context"
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/domain"
	"control-plane-svc/internals/service"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- in-memory UserStore ----------------------------------------------------

type memStore struct {
	mu    sync.Mutex
	users map[string]domain.User
}

func newMemStore() *memStore { return &memStore{users: map[string]domain.User{}} }

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (s *memStore) Find(_ context.Context, id string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", service.ErrNotFound, id)
	}
	return &u, nil
}
func (s *memStore) FindForUpdate(ctx context.Context, id string) (*domain.User, error) {
	return s.Find(ctx, id)
}
func (s *memStore) FindForUpdateSkipLocked(ctx context.Context, id string) (*domain.User, error) {
	return s.Find(ctx, id)
}
func (s *memStore) findBy(match func(domain.User) bool) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if match(u) {
			return &u, nil
		}
	}
	return nil, service.ErrNotFound
}
func (s *memStore) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	return s.findBy(func(u domain.User) bool { return u.Username == username })
}
func (s *memStore) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	return s.findBy(func(u domain.User) bool { return u.Email == email })
}
func (s *memStore) all() []*domain.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*domain.User
	for _, u := range s.users {
		out = append(out, &u)
	}
	slices.SortFunc(out, func(a, b *domain.User) int {
		return strings.Compare(a.RealmAlias+"/"+a.Username, b.RealmAlias+"/"+b.Username)
	})
	return out
}
func (s *memStore) FindByRealmAlias(_ context.Context, realm string) ([]*domain.User, error) {
	var out []*domain.User
	for _, u := range s.all() {
		if u.RealmAlias == realm {
			out = append(out, u)
		}
	}
	return out, nil
}
func (s *memStore) List(_ context.Context, offset, limit int) ([]*domain.User, error) {
	all := s.all()
	if offset >= len(all) {
		return nil, nil
	}
	return all[offset:min(len(all), offset+limit)], nil
}
func (s *memStore) FindStale(context.Context, int, time.Duration, int) ([]*domain.User, error) {
	return nil, nil
}
func (s *memStore) InTx(ctx context.Context, fn func(context.Context, service.UserStore) error) error {
	return fn(ctx, s)
}
func (s *memStore) Save(_ context.Context, user *domain.User) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == user.Username || u.Email == user.Email {
			return nil, service.ErrIdentifierTaken
		}
	}
	saved := *user
	saved.ID = newID()
	s.users[saved.ID] = saved
	return &saved, nil
}
func (s *memStore) Update(_ context.Context, user *domain.User) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return nil, service.ErrNotFound
	}
	s.users[user.ID] = *user
	updated := *user
	return &updated, nil
}
func (s *memStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return service.ErrNotFound
	}
	delete(s.users, id)
	return nil
}

// ---- fake Keycloak Admin API -----------------------------------------------

type fakeKeycloak struct {
	mu        sync.Mutex
	users     map[string]map[string]string // id -> {username, email, password}
	passwords int
}

func (f *fakeKeycloak) handler() http.Handler {
	const users = "/admin/realms/babylon/users"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /realms/babylon/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "token", "expires_in": 300})
	})
	mux.HandleFunc("POST "+users, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username    string `json:"username"`
			Email       string `json:"email"`
			Credentials []struct {
				Value     string `json:"value"`
				Temporary bool   `json:"temporary"`
			} `json:"credentials"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, u := range f.users {
			if u["username"] == body.Username || u["email"] == body.Email {
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		if len(body.Credentials) != 1 || !body.Credentials[0].Temporary || len(body.Credentials[0].Value) < 12 {
			http.Error(w, "bad credential", http.StatusBadRequest)
			return
		}
		id := newID()
		f.users[id] = map[string]string{"username": body.Username, "email": body.Email, "password": body.Credentials[0].Value}
		f.passwords++
		w.Header().Set("Location", "http://kc"+users+"/"+id)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET "+users, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var out []map[string]string
		for id, u := range f.users {
			if u["username"] == r.URL.Query().Get("username") {
				out = append(out, map[string]string{"id": id, "username": u["username"], "email": u["email"]})
			}
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("DELETE "+users+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.users[r.PathValue("id")]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.users, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT "+users+"/{id}/reset-password", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.users[r.PathValue("id")]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.passwords++
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// ---- test -------------------------------------------------------------------

func newConsole(t *testing.T) (*httptest.Server, *memStore, *fakeKeycloak) {
	t.Helper()
	kc := &fakeKeycloak{users: map[string]map[string]string{}}
	kcServer := httptest.NewServer(kc.handler())
	t.Cleanup(kcServer.Close)

	store := newMemStore()
	svc := service.NewControlPlanceService(store, map[string]*clients.KeycloakClient{
		"dc-1": clients.NewKeycloakClient(kcServer.URL, "control-plane", "secret", "dc-1"),
	})
	console := httptest.NewServer(NewWebInboundPort("", 0, svc).Routes())
	t.Cleanup(console.Close)
	return console, store, kc
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func do(t *testing.T, method, target string, form url.Values) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest(method, target, strings.NewReader(form.Encode()))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(body)
}

func mustContain(t *testing.T, body string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(body, p) {
			t.Fatalf("expected body to contain %q\n---\n%s", p, body)
		}
	}
}

func TestOperatorConsoleLifecycle(t *testing.T) {
	console, store, kc := newConsole(t)

	status, _, body := do(t, "GET", console.URL+"/users", nil)
	if status != 200 {
		t.Fatalf("list: %d", status)
	}
	mustContain(t, body, "No users yet", `rel="create-form" href="/users/new"`)

	_, _, body = do(t, "GET", console.URL+"/users/new", nil)
	mustContain(t, body, `<option value="dc-1"`, `action="/users"`)

	// Create: the password is shown once and never cached.
	form := url.Values{"username": {" Alice "}, "email": {"Alice@Acme.it"}, "realm": {"dc-1"}}
	status, header, body := do(t, "POST", console.URL+"/users", form)
	if status != http.StatusCreated {
		t.Fatalf("create: %d\n%s", status, body)
	}
	if header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(header.Get("Location"), "/users/") {
		t.Fatalf("create headers: %v", header)
	}
	mustContain(t, body, `id="temporary-password"`, "alice@acme.it", "CREATED")
	if len(kc.users) != 1 {
		t.Fatalf("keycloak users: %d", len(kc.users))
	}
	userURL := console.URL + header.Get("Location")
	user := store.all()[0]
	if user.State != domain.CREATED || user.RealmID == "" || user.Username != "alice" {
		t.Fatalf("stored user: %+v", user)
	}

	// The same email in any realm is rejected before Keycloak is called.
	status, _, body = do(t, "POST", console.URL+"/users", url.Values{"username": {"alice2"}, "email": {"alice@acme.it"}, "realm": {"dc-1"}})
	if status != http.StatusConflict {
		t.Fatalf("duplicate: %d", status)
	}
	mustContain(t, body, "already registered", `value="alice2"`)

	status, _, _ = do(t, "POST", console.URL+"/users", url.Values{"username": {"bob"}, "email": {"not-an-email"}, "realm": {"dc-1"}})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("invalid email: %d", status)
	}

	// The list offers exactly the transitions a CREATED user allows.
	_, _, body = do(t, "GET", console.URL+"/users", nil)
	mustContain(t, body, `rel="item"`, `rel="reset-password"`, "/password", `rel="delete"`, "state-created")

	_, _, body = do(t, "GET", userURL, nil)
	mustContain(t, body, user.RealmID, "reserved")

	passwordsBefore := kc.passwords
	status, header, body = do(t, "POST", userURL+"/password", url.Values{})
	if status != 200 || header.Get("Cache-Control") != "no-store" || kc.passwords != passwordsBefore+1 {
		t.Fatalf("reset: %d %v", status, header)
	}
	mustContain(t, body, `id="temporary-password"`)

	_, _, body = do(t, "GET", userURL+"/delete", nil)
	mustContain(t, body, "Delete alice?", `method="post"`)

	status, header, _ = do(t, "POST", userURL+"/delete", url.Values{})
	if status != http.StatusSeeOther || header.Get("Location") != "/users?notice=deleted" {
		t.Fatalf("delete: %d %v", status, header)
	}
	if len(kc.users) != 0 || len(store.all()) != 0 {
		t.Fatalf("after delete: keycloak=%d store=%d", len(kc.users), len(store.all()))
	}
	_, _, body = do(t, "GET", console.URL+"/users?notice=deleted", nil)
	mustContain(t, body, "User deleted")

	status, _, _ = do(t, "GET", userURL, nil)
	if status != http.StatusNotFound {
		t.Fatalf("deleted user page: %d", status)
	}
}

func TestPendingUserHasNoControls(t *testing.T) {
	console, store, _ := newConsole(t)
	pending, _ := store.Save(context.Background(), &domain.User{Username: "carol", Email: "carol@acme.it", RealmAlias: "dc-1", State: domain.PENDING_CREATION})

	_, _, body := do(t, "GET", console.URL+"/users", nil)
	mustContain(t, body, "state-pending-creation")
	if strings.Contains(body, `rel="delete"`) || strings.Contains(body, `rel="reset-password"`) {
		t.Fatalf("pending user must offer no controls:\n%s", body)
	}

	_, _, body = do(t, "GET", console.URL+"/users/"+pending.ID, nil)
	mustContain(t, body, "reconciler", `rel="self"`)

	status, _, _ := do(t, "GET", console.URL+"/users/"+pending.ID+"/delete", nil)
	if status != http.StatusConflict {
		t.Fatalf("confirm delete on pending: %d", status)
	}
	status, _, _ = do(t, "POST", console.URL+"/users/"+pending.ID+"/password", url.Values{})
	if status != http.StatusConflict {
		t.Fatalf("reset on pending: %d", status)
	}
}

// Over plain HTTP on a non-localhost host the browser sends no Sec-Fetch-Site,
// so CrossOriginProtection relies on Origin alone. The Referrer-Policy must
// let the browser send its real Origin on same-origin POSTs.
func TestSameOriginPostOverPlainHTTP(t *testing.T) {
	console, _, _ := newConsole(t)

	res, err := http.Get(console.URL + "/users/new")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if policy := res.Header.Get("Referrer-Policy"); policy == "no-referrer" || policy == "" {
		t.Fatalf("Referrer-Policy %q makes browsers send Origin: null on form posts", policy)
	}

	req, _ := http.NewRequest("POST", console.URL+"/users", strings.NewReader("username=dan&email=dan@acme.it&realm=dc-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", console.URL) // what the browser sends under Referrer-Policy: same-origin
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("same-origin post: %d", res.StatusCode)
	}
}

func TestBadIDsAndCrossSitePosts(t *testing.T) {
	console, _, _ := newConsole(t)

	status, _, _ := do(t, "GET", console.URL+"/users/not-a-uuid", nil)
	if status != http.StatusNotFound {
		t.Fatalf("bad id: %d", status)
	}

	req, _ := http.NewRequest("POST", console.URL+"/users", strings.NewReader("username=x&email=x@y.z&realm=dc-1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site post: %d", res.StatusCode)
	}
}
