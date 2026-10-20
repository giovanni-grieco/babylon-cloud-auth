package domain

import "time"

type Session struct {
	ID           uint64
	UserID       uint64
	RefreshToken string
	ExpiresAt    time.Time
}
