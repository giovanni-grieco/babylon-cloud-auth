package main

import (
	"control-plane-svc/internals/inbound/web"
	"control-plane-svc/internals/service"
	"errors"
	"log"
	"net/http"
	"strconv"
)

const (
	address = "control-plane.babylon.test"
	port    = 8080
)

func main() {
	log.Printf("Starting control plane service on %s:%d", address, port)
	controlPlaneService := service.NewControlPlanceService()

	webInboundPort := web.NewWebInboundPort(address, port, controlPlaneService)

	httpServer := &http.Server{
		Addr:    webInboundPort.Address + ":" + strconv.Itoa(webInboundPort.Port),
		Handler: webInboundPort.Routes(),
	}

	log.Printf("%s running on :%d\n", address, port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
