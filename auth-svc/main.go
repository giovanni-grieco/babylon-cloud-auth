/*
Piccola frontend con user-password form e login button
Quando il login è fatto, viene generato un access e refresh token che vengono codificati firmati dalla chiave privata del auth-svc
Viene salvata la sessione (il refresh token) nel KV-Store e viene annunciata la creazione della sessione sul message broker.

Quando un servizio viene colpito (oppure un gateway iniziale) e non trova token, allora bisogna generare un token randomico, associare quell'utente a quel token, e re-indirizzare l'utente verso il login.
Eseguito il login, avviene un-reindirizzamento al gateway originale, che a questo punto controllerà il token randomico, vedrà a che percorso originalmente voleva puntare
*/

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	domain      = "localhost"
	serviceName = "auth"
	servicePort = 8080

	username = "admin"
	password = "admin"
)

var (
	consulAddr = envOr("CONSUL_ADDR", "consul")
	consulPort = envOr("CONSUL_PORT", "8500")
	instanceID = hostname()
	httpClient = &http.Client{Timeout: 5 * time.Second}
)

//go:embed templates/*
var templateFS embed.FS

var loginTmpl = template.Must(template.ParseFS(templateFS, "templates/login.html"))

var postLoginTmpl = template.Must(template.ParseFS(templateFS, "templates/post-login.html"))

type loginPage struct {
	Error string
	State string
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return fmt.Sprintf("auth-%d", time.Now().UnixNano())
	}
	return name
}

// containerIP returns this container's IP on the docker network.
func containerIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return "127.0.0.1"
}

func agentURL() string {
	return fmt.Sprintf("http://%s:%s/v1/agent", consulAddr, consulPort)
}

type healthCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

type serviceRegistration struct {
	ID      string      `json:"ID"`
	Name    string      `json:"Name"`
	Address string      `json:"Address"`
	Port    int         `json:"Port"`
	Tags    []string    `json:"Tags"`
	Check   healthCheck `json:"Check"`
}

func RegisterService() bool {
	ip := containerIP()
	payload := serviceRegistration{
		ID:      instanceID,
		Name:    serviceName,
		Address: ip,
		Port:    servicePort,
		Tags:    []string{"traefik.enable=true"},
		Check: healthCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health", ip, servicePort),
			Interval:                       "10s",
			Timeout:                        "2s",
			DeregisterCriticalServiceAfter: "30s",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	req, err := http.NewRequest(http.MethodPut, agentURL()+"/service/register", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("register: unexpected status %d", resp.StatusCode)
		return false
	}

	log.Printf("registered with Consul as %s (%s)", instanceID, payload.Address)
	return true
}

func DeRegisterService() {
	req, err := http.NewRequest(http.MethodPut, agentURL()+"/service/deregister/"+instanceID, nil)
	if err != nil {
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("deregistration failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("deregistered %s from Consul", instanceID)
}

func RegisterLoop() {
	for !RegisterService() {
		time.Sleep(5 * time.Second)
	}
}

func InitServiceRegisterLoops() {
	go RegisterLoop()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		DeRegisterService()
		os.Exit(0)
	}()
}

func GenerateRandomStateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func main() {

	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "auth-kv:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		log.Println("/health check ok")
	})

	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := loginPage{
			State: r.URL.Query().Get("state"),
			Error: r.URL.Query().Get("error"),
		}
		if err := loginTmpl.Execute(w, data); err != nil {
			log.Printf("render login: %v", err)
		}
	})

	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			log.Printf("parse form: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		formUser := r.FormValue("username")
		formPass := r.FormValue("password")
		state := r.FormValue("state")

		if formUser != username || formPass != password {
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

		val, err := rdb.Get(ctx, state).Result()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("http://%s%s", domain, val), http.StatusSeeOther)
	})

	// Stub: always authenticates. Replace with real token validation later.
	// Method-agnostic on purpose: forwardAuth preserves the original request method.
	mux.HandleFunc("GET /verify", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("auth")
		if err != nil {
			switch {
			case errors.Is(err, http.ErrNoCookie):
				originalUri := r.Header.Get("X-Forwarded-Uri")
				if originalUri == "" {
					originalUri = r.URL.RequestURI()
				}
				state := GenerateRandomStateToken()

				err = rdb.Set(ctx, state, originalUri, 5*time.Minute).Err()
				if err != nil {
					log.Println(err)
					http.Error(w, "server error", http.StatusInternalServerError)
				}
				http.Redirect(w, r, fmt.Sprintf("http://%s/login?state=%s", domain, state), http.StatusMovedPermanently)
			default:
				log.Println(err)
				http.Error(w, "server error", http.StatusInternalServerError)
			}
		} else {
			fmt.Fprintf(w, "cookie: %s", cookie.Value)
		}
	})

	InitServiceRegisterLoops()

	log.Printf("Auth Service running on :%d", servicePort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", servicePort), mux))
}
