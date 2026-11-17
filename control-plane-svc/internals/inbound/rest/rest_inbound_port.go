package rest

import (
	"control-plane-svc/internals/service"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
)

type RestInboundPort struct {
	Address             string
	Port                int
	secret              string
	ControlPlaneService *service.ControlPlaneService
}

func NewRestInboundPort(address string, port int, secret string, controlPlaneService *service.ControlPlaneService) *RestInboundPort {
	return &RestInboundPort{
		Address:             address,
		Port:                port,
		secret:              secret,
		ControlPlaneService: controlPlaneService,
	}
}

func (rip *RestInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/discover", rip.DiscoverPostHandler)

	return securityHandler(rip.secret, http.NewCrossOriginProtection().Handler(mux))
}

func securityHandler(secret string, next http.Handler) http.Handler {
	want := []byte("Bearer " + secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(want, got) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="control-plane"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return // next is never called
		}
		next.ServeHTTP(w, r)
	})
}

type DiscoveryDto struct {
	LoginHint string `json:"login_hint"`
}

type DiscoveryResponseDto struct {
	Idp string `json:"idp"`
}

func (rip *RestInboundPort) DiscoverPostHandler(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	var discoveryDto DiscoveryDto
	err := json.NewDecoder(r.Body).Decode(&discoveryDto)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	slog.DebugContext(ctx, "discover request", "login_hint", discoveryDto.LoginHint)
	idpAlias, err := rip.ControlPlaneService.DiscoverRealm(ctx, discoveryDto.LoginHint)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(DiscoveryResponseDto{Idp: idpAlias})
}
