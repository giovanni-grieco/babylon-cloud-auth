package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"control-plane-svc/internal/app"
	"control-plane-svc/internal/domain"
)

type store struct{ created, deleted []string }

func (s *store) Groups(context.Context, domain.RealmID) ([]string, error) {
	return []string{"service-a"}, nil
}

func (s *store) CreateUser(_ context.Context, _ domain.RealmID, u domain.NewUser, _ string) (string, error) {
	s.created = append(s.created, u.Username)
	return "id-" + u.Username, nil
}

func (s *store) ListUsers(context.Context, domain.RealmID, string, int, int) ([]domain.User, error) {
	return []domain.User{{ID: "u1", Username: "<script>", Email: "x@acme.test", Enabled: true}}, nil
}

func (s *store) User(_ context.Context, _ domain.RealmID, id string) (domain.User, error) {
	return domain.User{ID: id, Username: "mrossi"}, nil
}

func (s *store) DeleteUser(_ context.Context, _ domain.RealmID, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

type broker struct{}

func (broker) Domains(context.Context, domain.RealmID) ([]string, error) {
	return []string{"acme.test"}, nil
}
func (broker) RemoveBrokeredUser(context.Context, domain.RealmID, string) error { return nil }

type passwords struct{}

func (passwords) New() (string, error) { return "TEMPPASS", nil }

func newTestServer(t *testing.T) (*httptest.Server, *store) {
	t.Helper()
	st := &store{}
	prov := app.NewProvisioner([]domain.Realm{{ID: "dc-1", Name: "DC 1"}}, st, broker{}, passwords{})
	srv := httptest.NewServer(NewServer(prov, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	t.Cleanup(srv.Close)
	return srv, st
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func upload(t *testing.T, url, name, content string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	io.WriteString(fw, content)
	mw.Close()
	resp, err := http.Post(url, mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestPages(t *testing.T) {
	srv, _ := newTestServer(t)

	code, body := get(t, srv.URL+"/")
	if code != 200 || !strings.Contains(body, `href="/realms/dc-1"`) {
		t.Fatalf("index: %d\n%s", code, body)
	}
	code, body = get(t, srv.URL+"/realms/dc-1")
	if code != 200 || !strings.Contains(body, "acme.test") || !strings.Contains(body, "service-a") {
		t.Fatalf("realm: %d\n%s", code, body)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("username isn't HTML-escaped")
	}
	if code, _ := get(t, srv.URL+"/realms/dc-9"); code != http.StatusNotFound {
		t.Fatalf("unknown realm: %d", code)
	}
}

func TestImport(t *testing.T) {
	srv, st := newTestServer(t)
	code, body := upload(t, srv.URL+"/realms/dc-1/import", "u.csv",
		"username,email,groups\nmrossi,mrossi@acme.test,service-a\n")
	if code != 200 || !strings.Contains(body, "TEMPPASS") || len(st.created) != 1 {
		t.Fatalf("import: %d, created %v\n%s", code, st.created, body)
	}
}

func TestImportRejected(t *testing.T) {
	srv, st := newTestServer(t)
	code, body := upload(t, srv.URL+"/realms/dc-1/import", "u.csv",
		"username,email\nmrossi,mrossi@acme.test\nbad,bad@gmail.com\n")
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "gmail.com") || len(st.created) != 0 {
		t.Fatalf("rejected: %d, created %v\n%s", code, st.created, body)
	}
}

func TestDelete(t *testing.T) {
	srv, st := newTestServer(t)
	resp, err := http.PostForm(srv.URL+"/realms/dc-1/delete", url.Values{"id": {"u1", "u2"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || len(st.deleted) != 2 {
		t.Fatalf("delete: %d, deleted %v", resp.StatusCode, st.deleted)
	}
}

func TestCrossSiteDeleteBlocked(t *testing.T) {
	srv, st := newTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/realms/dc-1/delete", strings.NewReader("id=u1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || len(st.deleted) != 0 {
		t.Fatalf("cross-site delete: %d, deleted %v", resp.StatusCode, st.deleted)
	}
}
