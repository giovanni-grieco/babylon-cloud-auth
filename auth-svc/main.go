package main

import (
	"auth-svc/internal/inbound/httpin"
	in_messaging "auth-svc/internal/inbound/messaging"
	out_messaging "auth-svc/internal/outbound/messaging"
	"auth-svc/internal/registry"
	"auth-svc/internal/service"
	"auth-svc/internal/store"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Embeds the IANA time zone database
	"uuid"

	"github.com/nats-io/nats.go"
)

const (
	serviceName = "auth"
	servicePort = 8080
)

func main() {

	serviceUniqueId := uuid.New().String()

	ctx := context.Background()

	redisStore := store.NewRedisStore(store.RedisStoreConfig{
		Address:    "auth-kv:6379",
		Password:   "",
		DB:         0,
		SessionTTL: 24 * time.Hour, // Set session TTL to 24 hours
	})

	postgresStore, err := store.NewPostgresStore(store.PostgresStoreConfig{
		Host:     "auth-db",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "auth-db",
		Timezone: "Europe/Rome",
	})

	if err != nil {
		panic(err)
	}

	err = postgresStore.Migrate(ctx)

	if err != nil {
		panic(err)
	}

	natsConn, err := nats.Connect("nats://message-broker:4222")
	if err != nil {
		panic(err)
	}
	defer natsConn.Close()

	natsOutboundClient := &out_messaging.NatsOutboundMessageClientImpl{
		Nc: natsConn,
	}

	authSvc := service.NewAuthService(serviceUniqueId, postgresStore, redisStore, natsOutboundClient)

	httpInboundPort := httpin.New(
		httpin.HttpInboundPortConfig{
			Domain:        envOr("AUTH_DOMAIN", "localhost"),
			SecureCookies: envOr("COOKIE_SECURE", "false") == "true",
		},
		authSvc,
		redisStore)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", servicePort),
		Handler: httpInboundPort.Routes(),
	}

	natsInboundPort := &in_messaging.NatsInboundPort{
		Nc:  natsConn,
		Svc: authSvc,
	}

	natsInboundPort.Subscribe("auth.user.registered.v1")

	reg := registry.New(
		envOr("CONSUL_ADDR", "consul"),
		envOr("CONSUL_PORT", "8500"),
		serviceName,
		servicePort,
	)

	go reg.RegisterLoop()

	shutdownCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		<-shutdownCtx.Done()
		reg.Deregister()

		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(timeoutCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("Auth Service running on :%d\n", servicePort)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
