package domain

type User struct {
	ID           uint64
	Username     string
	Email        string
	PasswordHash string
	Role         string
}
