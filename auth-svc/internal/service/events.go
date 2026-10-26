package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// Announces only the username -> realm association, so the central auth
// service can do home realm discovery. Credentials and PII never leave the realm.
const (
	UserRealmRegisteredEventType = "realm.user.registered"
	UserRealmRegisteredSubject   = "realm.user.registered.v1"
	eventVersion                 = 1
)

type UserRealmRegisteredEvent struct {
	ID         string                       `json:"id"`
	Type       string                       `json:"type"`
	Version    int                          `json:"version"`
	OccurredAt time.Time                    `json:"occurred_at"`
	Producer   string                       `json:"producer"`
	Data       UserRealmRegisteredEventData `json:"data"`
}

type UserRealmRegisteredEventData struct {
	Username string `json:"username"`
	RealmID  string `json:"realm_id"`
}

func NewUserRealmRegisteredEvent(
	username string,
	realmID string,
	occurredAt time.Time,
	eventProducer string,
) (UserRealmRegisteredEvent, error) {
	eventIDBytes := make([]byte, 16)
	if _, err := rand.Read(eventIDBytes); err != nil {
		return UserRealmRegisteredEvent{}, fmt.Errorf("generate event id: %w", err)
	}

	return UserRealmRegisteredEvent{
		ID:         base64.RawURLEncoding.EncodeToString(eventIDBytes),
		Type:       UserRealmRegisteredEventType,
		Version:    eventVersion,
		OccurredAt: occurredAt.UTC(),
		Producer:   eventProducer,
		Data: UserRealmRegisteredEventData{
			Username: username,
			RealmID:  realmID,
		},
	}, nil
}
