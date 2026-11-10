package store

import (
	"context"
	"control-plane-svc/internals/domain"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type UserRealm struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	Username   string `gorm:"uniqueIndex"`
	Email      string `gorm:"uniqueIndex"`
	RealmAlias string
}

type PostgresStoreConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DbName   string
	SSLMode  string //enabled-disabled
	Timezone string //Europe/Rome
}

type PostgresStore struct {
	db *gorm.DB
}

func NewPostgresStore(host, user, password, dbname, port, sslmode, timezone string) (*PostgresStore, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s", host, user, password, dbname, port, sslmode, timezone)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("Error while creating PostgresStore: %v", err)
	}

	db.AutoMigrate(&UserRealm{})
	return &PostgresStore{db: db}, nil
}

func NewPostgresStoreWithCfg(cfg PostgresStoreConfig) (*PostgresStore, error) {
	return NewPostgresStore(cfg.Host, cfg.User, cfg.Password, cfg.DbName, cfg.Port, cfg.SSLMode, cfg.Timezone)
}

func (ps *PostgresStore) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	user, err := gorm.G[UserRealm](ps.db).Where("username = ?", username).First(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.User{ID: user.ID, Username: user.Username, Email: user.Email, RealmAlias: user.RealmAlias}, nil
}

func (ps *PostgresStore) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := gorm.G[UserRealm](ps.db).Where("email = ?", email).First(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.User{ID: user.ID, Username: user.Username, Email: user.Email, RealmAlias: user.RealmAlias}, nil

}

func (ps *PostgresStore) FindByRealmAlias(ctx context.Context, realmAlias string) ([]*domain.User, error) {
	users, err := gorm.G[UserRealm](ps.db).Where("realm_alias = ?", realmAlias).Find(ctx)
	if err != nil {
		return nil, err
	}

	var domainUsers []*domain.User
	for _, user := range users {
		domainUsers = append(domainUsers, &domain.User{ID: user.ID, Username: user.Username, Email: user.Email, RealmAlias: user.RealmAlias})
	}
	return domainUsers, nil
}

func (ps *PostgresStore) Save(ctx context.Context, user *domain.User) error {
	userRealm := UserRealm{ID: user.ID, Username: user.Username, Email: user.Email, RealmAlias: user.RealmAlias}
	err := gorm.G[UserRealm](ps.db).Create(ctx, &userRealm)
	if err != nil {
		return err
	}
	return nil
}

func (ps *PostgresStore) Delete(ctx context.Context, user *domain.User) error {
	rowsAffected, err := gorm.G[UserRealm](ps.db).Where("username = ?", user.Username).Delete(ctx)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("User not found: %s", user.Username)
	}
	return nil
}
