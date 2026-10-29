package store

import (
	"common/domain"
	"time"
)

type sessionRecord struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       uint64     `gorm:"column:user_id;index;not null;"`
	UserRecord   userRecord `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	RefreshToken string     `gorm:"column:refresh_token;not null"`
	ExpiresAt    time.Time  `gorm:"column:expires_at;not null"`
}

func (sessionRecord) TableName() string { return "sessions" }

func (r sessionRecord) ToDomain() domain.Session {
	return domain.Session{
		ID:           r.ID,
		UserID:       r.UserID,
		RefreshToken: r.RefreshToken,
		ExpiresAt:    r.ExpiresAt,
	}
}
