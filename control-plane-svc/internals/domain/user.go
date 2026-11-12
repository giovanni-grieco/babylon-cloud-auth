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

func StateToString(state int) string {
	switch state {
	case UNKNOWN:
		return "UNKNOWN"
	case PENDING_CREATION:
		return "PENDING_CREATION"
	case CREATED:
		return "CREATED"
	case FAILED_CREATION:
		return "FAILED_CREATION"
	case PENDING_DELETION:
		return "PENDING_DELETION"
	case FAILED_DELETION:
		return "FAILED_DELETION"
	default:
		return "INVALID_STATE"
	}
}

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
