package main

import (
	"context"
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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/redis/go-redis/v9"
)

const transactionTTL = 2 * time.Minute

//go:embed templates/*
var templateFS embed.FS

type loginTransaction struct {
	Username string `json:"username"`
	Realm    string `json:"realm"`
}

type app struct {
	redis         *redis.Client
	realmByUser   map[string]string
	filters       map[string]*bloom.BloomFilter
	filtersMu     sync.RWMutex
	internalToken string
	templates     *template.Template
}

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Addr: envOr("REDIS_ADDR", "central-auth-kv:6379"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("connect to redis: %v", err)
	}

	a := &app{
		redis:         redisClient,
		filters:       make(map[string]*bloom.BloomFilter),
		internalToken: envOr("CENTRAL_AUTH_INTERNAL_TOKEN", "development-only"),
		templates:     template.Must(template.ParseFS(templateFS, "templates/*.html")),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /login", a.login)
	mux.HandleFunc("POST /login", a.loginPost)
	mux.HandleFunc("GET /internal/login-transactions/", a.exchangePreview)
	mux.HandleFunc("POST /internal/login-transactions/", a.exchange)
	mux.HandleFunc("PUT /internal/realms/", a.uploadBloom)

	log.Println("Central auth service running on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func (a *app) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	original := r.URL.Query().Get("return")
	if original == "" {
		original = "/"
	}
	if !strings.HasPrefix(original, "/") || strings.HasPrefix(original, "//") {
		http.Error(w, "invalid return path", http.StatusBadRequest)
		return
	}
	render(w, a.templates, "login.html", map[string]string{
		"Return": original,
		"Error":  r.URL.Query().Get("error"),
	})
}

func (a *app) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	if username == "" {
		http.Redirect(w, r, "/login?error=Username+is+required", http.StatusSeeOther)
		return
	}
	realm, ok := a.resolveRealm(username)
	if !ok {
		http.Redirect(w, r, "/login?error=Unknown+user", http.StatusSeeOther)
		return
	}

	transactionID, err := randomToken()
	if err != nil {
		http.Error(w, "could not create login transaction", http.StatusInternalServerError)
		return
	}

	transaction := loginTransaction{Username: username, Realm: realm}
	value, err := json.Marshal(transaction)
	if err != nil {
		http.Error(w, "could not create login transaction", http.StatusInternalServerError)
		return
	}
	if err := a.redis.Set(r.Context(), "central-auth:transaction:"+transactionID, value, transactionTTL).Err(); err != nil {
		log.Printf("store login transaction: %v", err)
		http.Error(w, "could not create login transaction", http.StatusInternalServerError)
		return
	}

	returnPath := r.FormValue("return")
	if returnPath == "" {
		returnPath = "/"
	}
	target := fmt.Sprintf("/_realm/%s/login?transaction=%s&return=%s",
		realm, transactionID, url.QueryEscape(returnPath))
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *app) uploadBloom(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Central-Auth-Token") != a.internalToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	realmPath := strings.TrimPrefix(r.URL.Path, "/internal/realms/")
	if !strings.HasSuffix(realmPath, "/bloom") {
		http.Error(w, "invalid realm", http.StatusBadRequest)
		return
	}
	realm := strings.TrimSuffix(realmPath, "/bloom")
	if realm == "" || strings.Contains(realm, "/") {
		http.Error(w, "invalid realm", http.StatusBadRequest)
		return
	}
	filter := bloom.New(0, 0)
	_, err := filter.ReadFrom(r.Body)
	if err != nil {
		http.Error(w, "invalid bloom filter", http.StatusBadRequest)
		return
	}
	a.filtersMu.Lock()
	a.filters[realm] = filter
	a.filtersMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) resolveRealm(username string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(username))
	a.filtersMu.RLock()
	defer a.filtersMu.RUnlock()
	for realm, filter := range a.filters {
		if filter.TestString(normalized) {
			return realm, true
		}
	}
	return "", false
}

func (a *app) exchange(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Central-Auth-Token") != a.internalToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	transactionID := strings.TrimPrefix(r.URL.Path, "/internal/login-transactions/")
	if transactionID == "" || strings.Contains(transactionID, "/") {
		http.Error(w, "invalid transaction", http.StatusBadRequest)
		return
	}

	key := "central-auth:transaction:" + transactionID
	value, err := a.redis.GetDel(r.Context(), key).Result()
	if errors.Is(err, redis.Nil) {
		http.Error(w, "transaction expired or already used", http.StatusGone)
		return
	}
	if err != nil {
		log.Printf("read login transaction: %v", err)
		http.Error(w, "transaction lookup failed", http.StatusInternalServerError)
		return
	}

	var transaction loginTransaction
	if err := json.Unmarshal([]byte(value), &transaction); err != nil {
		http.Error(w, "invalid transaction", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(transaction)
}

func (a *app) exchangePreview(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Central-Auth-Token") != a.internalToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	transactionID := strings.TrimPrefix(r.URL.Path, "/internal/login-transactions/")
	if transactionID == "" || strings.Contains(transactionID, "/") {
		http.Error(w, "invalid transaction", http.StatusBadRequest)
		return
	}
	value, err := a.redis.Get(r.Context(), "central-auth:transaction:"+transactionID).Result()
	if errors.Is(err, redis.Nil) {
		http.Error(w, "transaction expired or already used", http.StatusGone)
		return
	}
	if err != nil {
		http.Error(w, "transaction lookup failed", http.StatusInternalServerError)
		return
	}
	var transaction loginTransaction
	if err := json.Unmarshal([]byte(value), &transaction); err != nil {
		http.Error(w, "invalid transaction", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(transaction)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func render(w http.ResponseWriter, templates *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
