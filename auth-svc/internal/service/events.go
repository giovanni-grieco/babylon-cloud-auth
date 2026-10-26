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
	UserRealmRegisteredEventType     = "realm.user.registered"
	UserRealmRegisteredSubjectPrefix = "realm.user.registered.v1"
	eventVersion                     = 1
)

// UserRealmRegisteredSubject gives every user its own subject, so the stream
// can keep only the latest event per user. Usernames are base64url-encoded
// because subject tokens can't contain '.', '*', '>' or whitespace.
func UserRealmRegisteredSubject(username string) string {
	return UserRealmRegisteredSubjectPrefix + "." + base64.RawURLEncoding.EncodeToString([]byte(username))
}

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
