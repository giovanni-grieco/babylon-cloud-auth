package domain

import (
	"errors"
	"strings"
	"testing"
)

var (
	domains = []string{"acme.test"}
	groups  = []string{"service-a", "service-b"}
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		user NewUser
		want string // substring of the error; empty means valid
	}{
		{"valid", NewUser{Username: "mrossi", Email: "mrossi@acme.test", Groups: []string{"service-a"}}, ""},
		{"no groups", NewUser{Username: "mrossi", Email: "mrossi@acme.test"}, ""},
		{"short username", NewUser{Username: "ab", Email: "ab@acme.test"}, "3 to 255"},
		{"space in username", NewUser{Username: "m rossi", Email: "m@acme.test"}, "spaces"},
		{"at in username", NewUser{Username: "m@acme.test", Email: "m@acme.test"}, "can't contain @"},
		{"missing email", NewUser{Username: "mrossi"}, "plain address"},
		{"display name", NewUser{Username: "mrossi", Email: "Mario <m@acme.test>"}, "plain address"},
		{"foreign domain", NewUser{Username: "mrossi", Email: "m@gmail.com"}, `"gmail.com" isn't routed`},
		{"subdomain", NewUser{Username: "mrossi", Email: "m@eu.acme.test"}, "isn't routed"},
		{"unknown group", NewUser{Username: "mrossi", Email: "m@acme.test", Groups: []string{"admins"}}, `group "admins"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.user.Normalize().Validate(domains, groups)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestValidateNoDomains(t *testing.T) {
	err := NewUser{Username: "mrossi", Email: "m@acme.test"}.Validate(nil, groups)
	if err == nil || !strings.Contains(err.Error(), "routed: none") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalize(t *testing.T) {
	u := NewUser{Username: " MRossi ", Email: "MRossi@ACME.test", Groups: []string{" service-a", "", "service-a"}}.Normalize()
	if u.Username != "mrossi" || u.Email != "mrossi@acme.test" || len(u.Groups) != 1 || u.Groups[0] != "service-a" {
		t.Fatalf("got %+v", u)
	}
}

func TestValidateBatch(t *testing.T) {
	users := []NewUser{
		{Username: "mrossi", Email: "mrossi@acme.test"},
		{Username: "MROSSI", Email: "other@acme.test"},  // same username after lowercasing
		{Username: "gverdi", Email: "MROSSI@acme.test"}, // same email
		{Username: "ok-user", Email: "ok@acme.test"},
	}
	_, err := ValidateBatch(users, domains, groups)
	var batch *BatchError
	if !errors.As(err, &batch) {
		t.Fatalf("error = %v, want *BatchError", err)
	}
	if len(batch.Rows) != 2 || batch.Rows[0].Row != 2 || batch.Rows[1].Row != 3 {
		t.Fatalf("rows = %+v", batch.Rows)
	}
	if !strings.Contains(batch.Rows[0].Err.Error(), "repeats row 1") {
		t.Fatalf("row 2 error = %v", batch.Rows[0].Err)
	}
}

func TestValidateBatchLimits(t *testing.T) {
	if _, err := ValidateBatch(nil, domains, groups); err == nil {
		t.Fatal("empty batch accepted")
	}
	if _, err := ValidateBatch(make([]NewUser, MaxBatch+1), domains, groups); err == nil {
		t.Fatal("oversized batch accepted")
	}
}
