package messaging

import (
	"auth-svc/internal/service"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type NatsInboundPort struct {
	Nc  *nats.Conn
	Svc *service.AuthService
}

func (nip *NatsInboundPort) Subscribe(topic string) {
	nip.Nc.Subscribe(topic, func(m *nats.Msg) {
		ctx := context.Background()
		fmt.Printf("Received a message: %s\n", string(m.Data))
		userRegistrationEvent := service.UserRegisteredEvent{}
		err := json.Unmarshal(m.Data, &userRegistrationEvent)
		if err != nil {
			fmt.Printf("Error unmarshalling message: %v\n", err)
			return
		}
		nip.Svc.RegisterUserEvent(ctx, userRegistrationEvent)
	})
}
