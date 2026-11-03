package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"control-plane-svc/internal/domain"
)

type fakeStore struct {
	groups  []string
	users   map[string]domain.User // by ID
	failOn  map[string]error       // CreateUser error by username
	deleted []string
	nextID  int
}

func (f *fakeStore) Groups(context.Context, domain.RealmID) ([]string, error) { return f.groups, nil }

func (f *fakeStore) CreateUser(_ context.Context, _ domain.RealmID, u domain.NewUser, _ string) (string, error) {
	if err := f.failOn[u.Username]; err != nil {
		return "", err
	}
	f.nextID++
	id := fmt.Sprint("id-", f.nextID)
	f.users[id] = domain.User{ID: id, Username: u.Username, Email: u.Email}
	return id, nil
}

func (f *fakeStore) ListUsers(context.Context, domain.RealmID, string, int, int) ([]domain.User, error) {
	return nil, nil
}

func (f *fakeStore) User(_ context.Context, _ domain.RealmID, id string) (domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) DeleteUser(_ context.Context, _ domain.RealmID, id string) error {
	delete(f.users, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeBroker struct {
	domains []string
	removed []string
	fail    error
}

func (b *fakeBroker) Domains(context.Context, domain.RealmID) ([]string, error) {
	return b.domains, nil
}

func (b *fakeBroker) RemoveBrokeredUser(_ context.Context, realm domain.RealmID, id string) error {
	if b.fail != nil {
		return b.fail
	}
	b.removed = append(b.removed, string(realm)+"."+id)
	return nil
}

type fixedPasswords struct{}

func (fixedPasswords) New() (string, error) { return "TEMP", nil }

func setup() (*Provisioner, *fakeStore, *fakeBroker) {
	store := &fakeStore{groups: []string{"service-a"}, users: map[string]domain.User{}, failOn: map[string]error{}}
	broker := &fakeBroker{domains: []string{"acme.test"}}
	realms := []domain.Realm{{ID: "dc-1", Name: "DC 1"}}
	return NewProvisioner(realms, store, broker, fixedPasswords{}), store, broker
}

func TestImportCreatesUsersAndKeepsGoingOnConflict(t *testing.T) {
	p, store, _ := setup()
	store.failOn["taken"] = domain.ErrConflict

	report, err := p.Import(context.Background(), "dc-1", []domain.NewUser{
		{Username: "mrossi", Email: "mrossi@acme.test", Groups: []string{"service-a"}},
		{Username: "taken", Email: "taken@acme.test"},
		{Username: "gverdi", Email: "gverdi@acme.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed() != 1 || !errors.Is(report.Results[1].Err, domain.ErrConflict) {
		t.Fatalf("results = %+v", report.Results)
	}
	if report.Results[0].Password != "TEMP" || report.Results[2].Password != "TEMP" {
		t.Fatalf("created users have no password: %+v", report.Results)
	}
	if len(store.users) != 2 {
		t.Fatalf("store has %d users, want 2", len(store.users))
	}
}

func TestImportRejectsWholeBatch(t *testing.T) {
	p, store, _ := setup()
	_, err := p.Import(context.Background(), "dc-1", []domain.NewUser{
		{Username: "mrossi", Email: "mrossi@acme.test"},
		{Username: "bad", Email: "bad@gmail.com"},
	})
	var batch *domain.BatchError
	if !errors.As(err, &batch) || len(batch.Rows) != 1 || batch.Rows[0].Row != 2 {
		t.Fatalf("error = %v", err)
	}
	if len(store.users) != 0 {
		t.Fatal("a user was created from an invalid batch")
	}
}

func TestUnknownRealm(t *testing.T) {
	p, _, _ := setup()
	if _, err := p.Import(context.Background(), "dc-9", nil); !errors.Is(err, domain.ErrUnknownRealm) {
		t.Fatalf("Import: %v", err)
	}
	if _, err := p.Delete(context.Background(), "dc-9", []string{"x"}); !errors.Is(err, domain.ErrUnknownRealm) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDeleteRemovesCentralCopyFirst(t *testing.T) {
	p, store, broker := setup()
	store.users["u1"] = domain.User{ID: "u1", Username: "mrossi"}

	results, err := p.Delete(context.Background(), "dc-1", []string{"u1", "gone"})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Err != nil || results[0].Username != "mrossi" {
		t.Fatalf("u1 result = %+v", results[0])
	}
	if results[1].Err == nil {
		t.Fatal("deleting a missing user reported success")
	}
	if !slices.Equal(broker.removed, []string{"dc-1.u1"}) || !slices.Equal(store.deleted, []string{"u1"}) {
		t.Fatalf("removed = %v, deleted = %v", broker.removed, store.deleted)
	}
}

func TestDeleteKeepsDCAccountWhenCentralFails(t *testing.T) {
	p, store, broker := setup()
	store.users["u1"] = domain.User{ID: "u1", Username: "mrossi"}
	broker.fail = errors.New("central down")

	results, _ := p.Delete(context.Background(), "dc-1", []string{"u1"})
	if results[0].Err == nil {
		t.Fatal("expected an error")
	}
	if len(store.deleted) != 0 {
		t.Fatal("DC account deleted although the central copy remains")
	}
}
