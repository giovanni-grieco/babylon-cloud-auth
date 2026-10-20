package service

import (
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
	if username == "admin" && password == "admin" {
		return nil
	}
	return fmt.Errorf("invalid username or password")
}

func (s *AuthService) Register(ctx context.Context, username, email, password string) error {
	return fmt.Errorf("registration not implemented")
}
