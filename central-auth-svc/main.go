package main

import (
	"central-auth-svc/internal/inbound/messaging"
	"central-auth-svc/internal/inbound/web"
	"central-auth-svc/internal/service"
	"central-auth-svc/internal/store"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	serviceName = "central-auth"
	servicePort = 80
)

func main() {
	redisStore := store.NewRedisStore(store.RedisStoreConfig{
		Address:    "auth-kv:6379",
		Password:   "",
		DB:         0,
		SessionTTL: 24 * time.Hour, // Set session TTL to 24 hours
	})

	svc := &service.CentralAuthSvc{
		RealmStore: redisStore,
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

	inboundMessagePort := messaging.NatsInboundPort{
		Js:  js,
		Svc: svc,
	}

	consumeCtx, err := inboundMessagePort.Consume(context.Background())
	if err != nil {
		panic(err)
	}
	defer consumeCtx.Stop()

	httpInboundPort := web.New(web.HttpInboundPortConfig{
		Domain:        "localhost",
		SecureCookies: true,
	}, svc)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", servicePort),
		Handler: httpInboundPort.Routes(),
	}

	log.Printf("%s running on :%d\n", serviceName, servicePort)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}

}
