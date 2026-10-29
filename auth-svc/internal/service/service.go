package service

import (
	"common/domain"
	"common/utils"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenTTL  = 30 * time.Second
	refreshTokenTTL = 60 * time.Second
)

type AuthService struct {
	SvcName       string
	RealmID       string
	UserStore     UserStore
	SessionStore  SessionStore
	MessageClient OutboundMessageClient
	Argon2idHash  utils.Argon2idHash
	// Every known issuer's public key, indexed by key ID (= realm ID).
	publicKeys map[string]ed25519.PublicKey
	// This realm's signing key: unexported so no other package can read it.
	privateKey ed25519.PrivateKey
}

func NewAuthService(SvcName string, realmID string, userStore UserStore, sessionStore SessionStore, messageClient OutboundMessageClient, publicKeys map[string]ed25519.PublicKey, privateKey ed25519.PrivateKey) *AuthService {
	return &AuthService{
		SvcName:       SvcName,
		RealmID:       realmID,
		UserStore:     userStore,
		SessionStore:  sessionStore,
		MessageClient: messageClient,
		Argon2idHash:  utils.NewArgon2idHash(utils.Argon2idHashConfigDefault()),
		publicKeys:    publicKeys,
		privateKey:    privateKey,
	}
}

type VerifyResult struct {
	RefreshRealm string
}

func (s *AuthService) Verify(ctx context.Context, cookie string) (VerifyResult, error) {
	claims := utils.AccessClaims{}

	if err := utils.ParseJWT(cookie, s.publicKeys, &claims); err != nil {
		return VerifyResult{RefreshRealm: claims.Realm}, fmt.Errorf("parse JWT: %w", err)
	}

	return VerifyResult{RefreshRealm: claims.Realm}, nil
}

type RefreshResult struct {
	NewAccessToken  string
	NewAccessExpiry time.Time
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (RefreshResult, error) {
	session, err := s.SessionStore.GetSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("get session by refresh token: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		return RefreshResult{}, fmt.Errorf("refresh token expired")
	}

	user, err := s.UserStore.GetUser(ctx, session.UserID)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("get user by ID: %w", err)
	}

	now := time.Now()
	accessExpiry := now.Add(accessTokenTTL)
	claims := utils.AccessClaims{
		Role:  user.Role,
		Realm: s.RealmID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(user.ID, 10),
			Issuer:    s.RealmID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
		},
	}

	accessToken, err := utils.SignJWT(claims, s.RealmID, s.privateKey)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("sign access token: %w", err)
	}

	result := RefreshResult{
		NewAccessToken:  accessToken,
		NewAccessExpiry: accessExpiry,
	}

	return result, nil
}

type LoginResult struct {
	AccessToken   string
	AccessExpiry  time.Time
	RefreshToken  string
	RefreshExpiry time.Time
}

func (s *AuthService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, err := s.UserStore.GetUserByUsername(ctx, username)
	if err != nil {
		return LoginResult{}, fmt.Errorf("get user: %w", err)
	}

	if err := s.Argon2idHash.Compare(user.PasswordHash, user.Salt, password); err != nil {
		return LoginResult{}, fmt.Errorf("invalid username or password")
	}

	randomBytes, err := utils.GenerateRandomSecret(32)
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate random secret: %w", err)
	}

	randomString := fmt.Sprintf("%x", randomBytes)

	now := time.Now()
	newSession := domain.Session{
		UserID:       user.ID,
		RefreshToken: randomString,
		ExpiresAt:    now.Add(refreshTokenTTL),
	}

	if err := s.SessionStore.CreateSession(ctx, newSession); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	accessExpiry := now.Add(accessTokenTTL)
	claims := utils.AccessClaims{
		Role:  user.Role,
		Realm: s.RealmID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(user.ID, 10),
			Issuer:    s.RealmID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
		},
	}

	// The realm ID doubles as the key ID, so verifiers know which public key to use.
	accessToken, err := utils.SignJWT(claims, s.RealmID, s.privateKey)
	if err != nil {
		return LoginResult{}, fmt.Errorf("sign access token: %w", err)
	}

	result := LoginResult{
		AccessToken:   accessToken,
		AccessExpiry:  accessExpiry,
		RefreshToken:  newSession.RefreshToken,
		RefreshExpiry: newSession.ExpiresAt,
	}

	return result, nil
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
		Role:         domain.RoleUser,
	}

	if err := s.UserStore.CreateUser(ctx, newUser); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// The user now exists: publish even if the client hangs up, otherwise the
	// central auth service only learns about them at the next restart.
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.announceUserRealm(publishCtx, username)
}

// AnnounceRealmUsers publishes a realm event for every user already in this
// realm's DB. JetStream already keeps every announcement, so this is a backfill
// for users created before JetStream was in place or a recreated stream.
// Re-announcing is safe: the stream keeps one message per user.
func (s *AuthService) AnnounceRealmUsers(ctx context.Context) (int, error) {
	count := 0
	err := s.UserStore.ForEachUsername(ctx, func(username string) error {
		if err := s.announceUserRealm(ctx, username); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func (s *AuthService) announceUserRealm(ctx context.Context, username string) error {
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

	if err := s.MessageClient.Publish(
		ctx,
		UserRealmRegisteredSubject(username),
		event.ID,
		payload,
	); err != nil {
		return fmt.Errorf("publish user realm registered event: %w", err)
	}

	return nil
}
