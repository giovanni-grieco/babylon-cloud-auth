package messaging

import (
	"central-auth-svc/internal/service"
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type NatsInboundPort struct {
	Nc  *nats.Conn
	Svc *service.CentralAuthSvc
}

func (nip *NatsInboundPort) Subscribe(topic string) {
	nip.Nc.Subscribe(topic, func(m *nats.Msg) {
		ctx := context.Background()
		fmt.Printf("Received a message: %s\n", string(m.Data))
		event := service.UserRealmRegisteredEvent{}
		err := json.Unmarshal(m.Data, &event)
		if err != nil {
			fmt.Printf("Error unmarshalling message: %v\n", err)
			return
		}
		if event.Data.Username == "" || event.Data.RealmID == "" {
			fmt.Printf("Discarding event %s: missing username or realm_id\n", event.ID)
			return
		}
		if err := nip.Svc.NewUserRealmEvent(ctx, event.Data.Username, event.Data.RealmID); err != nil {
			fmt.Printf("Error storing user realm for event %s: %v\n", event.ID, err)
		}
	})
}
