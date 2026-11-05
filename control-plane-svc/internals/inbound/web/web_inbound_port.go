package web

import (
	"control-plane-svc/internals/service"
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
	return http.NewCrossOriginProtection().Handler(mux)
}

func (wip *WebInboundPort) IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}
