package main

import (
	"central-auth-svc/internal/inbound/messaging"
	"central-auth-svc/internal/service"
	"central-auth-svc/internal/store"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	redisStore := store.NewRedisStore(store.RedisStoreConfig{
		Address:    "central-auth-kv:6379",
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

	inboundMessagePort := messaging.NatsInboundPort{
		Nc:  natsConn,
		Svc: svc,
	}

	inboundMessagePort.Subscribe("realms")

}
