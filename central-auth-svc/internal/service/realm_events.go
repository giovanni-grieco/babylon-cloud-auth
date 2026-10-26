package service

import "time"

// Mirrors the event published by every auth-svc that allows registration,
// on UserRealmRegisteredSubjectPrefix.<base64url(username)>.
const UserRealmRegisteredSubjectPrefix = "realm.user.registered.v1"

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
