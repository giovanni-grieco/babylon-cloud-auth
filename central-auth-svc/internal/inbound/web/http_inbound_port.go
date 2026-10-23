package web

import (
	"central-auth-svc/internal/service"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
