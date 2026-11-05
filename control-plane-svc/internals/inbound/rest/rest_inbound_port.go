package rest

import (
	"control-plane-svc/internals/service"
	"net/http"
)

type RestInboundPort struct {
	Address             string
	Port                int
	ControlPlaneService *service.ControlPlaneService
}

func NewRestInboundPort(address string, port int, controlPlaneService *service.ControlPlaneService) *RestInboundPort {
	return &RestInboundPort{
		Address:             address,
		Port:                port,
		ControlPlaneService: controlPlaneService,
	}
}

func (wip *RestInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/api/discover", wip.DiscoverPostHandler)

	return http.NewCrossOriginProtection().Handler(mux)
}

func (wip *RestInboundPort) DiscoverPostHandler(w http.ResponseWriter, r *http.Request) {

	

	w.Write([]byte("{\"idp\":\"dc-1\"}"))
}
