package store

import (
	"context"
	"control-plane-svc/internals/domain"
	"control-plane-svc/internals/service"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRealm struct {
	ID         string `gorm:"primaryKey;type:uuid;not null;default:gen_random_uuid()"`
	RealmID    string `gorm:"type:varchar(100);default:null;index"`
	Username   string `gorm:"unique;not null;type:varchar(100);default:null;uniqueIndex"`
	Email      string `gorm:"unique;not null;type:varchar(100);default:null;uniqueIndex"`
	RealmAlias string `gorm:"not null;type:varchar(100);default:null"`
	State      int    `gorm:"default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time `gorm:"index"` // also used as a lease by ClaimStale
}

func FromUserRealm(userRealm *UserRealm) *domain.User {
	return &domain.User{
		ID:         userRealm.ID,
		RealmID:    userRealm.RealmID,
		Username:   userRealm.Username,
		Email:      userRealm.Email,
		RealmAlias: userRealm.RealmAlias,
		State:      userRealm.State,
	}
}

func ToUserRealm(user *domain.User) *UserRealm {
	return &UserRealm{
		ID:         user.ID,
		RealmID:    user.RealmID,
		Username:   user.Username,
		Email:      user.Email,
		RealmAlias: user.RealmAlias,
		State:      user.State,
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
	// TranslateError turns unique violations into gorm.ErrDuplicatedKey.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, fmt.Errorf("Error while creating PostgresStore: %v", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("Error while migrating the schema: %v", err)
	}
	return &PostgresStore{db: db}, nil
}

// migrateLockKey is an arbitrary key for pg_advisory_lock, shared by every
// control plane instance.
const migrateLockKey = 7_420_531

// migrate runs AutoMigrate under a Postgres advisory lock, so replicas
// starting together don't race on CREATE TABLE / CREATE INDEX. The lock is
// session-scoped, so lock, migration and unlock share one connection.
func migrate(db *gorm.DB) error {
	return db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SELECT pg_advisory_lock(?)", migrateLockKey).Error; err != nil {
			return err
		}
		defer conn.Exec("SELECT pg_advisory_unlock(?)", migrateLockKey)
		return conn.AutoMigrate(&UserRealm{})
	})
}

// Ping checks that the database answers and accepts writes. Behind the
// Patroni load balancer a connection always lands on the primary, but right
// after a failover an old connection may still point at a demoted node, which
// is read-only: report that as unhealthy too.
func (ps *PostgresStore) Ping(ctx context.Context) error {
	var readOnly bool
	if err := ps.db.WithContext(ctx).Raw("SELECT pg_is_in_recovery()").Scan(&readOnly).Error; err != nil {
		return err
	}
	if readOnly {
		return errors.New("connected to a read-only replica")
	}
	return nil
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

func (ps *PostgresStore) Find(ctx context.Context, id string) (*domain.User, error) {
	userRealm, err := gorm.G[UserRealm](ps.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s", service.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return FromUserRealm(&userRealm), nil
}

// List returns one page of users ordered by realm, then username.
func (ps *PostgresStore) List(ctx context.Context, offset, limit int) ([]*domain.User, error) {
	var rows []UserRealm
	err := ps.db.WithContext(ctx).Order("realm_alias, username").Offset(offset).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	users := make([]*domain.User, 0, len(rows))
	for i := range rows {
		users = append(users, FromUserRealm(&rows[i]))
	}
	return users, nil
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

func (ps *PostgresStore) Save(ctx context.Context, user *domain.User) (*domain.User, error) {
	userRealm := ToUserRealm(user)
	err := gorm.G[UserRealm](ps.db).Create(ctx, userRealm)
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, fmt.Errorf("%w: %v", service.ErrIdentifierTaken, err)
	}
	if err != nil {
		return nil, err
	}
	return FromUserRealm(userRealm), nil
}

// Update writes the user's columns explicitly (so created_at is never
// overwritten) and refreshes updated_at.
func (ps *PostgresStore) Update(ctx context.Context, user *domain.User) (*domain.User, error) {
	result := ps.db.WithContext(ctx).Model(&UserRealm{}).Where("id = ?", user.ID).Updates(map[string]any{
		"realm_id":    user.RealmID,
		"username":    user.Username,
		"email":       user.Email,
		"realm_alias": user.RealmAlias,
		"state":       user.State,
		"updated_at":  time.Now(),
	})
	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return nil, fmt.Errorf("%w: %v", service.ErrIdentifierTaken, result.Error)
	}
	if result.Error != nil {
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("No rows affected")
	}

	updated := *user
	return &updated, nil
}

// FindForUpdate reads the user and locks its row until the surrounding
// transaction ends. Use it inside InTx.
func (ps *PostgresStore) FindForUpdate(ctx context.Context, id string) (*domain.User, error) {
	var userRealm UserRealm
	err := ps.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&userRealm).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: %s", service.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return FromUserRealm(&userRealm), nil
}

// FindStale returns up to limit users in the given state whose row has not
// been touched for olderThan. It takes no lock: lock each candidate with
// FindForUpdateSkipLocked inside its own transaction.
func (ps *PostgresStore) FindStale(ctx context.Context, state int, olderThan time.Duration, limit int) ([]*domain.User, error) {
	var rows []UserRealm
	cutoff := time.Now().Add(-olderThan)
	err := ps.db.WithContext(ctx).
		Where("state = ? AND COALESCE(updated_at, 'epoch') < ?", state, cutoff).
		Order("updated_at").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	users := make([]*domain.User, 0, len(rows))
	for i := range rows {
		users = append(users, FromUserRealm(&rows[i]))
	}
	return users, nil
}

// FindForUpdateSkipLocked locks the user's row until the surrounding
// transaction ends. It returns nil, nil when the row is gone or already locked
// by someone else (another replica, a request in progress). Use it inside InTx.
func (ps *PostgresStore) FindForUpdateSkipLocked(ctx context.Context, id string) (*domain.User, error) {
	var rows []UserRealm
	err := ps.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("id = ?", id).
		Limit(1).
		Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return FromUserRealm(&rows[0]), nil
}

func (ps *PostgresStore) Delete(ctx context.Context, id string) error {
	rowsAffected, err := gorm.G[UserRealm](ps.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("User with id: %s not found", id)
	}
	return nil
}
