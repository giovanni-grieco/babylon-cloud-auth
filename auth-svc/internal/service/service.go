package service

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed templates/*
var templateFS embed.FS

type Config struct {
	Domain   string
	Username string
	Password string
}

// Service holds the auth service's configuration and dependencies.
type Service struct {
	cfg Config
	rdb *redis.Client
	tpl *template.Template
}

func New(cfg Config, rdb *redis.Client) *Service {
	return &Service{
		cfg: cfg,
		rdb: rdb,
		tpl: template.Must(template.ParseFS(templateFS, "templates/*.html")),
	}
}

// Routes builds the HTTP handlers served by the auth service.
func (s *Service) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLoginPost)
	mux.HandleFunc("GET /verify", s.handleVerify)

	return mux
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
	log.Println("/health check ok")
}

type loginPage struct {
	Error string
	State string
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := loginPage{
		State: r.URL.Query().Get("state"),
		Error: r.URL.Query().Get("error"),
	}
	if err := s.tpl.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("render login: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

func (s *Service) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		log.Printf("parse form: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	formUser := r.FormValue("username")
	formPass := r.FormValue("password")
	state := r.FormValue("state")

	if formUser != s.cfg.Username || formPass != s.cfg.Password {
		target := fmt.Sprintf("/login?state=%s&error=%s",
			url.QueryEscape(state), url.QueryEscape("invalid username or password"))
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}

	cookie := http.Cookie{
		Name:  "auth",
		Value: "1234",
	}
	http.SetCookie(w, &cookie)

	val, err := s.rdb.Get(ctx, state).Result()
	if err != nil {
		log.Printf("redis error: %v\n", err)
		log.Printf("Redis key not found")
		val = ""
	}

	http.Redirect(w, r, fmt.Sprintf("http://%s%s", s.cfg.Domain, val), http.StatusSeeOther)
}

// Stub: always authenticates. Replace with real token validation later.
// Method-agnostic on purpose: forwardAuth preserves the original request method.
func (s *Service) handleVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, err := r.Cookie("auth")
	if err != nil || cookie.Value != "1234" {
		switch {
		case errors.Is(err, http.ErrNoCookie):
			originalUri := r.Header.Get("X-Forwarded-Uri")
			if originalUri == "" {
				originalUri = r.URL.RequestURI()
			}
			state := generateRandomStateToken()

			if err := s.rdb.Set(ctx, state, originalUri, 5*time.Minute).Err(); err != nil {
				log.Println(err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, fmt.Sprintf("http://%s/login?state=%s", s.cfg.Domain, state), http.StatusMovedPermanently)
		default:
			log.Println(err)
			http.Error(w, "server error", http.StatusInternalServerError)
		}
	} else {
		fmt.Fprintf(w, "cookie: %s", cookie.Value)
	}
}

func generateRandomStateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
