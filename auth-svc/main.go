package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"auth-svc/internal/registry"
	"auth-svc/internal/service"
)

const (
	serviceName = "auth"
	servicePort = 8080
)

func main() {
	ctx := context.Background()

	rdb := redis.NewClient(&redis.Options{
		Addr:     "auth-kv:6379",
		Password: "",
		DB:       0,
		Protocol: 2,
	})

	svc := service.New(service.Config{
		Domain:   "localhost",
		Username: "admin",
		Password: "admin",
	}, rdb)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", servicePort),
		Handler: svc.Routes(),
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
		if err := srv.Shutdown(timeoutCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("Auth Service running on :%d\n", servicePort)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
