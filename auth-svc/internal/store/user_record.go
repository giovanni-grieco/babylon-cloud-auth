package store

import "common/domain"

type userRecord struct {
	ID           uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string `gorm:"column:username;index;not null"`
	Email        string `gorm:"column:email;uniqueIndex;not null"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	Salt         string `gorm:"column:salt;not null"`
}

func (userRecord) TableName() string { return "users" }

func (r userRecord) toDomain() domain.User {
	return domain.User{
		ID:           r.ID,
		Username:     r.Username,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Salt:         r.Salt,
	}
}
