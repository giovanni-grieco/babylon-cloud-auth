package service

import (
	"auth-svc/internal/domain"
	"auth-svc/internal/utils"
	"context"
	"fmt"
)

type AuthService struct {
	SvcName       string
	UserStore     UserStore
	SessionStore  SessionStore
	MessageClient OutboundMessageClient
	Argon2idHash  utils.Argon2idHash
}

func NewAuthService(SvcName string, userStore UserStore, sessionStore SessionStore, messageClient OutboundMessageClient) *AuthService {
	return &AuthService{
		SvcName:       SvcName,
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

	// DONT PUBLISH EVENTS FOR NOW - IN THE FUTURE WE WILL UPLOAD A NEW BLOOM FILTER TO THE CENTRAL-AUTH-SVC
	// CreateUser currently receives the user by value, so retrieve the
	// persisted record to obtain the database-generated ID.
	/*registeredUser, err := s.UserStore.GetUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("get registered user: %w", err)
	}

	event, err := NewUserRegisteredEvent(
		registeredUser,
		time.Now(),
		s.SvcName,
	)
	if err != nil {
		return fmt.Errorf("create user registered event: %w", err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal user registered event: %w", err)
	}

	if s.MessageClient == nil {
		return fmt.Errorf("publish user registered event: message client is nil")
	}

	if err := s.MessageClient.Send(
		UserRegisteredSubject,
		string(payload),
	); err != nil {
		return fmt.Errorf("publish user registered event: %w", err)
	}*/

	return nil
}

func (s *AuthService) RegisterUserEvent(ctx context.Context, event UserRegisteredEvent) error {
	if event.Producer == s.SvcName {
		// Skip processing events produced by this service to avoid duplicate user creation.
		fmt.Printf("Skipping processing of event produced by this service: %s\n", event.ID)
		return nil
	}
	user := domain.User{
		ID:           event.Data.UserID,
		Username:     event.Data.Username,
		Email:        event.Data.Email,
		PasswordHash: event.Data.Password,
		Salt:         event.Data.Salt,
	}
	return s.UserStore.CreateUser(ctx, user)
}
