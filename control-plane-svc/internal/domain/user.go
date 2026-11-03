// Package domain holds the provisioning rules. It has no knowledge of Keycloak,
// HTTP or file formats.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"
)

// MaxBatch caps how many users one import may create.
const MaxBatch = 5000

// RealmID names a DC realm. It's the same as the alias of that DC's identity
// provider in the central Keycloak, for example "dc-1".
type RealmID string

// Realm is a place where users can be provisioned.
type Realm struct {
	ID   RealmID
	Name string
}

// NewUser is one row of an import file.
type NewUser struct {
	Username string
	Email    string
	Groups   []string
}

// User is a user that already exists in a realm.
type User struct {
	ID        string
	Username  string
	Email     string
	Enabled   bool
	CreatedAt time.Time
}

// Normalize lowercases the username and email, as Keycloak does, and removes
// duplicate groups, so validation compares what Keycloak will actually store.
func (u NewUser) Normalize() NewUser {
	u.Username = strings.ToLower(strings.TrimSpace(u.Username))
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	groups := make([]string, 0, len(u.Groups))
	for _, g := range u.Groups {
		g = strings.TrimSpace(g)
		if g != "" && !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	u.Groups = groups
	return u
}

// Validate checks one normalized user against the realm's email domains and groups.
//
// The email is required because the central Keycloak finds a B2B user's DC
// only from the domain of the email they type. A user whose domain isn't
// routed to this realm could never sign in.
func (u NewUser) Validate(domains, groups []string) error {
	var errs []error

	switch {
	case len(u.Username) < 3 || len(u.Username) > 255:
		errs = append(errs, errors.New("username must be 3 to 255 characters"))
	case strings.ContainsFunc(u.Username, unicode.IsSpace):
		errs = append(errs, errors.New("username can't contain spaces"))
	case strings.Contains(u.Username, "@"):
		// The central Keycloak looks users up by email when the input contains @.
		errs = append(errs, errors.New("username can't contain @; put the address in email"))
	}

	if addr, err := mail.ParseAddress(u.Email); err != nil || addr.Address != u.Email {
		errs = append(errs, fmt.Errorf("email %q isn't a plain address", u.Email))
	} else if domain := u.Email[strings.LastIndexByte(u.Email, '@')+1:]; !slices.Contains(domains, domain) {
		errs = append(errs, fmt.Errorf("email domain %q isn't routed to this realm (routed: %s)", domain, listOrNone(domains)))
	}

	for _, g := range u.Groups {
		if !slices.Contains(groups, g) {
			errs = append(errs, fmt.Errorf("group %q doesn't exist in this realm", g))
		}
	}

	return errors.Join(errs...)
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

// RowError is a validation error for one row of an import, numbered from 1.
type RowError struct {
	Row      int
	Username string
	Err      error
}

// BatchError means an import was rejected before any user was created.
type BatchError struct {
	Rows []RowError
	// Err is set when the batch as a whole is wrong, such as an empty file.
	Err error
}

func (e *BatchError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%d invalid rows", len(e.Rows))
}

// ValidateBatch normalizes and checks every row. It returns the normalized users,
// or a *BatchError listing every problem, so an operator can fix the whole file at once.
func ValidateBatch(users []NewUser, domains, groups []string) ([]NewUser, error) {
	if len(users) == 0 {
		return nil, &BatchError{Err: errors.New("the file contains no users")}
	}
	if len(users) > MaxBatch {
		return nil, &BatchError{Err: fmt.Errorf("the file contains %d users; the limit is %d", len(users), MaxBatch)}
	}

	out := make([]NewUser, len(users))
	var rows []RowError
	seenUsername := map[string]int{}
	seenEmail := map[string]int{}
	for i, u := range users {
		u = u.Normalize()
		out[i] = u
		row := i + 1

		err := u.Validate(domains, groups)
		if prev, ok := seenUsername[u.Username]; ok && u.Username != "" {
			err = errors.Join(err, fmt.Errorf("username repeats row %d", prev))
		} else {
			seenUsername[u.Username] = row
		}
		if prev, ok := seenEmail[u.Email]; ok && u.Email != "" {
			err = errors.Join(err, fmt.Errorf("email repeats row %d", prev))
		} else {
			seenEmail[u.Email] = row
		}
		if err != nil {
			rows = append(rows, RowError{Row: row, Username: u.Username, Err: err})
		}
	}
	if len(rows) > 0 {
		return nil, &BatchError{Rows: rows}
	}
	return out, nil
}
