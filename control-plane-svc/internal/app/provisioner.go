package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"control-plane-svc/internal/domain"
)

// Provisioner allocates and removes B2B accounts in DC realms.
type Provisioner struct {
	realms    []domain.Realm
	store     IdentityStore
	broker    Broker
	passwords Passwords
}

func NewProvisioner(realms []domain.Realm, store IdentityStore, broker Broker, passwords Passwords) *Provisioner {
	return &Provisioner{realms: realms, store: store, broker: broker, passwords: passwords}
}

// Realms lists the realms an operator can provision into.
func (p *Provisioner) Realms() []domain.Realm {
	return p.realms
}

// Realm returns domain.ErrUnknownRealm for an ID that isn't configured.
func (p *Provisioner) Realm(id domain.RealmID) (domain.Realm, error) {
	for _, r := range p.realms {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.Realm{}, fmt.Errorf("%w: %q", domain.ErrUnknownRealm, id)
}

// RealmInfo is what an operator needs to prepare an import file.
type RealmInfo struct {
	Realm   domain.Realm
	Domains []string
	Groups  []string
}

func (p *Provisioner) Describe(ctx context.Context, id domain.RealmID) (RealmInfo, error) {
	realm, err := p.Realm(id)
	if err != nil {
		return RealmInfo{}, err
	}
	domains, err := p.broker.Domains(ctx, id)
	if err != nil {
		return RealmInfo{}, fmt.Errorf("reading the email domains of %s from the central Keycloak: %w", id, err)
	}
	groups, err := p.store.Groups(ctx, id)
	if err != nil {
		return RealmInfo{}, fmt.Errorf("reading the groups of %s: %w", id, err)
	}
	return RealmInfo{Realm: realm, Domains: domains, Groups: groups}, nil
}

// Created is the outcome of creating one user.
type Created struct {
	User domain.NewUser
	// Password is set when the user was created. It's shown only once.
	Password string
	Err      error
}

// ImportReport lists the outcome for every row, in file order.
type ImportReport struct {
	Realm   domain.Realm
	Results []Created
}

func (r ImportReport) Failed() int {
	n := 0
	for _, c := range r.Results {
		if c.Err != nil {
			n++
		}
	}
	return n
}

// Import validates the whole batch first and creates nothing if any row is
// invalid; that error is a *domain.BatchError. Once validation passes, it
// creates the users one by one. A user that fails, for example because the
// username is taken, doesn't stop the others.
func (p *Provisioner) Import(ctx context.Context, id domain.RealmID, users []domain.NewUser) (ImportReport, error) {
	info, err := p.Describe(ctx, id)
	if err != nil {
		return ImportReport{}, err
	}
	users, err = domain.ValidateBatch(users, info.Domains, info.Groups)
	if err != nil {
		return ImportReport{}, err
	}

	report := ImportReport{Realm: info.Realm, Results: make([]Created, 0, len(users))}
	for _, u := range users {
		if err := ctx.Err(); err != nil {
			// The operator gave up on the request. Report what's left as not attempted.
			report.Results = append(report.Results, Created{User: u, Err: fmt.Errorf("not attempted: %w", err)})
			continue
		}
		report.Results = append(report.Results, p.create(ctx, id, u))
	}
	return report, nil
}

func (p *Provisioner) create(ctx context.Context, id domain.RealmID, u domain.NewUser) Created {
	password, err := p.passwords.New()
	if err != nil {
		return Created{User: u, Err: fmt.Errorf("generating a password: %w", err)}
	}
	if _, err := p.store.CreateUser(ctx, id, u, password); err != nil {
		return Created{User: u, Err: err}
	}
	return Created{User: u, Password: password}
}

// Users lists users in a realm, optionally filtered by a search string that
// matches username, email or name.
func (p *Provisioner) Users(ctx context.Context, id domain.RealmID, search string, first, max int) ([]domain.User, error) {
	if _, err := p.Realm(id); err != nil {
		return nil, err
	}
	return p.store.ListUsers(ctx, id, strings.TrimSpace(search), first, max)
}

// Deleted is the outcome of deleting one user.
type Deleted struct {
	ID       string
	Username string
	Err      error
}

// Delete removes users from a DC realm. It removes the central copy first: if
// that fails, the DC account stays, so the operator can retry from the same
// list. In the other order, a failure would leave a central session alive for
// an account that no longer appears anywhere.
func (p *Provisioner) Delete(ctx context.Context, id domain.RealmID, userIDs []string) ([]Deleted, error) {
	if _, err := p.Realm(id); err != nil {
		return nil, err
	}
	results := make([]Deleted, 0, len(userIDs))
	for _, uid := range userIDs {
		results = append(results, p.delete(ctx, id, uid))
	}
	return results, nil
}

func (p *Provisioner) delete(ctx context.Context, id domain.RealmID, uid string) Deleted {
	user, err := p.store.User(ctx, id, uid)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return Deleted{ID: uid, Err: errors.New("already deleted")}
		}
		return Deleted{ID: uid, Err: err}
	}
	d := Deleted{ID: uid, Username: user.Username}
	if err := p.broker.RemoveBrokeredUser(ctx, id, uid); err != nil {
		d.Err = fmt.Errorf("removing the central copy, so the DC account was kept: %w", err)
		return d
	}
	if err := p.store.DeleteUser(ctx, id, uid); err != nil && !errors.Is(err, domain.ErrNotFound) {
		d.Err = err
	}
	return d
}
