package main

import (
	"control-plane-svc/internals/clients"
	"control-plane-svc/internals/inbound/rest"
	"control-plane-svc/internals/inbound/web"
	"control-plane-svc/internals/service"
	"control-plane-svc/internals/store"
	"control-plane-svc/internals/utils"
	"errors"
	"fmt"
	"log"
	"net/http"
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

func LoadConfig() Configuration {
	var cfg Configuration
	cfg.Address = utils.EnvOr("ADDRESS")

	webPortString := utils.EnvOr("WEB_PORT")
	webPortInt, err := strconv.Atoi(webPortString)
	if err != nil {
		panic(fmt.Errorf("Failed to convert env variable"))
	}
	cfg.WebPort = webPortInt

	restPortString := utils.EnvOr("REST_PORT")
	restPortInt, err := strconv.Atoi(restPortString)
	if err != nil {
		panic(fmt.Errorf("Failed to convert env variable"))
	}
	cfg.RestPort = restPortInt

	cfg.ControlPlaneConfig = utils.EnvOr("CP_CONFIG")
	cfg.ControlPlaneSecret = utils.EnvOr("CONTROL_PLANE_SECRET")
	cfg.PostgresHost = utils.EnvOr("POSTGRES_HOST")
	cfg.PostgresPort = utils.EnvOr("POSTGRES_PORT")
	cfg.PostgresUser = utils.EnvOr("POSTGRES_USER")
	cfg.PostgresDb = utils.EnvOr("POSTGRES_DB")
	cfg.PostgresPassword = utils.EnvOr("POSTGRES_PASSWORD")
	cfg.PostgresSSLMode = utils.EnvOr("POSTGRES_SSL_MODE")
	cfg.Timezone = utils.EnvOr("TIMEZONE")
	return cfg
}

func main() {
	cfg := LoadConfig()
	realmsCfg := clients.LoadRealmConfig(cfg.ControlPlaneConfig)
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

	/*ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()*/

	//keycloakClientCentral := clients.NewKeycloakClient(realmsCfg["central"])
	keycloakClientDC1 := clients.NewKeycloakClientFromCfg(realmsCfg["dc-1"])
	keycloakClientDC2 := clients.NewKeycloakClientFromCfg(realmsCfg["dc-2"])

	users, err := keycloakClientDC1.GetAllUsers()
	if err != nil {
		panic(err)
	}

	for _, user := range users {
		fmt.Println(user.ToString())
	}

	users, err = keycloakClientDC2.GetAllUsers()
	if err != nil {
		panic(err)
	}

	for _, user := range users {
		fmt.Println(user.ToString())
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
