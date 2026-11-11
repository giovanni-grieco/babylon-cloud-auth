package domain

import "fmt"

const (
	UNKNOWN          = -1
	PENDING_CREATION = 1
	CREATED          = 2
	FAILED_CREATION  = 3
	PENDING_DELETION = 4
	FAILED_DELETION  = 5
)

type User struct {
	ID         string
	RealmID    string
	Username   string
	Email      string
	RealmAlias string
	State      int
}

func (u User) ToString() string {
	return fmt.Sprintf("ID: %s, RealmID: %s, Username: %s, Email: %s, RealmAlias: %s, Steate:%d ", u.ID, u.RealmID, u.Username, u.Email, u.RealmAlias, u.State)
}
