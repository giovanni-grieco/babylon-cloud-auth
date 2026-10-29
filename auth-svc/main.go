package main

import (
	"auth-svc/internal/inbound/httpin"
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
	"github.com/nats-io/nats.go/jetstream"
)

const (
	serviceName = "auth"
	servicePort = 8080
)

func main() {

	serviceUniqueId := uuid.New().String()

	realmID := os.Getenv("REALM_ID")
	if realmID == "" {
		log.Fatal("REALM_ID must be set")
	}

	ctx := context.Background()

	redisStore := store.NewRedisStore(store.RedisStoreConfig{
		Address:    "state-kv:6379",
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

	js, err := jetstream.New(natsConn)
	if err != nil {
		panic(err)
	}

	if err := out_messaging.EnsureRealmUsersStream(ctx, js); err != nil {
		panic(err)
	}

	natsOutboundClient := &out_messaging.NatsOutboundMessageClientImpl{
		Js: js,
	}

	authSvc := service.NewAuthService(serviceUniqueId, realmID, postgresStore, postgresStore, natsOutboundClient)

	// Backfill the stream with every user in this realm. Async so the HTTP
	// server isn't held up.
	go func() {
		announceCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		count, err := authSvc.AnnounceRealmUsers(announceCtx)
		if err != nil {
			log.Printf("announce realm users: %v (announced %d)", err, count)
			return
		}
		log.Printf("announced %d users of realm %s", count, realmID)
	}()

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
