// Package web is the operator console: plain HTML pages served by the standard library.
package web

import (
	"embed"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"control-plane-svc/internal/app"
	"control-plane-svc/internal/domain"
)

const (
	maxUpload = 5 << 20 // 5 MiB is far more than MaxBatch rows need
	pageSize  = 50
)

//go:embed templates/*.html
var templateFS embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
}).ParseFS(templateFS, "templates/*.html"))

type Server struct {
	prov *app.Provisioner
	log  *slog.Logger
}

func NewServer(prov *app.Provisioner, log *slog.Logger) *Server {
	return &Server{prov: prov, log: log}
}

// Handler returns the routes, protected against cross-site form posts: any
// web page the operator visits could otherwise submit the delete form.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /realms/{realm}", s.realm)
	mux.HandleFunc("POST /realms/{realm}/import", s.importUsers)
	mux.HandleFunc("POST /realms/{realm}/delete", s.deleteUsers)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok\n") })
	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "index.html", map[string]any{"Realms": s.prov.Realms()})
}

type realmPage struct {
	Info   app.RealmInfo
	Users  []domain.User
	Search string
	First  int
	Next   int // 0 when there's no next page
	Prev   int // -1 when there's no previous page
}

func (s *Server) realm(w http.ResponseWriter, r *http.Request) {
	id := domain.RealmID(r.PathValue("realm"))
	info, err := s.prov.Describe(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	search := r.URL.Query().Get("search")
	first, _ := strconv.Atoi(r.URL.Query().Get("first"))
	first = max(first, 0)
	// Ask for one extra user to know whether there's a next page.
	users, err := s.prov.Users(r.Context(), id, search, first, pageSize+1)
	if err != nil {
		s.fail(w, err)
		return
	}
	page := realmPage{Info: info, Search: search, First: first, Prev: -1}
	if len(users) > pageSize {
		users = users[:pageSize]
		page.Next = first + pageSize
	}
	if first > 0 {
		page.Prev = max(first-pageSize, 0)
	}
	page.Users = users
	s.render(w, http.StatusOK, "realm.html", page)
}

func (s *Server) importUsers(w http.ResponseWriter, r *http.Request) {
	id := domain.RealmID(r.PathValue("realm"))
	realm, err := s.prov.Realm(id)
	if err != nil {
		s.fail(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	file, header, err := r.FormFile("file")
	if err != nil {
		s.badRequest(w, realm, "Choose a CSV or JSON file to upload (at most 5 MiB).")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		s.badRequest(w, realm, "The upload was interrupted or is larger than 5 MiB.")
		return
	}
	users, err := parseUsers(header.Filename, data)
	if err != nil {
		s.badRequest(w, realm, err.Error())
		return
	}

	report, err := s.prov.Import(r.Context(), id, users)
	if batch, ok := errors.AsType[*domain.BatchError](err); ok {
		s.render(w, http.StatusUnprocessableEntity, "rejected.html", map[string]any{"Realm": realm, "Batch": batch})
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("import", "realm", id, "rows", len(report.Results), "failed", report.Failed())
	// The page holds the only copy of the initial passwords.
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, http.StatusOK, "imported.html", report)
}

func (s *Server) deleteUsers(w http.ResponseWriter, r *http.Request) {
	id := domain.RealmID(r.PathValue("realm"))
	realm, err := s.prov.Realm(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, realm, "The form couldn't be read.")
		return
	}
	ids := r.PostForm["id"]
	if len(ids) == 0 {
		s.badRequest(w, realm, "Select at least one user to delete.")
		return
	}
	results, err := s.prov.Delete(r.Context(), id, ids)
	if err != nil {
		s.fail(w, err)
		return
	}
	failed := 0
	for _, d := range results {
		if d.Err != nil {
			failed++
		}
	}
	s.log.Info("delete", "realm", id, "requested", len(ids), "failed", failed)
	s.render(w, http.StatusOK, "deleted.html", map[string]any{"Realm": realm, "Results": results})
}

func (s *Server) badRequest(w http.ResponseWriter, realm domain.Realm, msg string) {
	s.render(w, http.StatusBadRequest, "error.html", map[string]any{"Realm": realm, "Message": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway // most failures come from Keycloak
	if errors.Is(err, domain.ErrUnknownRealm) {
		status = http.StatusNotFound
	} else {
		s.log.Error("request failed", "err", err)
	}
	s.render(w, status, "error.html", map[string]any{"Message": err.Error()})
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("rendering", "template", name, "err", err)
	}
}
