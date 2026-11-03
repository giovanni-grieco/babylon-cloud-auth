package web

import (
	"slices"
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	data := "\xef\xbb\xbfEmail, username ,groups\nm@acme.test,mrossi,service-a;service-b\ng@acme.test,gverdi,\n"
	users, err := parseUsers("users.csv", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].Username != "mrossi" || users[0].Email != "m@acme.test" {
		t.Fatalf("users = %+v", users)
	}
	if !slices.Equal(users[0].Groups, []string{"service-a", "service-b"}) || users[1].Groups != nil {
		t.Fatalf("groups = %v, %v", users[0].Groups, users[1].Groups)
	}
}

func TestParseCSVErrors(t *testing.T) {
	for name, data := range map[string]string{
		"unknown column":   "username,email,role\n",
		"missing email":    "username\nmrossi\n",
		"duplicate column": "username,email,email\n",
		"ragged row":       "username,email\nmrossi\n",
	} {
		if _, err := parseUsers("u.csv", []byte(data)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestParseJSON(t *testing.T) {
	data := `{"users": [{"username": "mrossi", "email": "m@acme.test", "groups": ["service-a"]}]}`
	// No extension: detected from the content.
	users, err := parseUsers("upload", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "mrossi" || users[0].Groups[0] != "service-a" {
		t.Fatalf("users = %+v", users)
	}
}

func TestParseJSONRejectsUnknownFields(t *testing.T) {
	_, err := parseUsers("u.json", []byte(`{"users": [{"username": "m", "group": ["x"]}]}`))
	if err == nil || !strings.Contains(err.Error(), "group") {
		t.Fatalf("error = %v", err)
	}
}
