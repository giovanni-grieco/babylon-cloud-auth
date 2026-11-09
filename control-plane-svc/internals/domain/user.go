package domain

import "fmt"

type User struct {
	ID         string
	Username   string
	Email      string
	RealmAlias string
}

func (u User) ToString() string {
	return fmt.Sprintf("ID: %s, Username: %s, Email: %s, RealmAlias: %s", u.ID, u.Username, u.Email, u.RealmAlias)
}
