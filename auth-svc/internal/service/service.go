package service

import (
	"auth-svc/internal/domain"
	"auth-svc/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type AuthService struct {
	SvcName       string
	RealmID       string
	UserStore     UserStore
	SessionStore  SessionStore
	MessageClient OutboundMessageClient
	Argon2idHash  utils.Argon2idHash
}

func NewAuthService(SvcName string, realmID string, userStore UserStore, sessionStore SessionStore, messageClient OutboundMessageClient) *AuthService {
	return &AuthService{
		SvcName:       SvcName,
		RealmID:       realmID,
		UserStore:     userStore,
		SessionStore:  sessionStore,
		MessageClient: messageClient,
		Argon2idHash:  utils.NewArgon2idHash(utils.Argon2idHashConfigDefault()),
	}
}

func (s *AuthService) Verify(ctx context.Context, cookie string) bool {
	return cookie == "auth:1234"
}

func (s *AuthService) Login(ctx context.Context, username, password string) error {
	user, err := s.UserStore.GetUser(ctx, username)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if err := s.Argon2idHash.Compare(user.PasswordHash, user.Salt, password); err != nil {
		return fmt.Errorf("invalid username or password")
	}

	return nil
}

func (s *AuthService) Register(
	ctx context.Context,
	username string,
	email string,
	password string,
) error {
	salt, err := s.Argon2idHash.GenerateSalt()
	if err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	hashAndSalt, err := s.Argon2idHash.GeneratePasswordHash(
		[]byte(password),
		salt,
	)
	if err != nil {
		return fmt.Errorf("generate password hash: %w", err)
	}

	newUser := domain.User{
		Username:     username,
		Email:        email,
		PasswordHash: hashAndSalt.Hash,
		Salt:         hashAndSalt.Salt,
	}

	if err := s.UserStore.CreateUser(ctx, newUser); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	event, err := NewUserRealmRegisteredEvent(
		username,
		s.RealmID,
		time.Now(),
		s.SvcName,
	)
	if err != nil {
		return fmt.Errorf("create user realm registered event: %w", err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal user realm registered event: %w", err)
	}

	if s.MessageClient == nil {
		return fmt.Errorf("publish user realm registered event: message client is nil")
	}

	if err := s.MessageClient.Send(
		UserRealmRegisteredSubject,
		string(payload),
	); err != nil {
		return fmt.Errorf("publish user realm registered event: %w", err)
	}

	return nil
}
