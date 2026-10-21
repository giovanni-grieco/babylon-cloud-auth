package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"auth-svc/internal/domain"
	"auth-svc/internal/service"
)

var ErrUserNotFound = errors.New("user not found")

// Compile-time proof that PostgresUserStore implements the user port.
var _ service.UserStore = (*PostgresStore)(nil)

type PostgresStoreConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	Timezone string
}

type PostgresStore struct {
	db *gorm.DB
}

func NewPostgresStore(config PostgresStoreConfig) (*PostgresStore, error) {
	dsn := "host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=%s"
	dsn = fmt.Sprintf(dsn, config.Host, config.User, config.Password, config.DBName, config.Port, config.Timezone)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

// NewPostgresStoreFromDB reuses an already opened connection pool, for
// callers that configure GORM themselves.
func NewPostgresStoreFromDB(db *gorm.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Migrate creates or updates the users table. Call it once at startup.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	if err := s.db.WithContext(ctx).AutoMigrate(&userRecord{}); err != nil {
		return fmt.Errorf("migrate users table: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetUser(ctx context.Context, username string) (domain.User, error) {
	var rec userRecord

	err := s.db.WithContext(ctx).Where("username = ?", username).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.User{}, ErrUserNotFound
	case err != nil:
		return domain.User{}, fmt.Errorf("get user %q: %w", username, err)
	}

	return rec.toDomain(), nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var rec userRecord

	err := s.db.WithContext(ctx).Where("email = ?", email).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.User{}, ErrUserNotFound
	case err != nil:
		return domain.User{}, fmt.Errorf("get user by email %q: %w", email, err)
	}

	return rec.toDomain(), nil
}

func (s *PostgresStore) CreateUser(ctx context.Context, user domain.User) error {

	rec := userRecord{
		ID:           user.ID,
		Username:     user.Username,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
	}
	if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("create user %q: %w", user.Username, err)
	}

	user.ID = rec.ID
	return nil
}

func (s *PostgresStore) UpdateUser(ctx context.Context, user domain.User) error {

	// A map (not a struct) so zero values are still written.
	result := s.db.WithContext(ctx).
		Model(&userRecord{}).
		Where("username = ?", user.Username).
		Updates(map[string]any{
			"email":         user.Email,
			"password_hash": user.PasswordHash,
		})
	if result.Error != nil {
		return fmt.Errorf("update user %q: %w", user.Username, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (s *PostgresStore) DeleteUser(ctx context.Context, username string) error {
	result := s.db.WithContext(ctx).Where("username = ?", username).Delete(&userRecord{})
	if result.Error != nil {
		return fmt.Errorf("delete user %q: %w", username, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}
