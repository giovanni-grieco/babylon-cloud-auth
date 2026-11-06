package rest

import (
	"control-plane-svc/internals/service"
	"encoding/json"
	"fmt"
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

func (rip *RestInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/discover", rip.DiscoverPostHandler)

	return http.NewCrossOriginProtection().Handler(mux)
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
	jsonMarshalled, err := json.Marshal(discoveryDto)
	fmt.Printf("Discover Dto: %v\n", string(jsonMarshalled))
	idpAlias, err := rip.ControlPlaneService.DiscoverRealm(ctx, discoveryDto.LoginHint)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(DiscoveryResponseDto{Idp: idpAlias})
}
