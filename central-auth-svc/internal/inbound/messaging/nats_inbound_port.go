package messaging

import (
	"central-auth-svc/internal/service"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// RealmUsersStream keeps the latest username -> realm event per user.
// Its config must stay identical to the one in auth-svc: both services
// create it, whichever starts first wins.
var RealmUsersStream = jetstream.StreamConfig{
	Name:              "REALM_USERS",
	Subjects:          []string{service.UserRealmRegisteredSubjectPrefix + ".>"},
	Storage:           jetstream.FileStorage,
	MaxMsgsPerSubject: 1,
}

// Durable: the server remembers what this service has acked, so after a
// restart it resumes where it left off, and on first start it replays all.
const realmUsersConsumer = "central-auth"

type NatsInboundPort struct {
	Js  jetstream.JetStream
	Svc *service.CentralAuthSvc
}

func (nip *NatsInboundPort) Consume(ctx context.Context) (jetstream.ConsumeContext, error) {
	_, err := nip.Js.CreateStream(ctx, RealmUsersStream)
	if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
		return nil, fmt.Errorf("stream %s exists with a different config: %w", RealmUsersStream.Name, err)
	}
	if err != nil {
		return nil, fmt.Errorf("create stream %s: %w", RealmUsersStream.Name, err)
	}

	cons, err := nip.Js.CreateOrUpdateConsumer(ctx, RealmUsersStream.Name, jetstream.ConsumerConfig{
		Durable:       realmUsersConsumer,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: service.UserRealmRegisteredSubjectPrefix + ".>",
	})
	if err != nil {
		return nil, fmt.Errorf("create consumer %s: %w", realmUsersConsumer, err)
	}

	return cons.Consume(nip.handle)
}

func (nip *NatsInboundPort) handle(m jetstream.Msg) {
	fmt.Printf("Received a message: %s\n", string(m.Data()))
	event := service.UserRealmRegisteredEvent{}
	if err := json.Unmarshal(m.Data(), &event); err != nil {
		// Malformed messages will never succeed: stop redelivering them.
		fmt.Printf("Error unmarshalling message: %v\n", err)
		m.Term()
		return
	}
	if event.Data.Username == "" || event.Data.RealmID == "" {
		fmt.Printf("Discarding event %s: missing username or realm_id\n", event.ID)
		m.Term()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := nip.Svc.NewUserRealmEvent(ctx, event.Data.Username, event.Data.RealmID); err != nil {
		// Transient (e.g. Redis down): ask for redelivery.
		fmt.Printf("Error storing user realm for event %s: %v\n", event.ID, err)
		m.Nak()
		return
	}
	m.Ack()
}
