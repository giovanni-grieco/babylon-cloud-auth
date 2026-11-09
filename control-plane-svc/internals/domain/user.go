package domain

import "fmt"

type User struct {
	Username   string
	Email      string
	RealmAlias string
}

func (u User) ToString() string {
	return fmt.Sprintf("Username: %s, Email: %s, RealmAlias: %s", u.Username, u.Email, u.RealmAlias)
}
