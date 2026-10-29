package main

import (
	"auth-svc/internal/inbound/httpin"
	out_messaging "auth-svc/internal/outbound/messaging"
	"auth-svc/internal/registry"
	"auth-svc/internal/service"
	"auth-svc/internal/store"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // Embeds the IANA time zone database
	"uuid"

	"github.com/golang-jwt/jwt/v5"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	serviceName = "auth"
	servicePort = 8080
)

// loadPublicKeys parses every "<keyID>.pub.pem" file in dir, keyed by keyID.
// Anything else in the folder (e.g. macOS .DS_Store) is skipped.
func loadPublicKeys(dir string) (map[string]ed25519.PublicKey, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read public keys dir: %w", err)
	}

	keys := make(map[string]ed25519.PublicKey)
	for _, file := range files {
		keyID, ok := strings.CutSuffix(file.Name(), ".pub.pem")
		if file.IsDir() || !ok {
			continue
		}

		keyPath := filepath.Join(dir, file.Name())
		pemBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read public key %s: %w", keyPath, err)
		}
		key, err := jwt.ParseEdPublicKeyFromPEM(pemBytes)
		if err != nil {
			return nil, fmt.Errorf("parse public key %s: %w", keyPath, err)
		}
		keys[keyID] = key.(ed25519.PublicKey)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no *.pub.pem files in %s", dir)
	}
	return keys, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}
	key, err := jwt.ParseEdPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", path, err)
	}
	return key.(ed25519.PrivateKey), nil
}

func main() {

	serviceUniqueId := uuid.New().String()

	realmID := os.Getenv("REALM_ID")
	if realmID == "" {
		log.Fatal("REALM_ID must be set")
	}

	privateKeyPath := os.Getenv("JWT_PRIVATE_KEY_PATH")
	if privateKeyPath == "" {
		log.Fatal("JWT_PRIVATE_KEY_PATH must be set")
	}

	publicKeysPath := os.Getenv("JWT_PUBLIC_KEY_PATH")
	if publicKeysPath == "" {
		log.Fatal("JWT_PUBLIC_KEY_PATH must be set")
	}

	publicKeys, err := loadPublicKeys(publicKeysPath)
	if err != nil {
		log.Fatalf("failed to load public keys: %v", err)
	}

	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		log.Fatalf("failed to load private key: %v", err)
	}

	// we check if exists a public key for this realm and if it matches the private key, otherwise we panic
	if pub, ok := publicKeys[realmID]; !ok || !pub.Equal(privateKey.Public()) {
		log.Fatalf("public key %q missing or not matching the private key", realmID)
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

	authSvc := service.NewAuthService(serviceUniqueId, realmID, postgresStore, postgresStore, natsOutboundClient, publicKeys, privateKey)

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
			RealmID:       realmID,
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
