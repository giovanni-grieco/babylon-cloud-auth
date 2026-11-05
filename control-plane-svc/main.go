package main

import (
	"control-plane-svc/internals/inbound/rest"
	"control-plane-svc/internals/inbound/web"
	"control-plane-svc/internals/service"
	"errors"
	"log"
	"net/http"
	"strconv"
)

const (
	address  = "0.0.0.0"
	webport  = 8080
	restport = 8081
)

func main() {
	log.Printf("Starting control plane")
	controlPlaneService := service.NewControlPlanceService()

	webInboundPort := web.NewWebInboundPort(address, webport, controlPlaneService)
	restInboundPort := rest.NewRestInboundPort(address, restport, controlPlaneService)

	httpServer := &http.Server{
		Addr:    webInboundPort.Address + ":" + strconv.Itoa(webInboundPort.Port),
		Handler: webInboundPort.Routes(),
	}

	restHttpServer := &http.Server{
		Addr:    restInboundPort.Address + ":" + strconv.Itoa(restInboundPort.Port),
		Handler: restInboundPort.Routes(),
	}

	go func() {
		log.Printf("rest api server started %s:%d\n", address, restport)
		if err := restHttpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	log.Printf("web server started %s:%d\n", address, webport)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
