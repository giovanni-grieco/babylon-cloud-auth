package domain

type User struct {
	ID           uint64
	Username     string
	Email        string
	Role         string
	PasswordHash string
	Salt         string
}
