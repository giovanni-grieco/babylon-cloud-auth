package service

import (
	"auth-svc/internal/domain"
	"auth-svc/internal/utils"
	"context"
	"fmt"
)

type AuthService struct {
	UserStore    UserStore
	SessionStore SessionStore
	Argon2idHash utils.Argon2idHash
}

func NewAuthService(userStore UserStore, sessionStore SessionStore) *AuthService {
	return &AuthService{
		UserStore:    userStore,
		SessionStore: sessionStore,
		Argon2idHash: utils.NewArgon2idHash(utils.Argon2idHashConfigDefault()),
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

func (s *AuthService) Register(ctx context.Context, username, email, password string) error {

	salt, err := s.Argon2idHash.GenerateSalt()
	if err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}
	hashAndSalt, err := s.Argon2idHash.GeneratePasswordHash([]byte(password), salt)
	if err != nil {
		return fmt.Errorf("generate password hash: %w", err)
	}

	newUser := domain.User{
		Username:     username,
		Email:        email,
		PasswordHash: hashAndSalt.Hash,
		Salt:         hashAndSalt.Salt,
	}

	return s.UserStore.CreateUser(ctx, newUser)
}
