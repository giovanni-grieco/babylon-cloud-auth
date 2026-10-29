package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"auth-svc/internal/service"
	"common/domain"
)

var ErrUserNotFound = errors.New("user not found")

// Compile-time proof that PostgresUserStore implements the user port.
var _ service.UserStore = (*PostgresStore)(nil)
var _ service.SessionStore = (*PostgresStore)(nil)

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
	if err := s.db.WithContext(ctx).AutoMigrate(&userRecord{}, &sessionRecord{}); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetUser(ctx context.Context, id uint64) (domain.User, error) {
	var rec userRecord

	err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.User{}, ErrUserNotFound
	case err != nil:
		return domain.User{}, fmt.Errorf("get user %d: %w", id, err)
	}
	return rec.toDomain(), nil
}

func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
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
		Salt:         user.Salt,
		Role:         user.Role,
	}
	if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("create user %q: %w", user.Username, err)
	}
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
			"salt":          user.Salt,
			"role":          user.Role,
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

const forEachUsernameBatchSize = 1000

func (s *PostgresStore) ForEachUsername(ctx context.Context, fn func(username string) error) error {
	var batch []userRecord

	// Batched so large realms are never loaded into memory at once.
	result := s.db.WithContext(ctx).
		Select("id", "username").
		FindInBatches(&batch, forEachUsernameBatchSize, func(tx *gorm.DB, _ int) error {
			for _, rec := range batch {
				if err := fn(rec.Username); err != nil {
					return err
				}
			}
			return nil
		})
	if result.Error != nil {
		return fmt.Errorf("for each username: %w", result.Error)
	}

	return nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, session domain.Session) error {
	rec := sessionRecord{
		ID:           session.ID,
		UserID:       session.UserID,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    session.ExpiresAt,
	}
	if err := s.db.WithContext(ctx).Create(&rec).Error; err != nil {
		return fmt.Errorf("create session for user %d: %w", session.UserID, err)
	}

	return nil
}

func (s *PostgresStore) GetSession(ctx context.Context, id uint64) (domain.Session, error) {
	var rec sessionRecord

	err := s.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Session{}, ErrSessionNotFound
	case err != nil:
		return domain.Session{}, fmt.Errorf("get session %d: %w", id, err)
	}

	return rec.ToDomain(), nil
}

// GetSessionByUserID returns the user's most recent session, since a user can
// have several active at once.
func (s *PostgresStore) GetSessionByUserID(ctx context.Context, userID uint64) (domain.Session, error) {
	var rec sessionRecord

	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("id DESC").First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Session{}, ErrSessionNotFound
	case err != nil:
		return domain.Session{}, fmt.Errorf("get session by user id %d: %w", userID, err)
	}

	return rec.ToDomain(), nil
}

func (s *PostgresStore) GetSessionByRefreshToken(ctx context.Context, refreshToken string) (domain.Session, error) {
	var rec sessionRecord

	err := s.db.WithContext(ctx).Where("refresh_token = ?", refreshToken).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Session{}, ErrSessionNotFound
	case err != nil:
		// The token is a secret: keep it out of the error.
		return domain.Session{}, fmt.Errorf("get session by refresh token: %w", err)
	}

	return rec.ToDomain(), nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, id uint64) error {
	result := s.db.WithContext(ctx).Where("id = ?", id).Delete(&sessionRecord{})
	if result.Error != nil {
		return fmt.Errorf("delete session %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrSessionNotFound
	}

	return nil
}
