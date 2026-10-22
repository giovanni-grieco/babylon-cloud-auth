package messaging

import (
	"auth-svc/internal/service"

	"github.com/nats-io/nats.go"
)

// Implementation for the messaging interface owned by the application layer

var (
	_ service.OutboundMessageClient = (*NatsOutboundMessageClientImpl)(nil)
)

type NatsOutboundMessageClientImpl struct {
	Nc *nats.Conn
}

func (nats *NatsOutboundMessageClientImpl) Send(topic, message string) error {
	return nats.Nc.Publish(topic, []byte(message))
}
