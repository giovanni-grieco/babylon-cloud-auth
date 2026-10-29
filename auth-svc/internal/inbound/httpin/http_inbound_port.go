package httpin

import (
	"auth-svc/internal/service"
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
	cfg   HttpInboundPortConfig
	tpl   *template.Template
	svc   *service.AuthService
	store StateStore
}

func New(cfg HttpInboundPortConfig, svc *service.AuthService, store StateStore) *HttpInboundPort {
	return &HttpInboundPort{
		cfg:   cfg,
		tpl:   template.Must(template.ParseFS(templateFS, "templates/*.html")),
		svc:   svc,
		store: store,
	}
}

// Routes builds the HTTP handlers served by the auth service.
func (s *HttpInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /login", s.handleLoginPost)
	mux.HandleFunc("GET /register", s.handleRegister)
	mux.HandleFunc("POST /register", s.handleRegisterPost)
	mux.HandleFunc("GET /verify", s.handleVerify)

	// Rejects cross-site POST/PUT/DELETE (CSRF, including login CSRF) based on
	// the browser-set Sec-Fetch-Site / Origin headers. GETs are not checked.
	return http.NewCrossOriginProtection().Handler(mux)
}

func (s *HttpInboundPort) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

type loginPage struct {
	Error     string
	State     string
	Username  string
	ReturnUrl string
}

func (s *HttpInboundPort) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := loginPage{
		State:     r.URL.Query().Get("state"),
		Error:     r.URL.Query().Get("error"),
		Username:  r.URL.Query().Get("login_hint"),
		ReturnUrl: fmt.Sprintf("/realm/%s/login", s.svc.RealmID),
	}
	if err := s.tpl.ExecuteTemplate(w, "login.html", data); err != nil {
		log.Printf("render login: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

func (s *HttpInboundPort) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		log.Printf("parse form: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	formUser := r.FormValue("username")
	formPass := r.FormValue("password")
	state := r.FormValue("state")

	loginResult, err := s.svc.Login(ctx, formUser, formPass)
	if err != nil {
		// Back to this realm's login page, keeping the username for the retry.
		q := url.Values{}
		q.Set("error", "invalid username or password")
		q.Set("login_hint", formUser)
		if state != "" {
			q.Set("state", state)
		}
		http.Redirect(w, r, fmt.Sprintf("/realm/%s/login?%s", s.svc.RealmID, q.Encode()), http.StatusSeeOther)
		return
	}

	accessTokenCookie := http.Cookie{
		Name:     "access_token",
		Value:    loginResult.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, // not sent on cross-site POSTs
	}
	http.SetCookie(w, &accessTokenCookie)

	refreshTokenCookie := http.Cookie{
		Name:     "refresh_token",
		Value:    loginResult.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, // not sent on cross-site POSTs
	}
	http.SetCookie(w, &refreshTokenCookie)

	val, err := s.store.GetState(ctx, state)

	if err != nil {
		log.Printf("redis error: %v\n", err)
		log.Printf("Redis key not found")
		val = ""
	}

	http.Redirect(w, r, fmt.Sprintf("http://%s%s", s.cfg.Domain, val), http.StatusSeeOther)
}

type registerPage struct {
	Error string
}

func (s *HttpInboundPort) handleRegister(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Printf("parse form: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	data := registerPage{
		Error: r.URL.Query().Get("error"),
	}

	if err := s.tpl.ExecuteTemplate(w, "register.html", data); err != nil {
		log.Printf("render register: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

func (s *HttpInboundPort) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		log.Printf("parse form: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	formEmail := r.FormValue("email")
	formUser := r.FormValue("username")
	formPass := r.FormValue("password")

	if err := s.svc.Register(ctx, formUser, formEmail, formPass); err != nil {
		target := fmt.Sprintf("/register?error=%s", url.QueryEscape(err.Error()))
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)

}

// Stub: always authenticates. Replace with real token validation later.
// forwardAuth always calls this with GET; the original method is in X-Forwarded-Method.
func (s *HttpInboundPort) handleVerify(w http.ResponseWriter, r *http.Request) {
	// CrossOriginProtection skips GETs, so the forwarded request is checked here:
	// this covers every route behind auth-middleware.
	if s.isCrossSiteWrite(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	cookie, err := r.Cookie("auth")

	if err != nil || cookie.Value != "1234" {
		switch {
		case errors.Is(err, http.ErrNoCookie):
			originalUri := r.Header.Get("X-Forwarded-Uri")
			log.Printf("X-Forwarded-Uri: %s\n", originalUri)
			if originalUri == "" {
				originalUri = r.URL.RequestURI()
				log.Printf("RequestURI: %s\n", originalUri)
			}

			state := generateRandomStateToken()

			if err := s.store.SetState(ctx, state, originalUri, 5*time.Minute); err != nil {
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

// isCrossSiteWrite reports whether the original (forwarded) request is a
// state-changing request started by another site.
func (s *HttpInboundPort) isCrossSiteWrite(r *http.Request) bool {
	method := r.Header.Get("X-Forwarded-Method")
	if method == "" {
		method = r.Method
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}

	// Modern browsers: they say outright where the request came from.
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site":
		return true
	case "same-origin", "same-site", "none":
		return false
	}

	// Older browsers: compare Origin with our public domain. r.Host can't be
	// used here, it's auth-svc's internal address.
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false // not a browser cross-site request (curl, service-to-service)
	}
	u, err := url.Parse(origin)
	return err != nil || u.Host != s.cfg.Domain
}

func generateRandomStateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
