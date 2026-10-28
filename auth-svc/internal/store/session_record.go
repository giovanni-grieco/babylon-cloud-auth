package store

import "time"

type sessionRecord struct {
	ID           uint64 `gorm`
	UserID       uint64
	RefreshToken string
	ExpiresAt    time.Time
}
