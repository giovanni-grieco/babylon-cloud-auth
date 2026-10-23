package httpin

import (
	"auth-svc/internal/service"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

type HttpInboundPortConfig struct {
	Domain            string
	Realm             string
	CentralAuthURL    string
	InternalAuthToken string
}

func (s *HttpInboundPort) lookupTransaction(r *http.Request, transaction string, consume bool) (string, error) {
	if transaction == "" || s.cfg.CentralAuthURL == "" || s.cfg.InternalAuthToken == "" {
		return "", errors.New("transaction exchange is not configured")
	}
	method := http.MethodGet
	if consume {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(r.Context(), method,
		strings.TrimRight(s.cfg.CentralAuthURL, "/")+"/internal/login-transactions/"+url.PathEscape(transaction), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Central-Auth-Token", s.cfg.InternalAuthToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("central auth returned %s", response.Status)
	}
	var result struct {
		Username string `json:"username"`
		Realm    string `json:"realm"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Username == "" {
		return "", errors.New("central auth returned an empty username")
	}
	if s.cfg.Realm != "" && result.Realm != s.cfg.Realm {
		return "", errors.New("transaction belongs to another realm")
	}
	return result.Username, nil
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

	return mux
}

func (s *HttpInboundPort) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

type loginPage struct {
	Error       string
	Transaction string
	Return      string
	Username    string
	Action      string
}

func (s *HttpInboundPort) handleLogin(w http.ResponseWriter, r *http.Request) {
	transaction := r.URL.Query().Get("transaction")
	if transaction == "" {
		http.Error(w, "login transaction is required", http.StatusBadRequest)
		return
	}
	username, err := s.lookupTransaction(r, transaction, false)
	if err != nil {
		http.Error(w, "login transaction is invalid or expired", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := loginPage{
		Transaction: transaction,
		Return:      r.URL.Query().Get("return"),
		Username:    username,
		Error:       r.URL.Query().Get("error"),
		Action:      fmt.Sprintf("/_realm/%s/login", url.PathEscape(s.cfg.Realm)),
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

	formPass := r.FormValue("password")
	transaction := r.FormValue("transaction")
	returnPath := r.FormValue("return")
	username, err := s.lookupTransaction(r, transaction, false)
	if err != nil {
		http.Error(w, "login transaction is invalid or expired", http.StatusGone)
		return
	}

	if err := s.svc.Login(ctx, username, formPass); err != nil {
		target := fmt.Sprintf("/_realm/%s/login?transaction=%s&return=%s&error=%s",
			url.PathEscape(s.cfg.Realm),
			url.QueryEscape(transaction),
			url.QueryEscape(returnPath),
			url.QueryEscape("invalid username or password"))
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	if _, err := s.lookupTransaction(r, transaction, true); err != nil {
		http.Error(w, "login transaction is invalid or expired", http.StatusGone)
		return
	}

	cookie := http.Cookie{
		Name:  "auth",
		Value: "1234",
	}
	http.SetCookie(w, &cookie)

	target := returnPath
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
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
// Method-agnostic on purpose: forwardAuth preserves the original request method.
func (s *HttpInboundPort) handleVerify(w http.ResponseWriter, r *http.Request) {
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

			http.Redirect(w, r, fmt.Sprintf("http://%s/login?return=%s", s.cfg.Domain, url.QueryEscape(originalUri)), http.StatusSeeOther)
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
