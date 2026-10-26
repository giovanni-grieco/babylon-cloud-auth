package store

import (
	"central-auth-svc/internal/service"
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	realmPrefix = "realmpfx:"
)

var (
	ErrStateNotFound   = errors.New("state not found")
	ErrSessionNotFound = errors.New("session not found")
)

// Compile-time proof that RedisStore implements both store ports.
var (
	_ service.RealmStore = (*RedisStore)(nil)
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

func (s *RedisStore) GetRealm(ctx context.Context, username string) (string, error) {
	return s.client.Get(ctx, realmPrefix+username).Result()
}

func (s *RedisStore) SetRealm(ctx context.Context, username, realm_id string) error {
	return s.client.Set(ctx, realmPrefix+username, realm_id, 0).Err()
}
