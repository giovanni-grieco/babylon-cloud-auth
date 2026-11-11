package store

import (
	"context"
	"control-plane-svc/internals/domain"
	"control-plane-svc/internals/service"
	"fmt"
	"log/slog"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type UserRealm struct {
	ID         string `gorm:"primaryKey;type:uuid;not null;default:gen_random_uuid()"`
	RealmID    string `gorm:"unique;not null;type:varchar(100);default:null;uniqueIndex"`
	Username   string `gorm:"unique;not null;type:varchar(100);default:null;uniqueIndex"`
	Email      string `gorm:"unique;not null;type:varchar(100);default:null;uniqueIndex"`
	RealmAlias string `gorm:"not null;type:varchar(100);default:null"`
}

func FromUserRealm(userRealm *UserRealm) *domain.User {
	return &domain.User{
		ID:         userRealm.ID,
		RealmID:    userRealm.RealmID,
		Username:   userRealm.Username,
		Email:      userRealm.Email,
		RealmAlias: userRealm.RealmAlias,
	}
}

func ToUserRealm(user *domain.User) *UserRealm {
	return &UserRealm{
		ID:         user.ID,
		RealmID:    user.RealmID,
		Username:   user.Username,
		Email:      user.Email,
		RealmAlias: user.RealmAlias,
	}
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

func (ps *PostgresStore) InTx(ctx context.Context, fn func(ctx context.Context, store service.UserStore) error) error {
	if fn == nil {
		return fmt.Errorf("transaction callback cannot be nil")
	}

	return ps.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		slog.InfoContext(ctx, "Starting TX")
		transactionStore := &PostgresStore{db: tx}
		err := fn(ctx, transactionStore)
		slog.InfoContext(ctx, "Ending TX")
		return err
	})
}

func (ps *PostgresStore) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	user, err := gorm.G[UserRealm](ps.db).Where("username = ?", username).First(ctx)
	if err != nil {
		return nil, err
	}
	return FromUserRealm(&user), nil
}

func (ps *PostgresStore) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := gorm.G[UserRealm](ps.db).Where("email = ?", email).First(ctx)
	if err != nil {
		return nil, err
	}
	return FromUserRealm(&user), nil

}

func (ps *PostgresStore) FindByRealmAlias(ctx context.Context, realmAlias string) ([]*domain.User, error) {
	users, err := gorm.G[UserRealm](ps.db).Where("realm_alias = ?", realmAlias).Find(ctx)
	if err != nil {
		return nil, err
	}

	var domainUsers []*domain.User
	for _, user := range users {
		domainUsers = append(domainUsers, FromUserRealm(&user))
	}
	return domainUsers, nil
}

func (ps *PostgresStore) Save(ctx context.Context, user *domain.User) error {
	userRealm := ToUserRealm(user)
	err := gorm.G[UserRealm](ps.db).Create(ctx, userRealm)
	if err != nil {
		return err
	}
	return nil
}

func (ps *PostgresStore) Delete(ctx context.Context, user *domain.User) error {
	rowsAffected, err := gorm.G[UserRealm](ps.db).Where("id = ?", user.ID).Delete(ctx)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("User not found: %s", user.Username)
	}
	return nil ///COMMMENTS
}
