package main

import (
	"auth-svc/internal/httpin"
	"auth-svc/internal/registry"
	"auth-svc/internal/service"
	"auth-svc/internal/store"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Embeds the IANA time zone database
)

const (
	serviceName = "auth"
	servicePort = 8080
)

func main() {
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

	authSvc := service.NewAuthService(postgresStore, redisStore)

	httpInboundPort := httpin.New(
		httpin.HttpInboundPortConfig{
			Domain: envOr("AUTH_DOMAIN", "localhost"),
		},
		authSvc,
		redisStore)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpInboundPort.Routes(),
	}

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
