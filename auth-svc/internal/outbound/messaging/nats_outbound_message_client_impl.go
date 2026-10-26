package messaging

import (
	"auth-svc/internal/service"
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// Implementation for the messaging interface owned by the application layer

var (
	_ service.OutboundMessageClient = (*NatsOutboundMessageClientImpl)(nil)
)

// RealmUsersStream keeps the latest username -> realm event per user.
// Its config must stay identical to the one in central-auth-svc: both services
// create it, whichever starts first wins.
var RealmUsersStream = jetstream.StreamConfig{
	Name:              "REALM_USERS",
	Subjects:          []string{service.UserRealmRegisteredSubjectPrefix + ".>"},
	Storage:           jetstream.FileStorage,
	MaxMsgsPerSubject: 1,
}

func EnsureRealmUsersStream(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.CreateStream(ctx, RealmUsersStream)
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("stream %s exists with a different config: %w", RealmUsersStream.Name, err)
	}
	return err
}

type NatsOutboundMessageClientImpl struct {
	Js jetstream.JetStream
}

func (nats *NatsOutboundMessageClientImpl) Publish(ctx context.Context, subject string, msgID string, payload []byte) error {
	_, err := nats.Js.Publish(ctx, subject, payload, jetstream.WithMsgID(msgID))
	return err
}
