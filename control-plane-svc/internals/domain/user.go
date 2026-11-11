package domain

import "fmt"

type User struct {
	ID         string
	RealmID    string
	Username   string
	Email      string
	RealmAlias string
}

func (u User) ToString() string {
	return fmt.Sprintf("ID: %s, RealmID: %s, Username: %s, Email: %s, RealmAlias: %s", u.ID, u.RealmID, u.Username, u.Email, u.RealmAlias)
}
