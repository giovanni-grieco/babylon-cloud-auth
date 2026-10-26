package web

import (
	"central-auth-svc/internal/service"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
)

//go:embed templates/*
var templateFS embed.FS

type HttpInboundPortConfig struct {
	Domain string
	// SecureCookies marks the session cookie Secure (HTTPS only). Off for local plain-HTTP runs.
	SecureCookies bool
}

// Service holds the auth service's configuration and dependencies.
type HttpInboundPort struct {
	cfg HttpInboundPortConfig
	tpl *template.Template
	svc *service.CentralAuthSvc
}

type loginPage struct {
	Error string
	State string
}

func New(cfg HttpInboundPortConfig, svc *service.CentralAuthSvc) *HttpInboundPort {
	return &HttpInboundPort{
		cfg: cfg,
		tpl: template.Must(template.ParseFS(templateFS, "templates/*.html")),
		svc: svc,
	}
}

func (s *HttpInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLoginPost)

	// Rejects cross-site POST/PUT/DELETE (CSRF, including login CSRF) based on
	// the browser-set Sec-Fetch-Site / Origin headers. GETs are not checked.
	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *HttpInboundPort) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func (s *HttpInboundPort) handleLogin(w http.ResponseWriter, r *http.Request) {
	data := loginPage{
		State: r.URL.Query().Get("state"),
		Error: r.URL.Query().Get("error"),
	}
	if err := s.tpl.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("render login: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

type loginForm struct {
	State    string
	Error    string
	Username string
}

func (s *HttpInboundPort) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	data := loginForm{
		State:    r.FormValue("state"),
		Error:    r.FormValue("error"),
		Username: r.FormValue("username"),
	}

	realmName, err := s.svc.FindRealm(r.Context(), data.Username)
	if err != nil {
		log.Printf("post login: %v", err)
		errorMsg := url.QueryEscape("Username doesn't exist for any realm")
		if data.State != "" {
			http.Redirect(w, r, fmt.Sprintf("/login?state=%s&error=%s", data.State, errorMsg), http.StatusSeeOther)
		} else {
			http.Redirect(w, r, fmt.Sprintf("/login?error=%s", errorMsg), http.StatusSeeOther)
		}
		return
	}

	log.Printf("realm found: %s", realmName)
	// login_hint (as in OIDC) lets the realm prefill the username.
	q := url.Values{}
	q.Set("login_hint", data.Username)
	if data.State != "" {
		q.Set("state", data.State)
	}
	http.Redirect(w, r, fmt.Sprintf("/realm/%s/login?%s", url.PathEscape(realmName), q.Encode()), http.StatusSeeOther)

}
