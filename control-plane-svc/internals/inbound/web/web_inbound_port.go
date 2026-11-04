package web

import (
	"control-plane-svc/internals/service"
	"log"
	"net/http"
)

type WebInboundPort struct {
	Address             string
	Port                int
	ControlPlaneService *service.ControlPlaneService
}

func NewWebInboundPort(address string, port int, controlPlaneService *service.ControlPlaneService) *WebInboundPort {
	return &WebInboundPort{
		Address:             address,
		Port:                port,
		ControlPlaneService: controlPlaneService,
	}
}

func (wip *WebInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", wip.IndexHandler)
	mux.HandleFunc("GET /discover", wip.DiscoverHandler)

	return http.NewCrossOriginProtection().Handler(mux)
}

func (wip *WebInboundPort) IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}

func (wip *WebInboundPort) DiscoverHandler(w http.ResponseWriter, r *http.Request) {
	loginHint := r.URL.Query().Get("login_hint")
	if loginHint == "" {
		log.Printf("Missing login_hint parameter in request or login_hint is empty: %v", r.URL)
		http.Error(w, "Missing login_hint parameter or login_hint is empty", http.StatusBadRequest)
		return
	}
	realm, err := wip.ControlPlaneService.DiscoverRealm(loginHint)
	if err != nil {
		log.Printf("Error discovering realm for login hint '%s': %v", loginHint, err)
		http.Error(w, "Internal Server Error", http.StatusNotFound)
		return
	}
	w.Write([]byte(realm))
}
