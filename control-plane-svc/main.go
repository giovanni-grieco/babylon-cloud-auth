// Command control-plane-svc is the operator console for provisioning B2B
// accounts into DC realms.
//
// It's built as ports and adapters: internal/domain holds the rules,
// internal/app the use cases and their ports, and internal/adapters the
// Keycloak client (driven) and the HTML console (driving). This file only wires
// them together.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"control-plane-svc/internal/adapters/keycloak"
	"control-plane-svc/internal/adapters/password"
	"control-plane-svc/internal/adapters/web"
	"control-plane-svc/internal/app"
	"control-plane-svc/internal/config"
	"control-plane-svc/internal/domain"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(envOr("CP_CONFIG", "config.json"))
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	conn := func(k config.Keycloak) keycloak.Connection {
		return keycloak.Connection{BaseURL: k.BaseURL, Realm: k.Realm, ClientID: k.ClientID, ClientSecret: k.Secret}
	}

	realms := make([]domain.Realm, len(cfg.Realms))
	clients := make(map[domain.RealmID]*keycloak.Client, len(cfg.Realms))
	for i, r := range cfg.Realms {
		id := domain.RealmID(r.ID)
		realms[i] = domain.Realm{ID: id, Name: r.Name}
		clients[id] = keycloak.NewClient(conn(r.Keycloak), httpClient)
	}

	prov := app.NewProvisioner(
		realms,
		keycloak.NewStore(clients),
		keycloak.NewBroker(keycloak.NewClient(conn(cfg.Central), httpClient)),
		password.Generator{},
	)

	srv := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           web.NewServer(prov, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// A large import makes one Keycloak call per user.
		WriteTimeout: 10 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "realms", len(realms))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
