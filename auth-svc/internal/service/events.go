package service

import (
	"auth-svc/internal/domain"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

const (
	UserRegisteredEventType = "auth.user.registered"
	UserRegisteredSubject   = "auth.user.registered.v1"
	eventVersion            = 1
)

type UserRegisteredEvent struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Version    int                     `json:"version"`
	OccurredAt time.Time               `json:"occurred_at"`
	Producer   string                  `json:"producer"`
	Data       UserRegisteredEventData `json:"data"`
}

type UserRegisteredEventData struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Salt     string `json:"salt"`
}

func NewUserRegisteredEvent(
	user domain.User,
	occurredAt time.Time,
	eventProducer string,
) (UserRegisteredEvent, error) {
	eventIDBytes := make([]byte, 16)
	if _, err := rand.Read(eventIDBytes); err != nil {
		return UserRegisteredEvent{}, fmt.Errorf("generate event id: %w", err)
	}

	return UserRegisteredEvent{
		ID:         base64.RawURLEncoding.EncodeToString(eventIDBytes),
		Type:       UserRegisteredEventType,
		Version:    eventVersion,
		OccurredAt: occurredAt.UTC(),
		Producer:   eventProducer,
		Data: UserRegisteredEventData{
			UserID:   user.ID,
			Username: user.Username,
			Email:    user.Email,
			Password: user.PasswordHash,
			Salt:     user.Salt,
		},
	}, nil
}
