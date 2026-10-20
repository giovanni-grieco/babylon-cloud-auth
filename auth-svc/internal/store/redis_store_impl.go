package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"auth-svc/internal/httpin"
	"auth-svc/internal/service"
)

const (
	stateKeyPrefix   = "auth:state:"
	sessionKeyPrefix = "auth:session:"
)

var (
	ErrStateNotFound   = errors.New("state not found")
	ErrSessionNotFound = errors.New("session not found")
)

// Compile-time proof that RedisStore implements both store ports.
var (
	_ httpin.StateStore    = (*RedisStore)(nil)
	_ service.SessionStore = (*RedisStore)(nil)
)

type RedisStoreConfig struct {
	Address  string
	Password string
	DB       int
	// SessionTTL is applied to newly created sessions. Non-positive values keep
	// the session until DeleteSession is called.
	SessionTTL time.Duration
}

type RedisStore struct {
	client     *redis.Client
	sessionTTL time.Duration
}

func NewRedisStore(config RedisStoreConfig) *RedisStore {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
		Protocol: 2,
	})

	return &RedisStore{client: client, sessionTTL: config.SessionTTL}
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

// http.StateStore implementation.

func (s *RedisStore) SetState(ctx context.Context, key, value string, expiration time.Duration) error {
	if err := s.client.Set(ctx, stateKeyPrefix+key, value, expiration).Err(); err != nil {
		return fmt.Errorf("set state %q: %w", key, err)
	}
	return nil
}

func (s *RedisStore) GetState(ctx context.Context, key string) (string, error) {
	value, err := s.client.Get(ctx, stateKeyPrefix+key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		return "", ErrStateNotFound
	case err != nil:
		return "", fmt.Errorf("get state %q: %w", key, err)
	}
	return value, nil
}

func (s *RedisStore) DeleteState(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, stateKeyPrefix+key).Err(); err != nil {
		return fmt.Errorf("delete state %q: %w", key, err)
	}
	return nil
}

// service.SessionStore implementation.

func (s *RedisStore) CreateSession(ctx context.Context, sessionID string, userID uint64) error {
	if sessionID == "" {
		return errors.New("create session: empty session id")
	}

	value := strconv.FormatUint(userID, 10)
	if err := s.client.Set(ctx, sessionKeyPrefix+sessionID, value, s.sessionTTL).Err(); err != nil {
		return fmt.Errorf("create session %q: %w", sessionID, err)
	}
	return nil
}

func (s *RedisStore) GetSession(ctx context.Context, sessionID string) (uint64, error) {
	value, err := s.client.Get(ctx, sessionKeyPrefix+sessionID).Result()
	switch {
	case errors.Is(err, redis.Nil):
		return 0, ErrSessionNotFound
	case err != nil:
		return 0, fmt.Errorf("get session %q: %w", sessionID, err)
	}

	userID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("get session %q: malformed user id %q: %w", sessionID, value, err)
	}
	return userID, nil
}

func (s *RedisStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.client.Del(ctx, sessionKeyPrefix+sessionID).Err(); err != nil {
		return fmt.Errorf("delete session %q: %w", sessionID, err)
	}
	return nil
}
