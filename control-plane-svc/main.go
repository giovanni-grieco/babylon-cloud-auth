package main

import (
	"control-plane-svc/internals/inbound/rest"
	"control-plane-svc/internals/inbound/web"
	"control-plane-svc/internals/service"
	"control-plane-svc/internals/store"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

type Configuration struct {
	Address            string
	WebPort            int
	RestPort           int
	ControlPlaneConfig string
	ControlPlaneSecret string
	PostgresHost       string
	PostgresPort       string
	PostgresUser       string
	PostgresDb         string
	PostgresPassword   string
	PostgresSSLMode    string
	Timezone           string
}

func envOr(envName string) string {
	envVarValue := os.Getenv(envName)
	if envVarValue == "" {
		panic(fmt.Errorf("Env variable %s is not set\n", envName))
	}
	return envVarValue
}

func LoadConfig() Configuration {
	var cfg Configuration
	cfg.Address = envOr("ADDRESS")

	webPortString := envOr("WEB_PORT")
	webPortInt, err := strconv.Atoi(webPortString)
	if err != nil {
		panic(fmt.Errorf("Failed to convert env variable"))
	}
	cfg.WebPort = webPortInt

	restPortString := envOr("REST_PORT")
	restPortInt, err := strconv.Atoi(restPortString)
	if err != nil {
		panic(fmt.Errorf("Failed to convert env variable"))
	}
	cfg.RestPort = restPortInt

	cfg.ControlPlaneConfig = envOr("CP_CONFIG")
	cfg.ControlPlaneSecret = envOr("CONTROL_PLANE_SECRET")
	cfg.PostgresHost = envOr("POSTGRES_HOST")
	cfg.PostgresPort = envOr("POSTGRES_PORT")
	cfg.PostgresUser = envOr("POSTGRES_USER")
	cfg.PostgresDb = envOr("POSTGRES_DB")
	cfg.PostgresPassword = envOr("POSTGRES_PASSWORD")
	cfg.PostgresSSLMode = envOr("POSTGRES_SSL_MODE")
	cfg.Timezone = envOr("TIMEZONE")
	return cfg
}

func main() {
	cfg := LoadConfig()
	log.Printf("Starting control plane")

	postgresStore, err := store.NewPostgresStore(
		cfg.PostgresHost,
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresDb,
		cfg.PostgresPort,
		cfg.PostgresSSLMode,
		cfg.Timezone,
	)

	if err != nil {
		panic(err)
	}

	controlPlaneService := service.NewControlPlanceService(postgresStore)

	webInboundPort := web.NewWebInboundPort(cfg.Address, cfg.WebPort, controlPlaneService)
	restInboundPort := rest.NewRestInboundPort(cfg.Address, cfg.RestPort, controlPlaneService)

	httpServer := &http.Server{
		Addr:    webInboundPort.Address + ":" + strconv.Itoa(webInboundPort.Port),
		Handler: webInboundPort.Routes(),
	}

	restHttpServer := &http.Server{
		Addr:    restInboundPort.Address + ":" + strconv.Itoa(restInboundPort.Port),
		Handler: restInboundPort.Routes(),
	}

	go func() {
		log.Printf("rest api server started %s:%d\n", cfg.Address, cfg.RestPort)
		if err := restHttpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	log.Printf("web server started %s:%d\n", cfg.Address, cfg.WebPort)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
