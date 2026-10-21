package service

import (
	"auth-svc/internal/domain"
	"context"
	"fmt"
)

type AuthService struct {
	UserStore    UserStore
	SessionStore SessionStore
}

func NewAuthService(userStore UserStore, sessionStore SessionStore) *AuthService {
	return &AuthService{
		UserStore:    userStore,
		SessionStore: sessionStore,
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

	if user.PasswordHash != password {
		return fmt.Errorf("invalid username or password")
	}

	return nil
}

func (s *AuthService) Register(ctx context.Context, username, email, password string) error {
	newUser := domain.User{
		Username:     username,
		Email:        email,
		PasswordHash: password,
	}
	return s.UserStore.CreateUser(ctx, newUser)
}
