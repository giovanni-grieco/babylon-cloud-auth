package web

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"control-plane-svc/internal/domain"
)

// The accepted file formats. Both carry the same fields.
//
// JSON:
//
//	{"users": [{"username": "mrossi", "email": "mrossi@acme.test", "groups": ["service-a"]}]}
//
// CSV, with a header row in any column order. Groups are separated by ";":
//
//	username,email,groups
//	mrossi,mrossi@acme.test,service-a;service-b

type jsonFile struct {
	Users []jsonUser `json:"users"`
}

type jsonUser struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Groups   []string `json:"groups"`
}

// parseUsers reads an import file. It picks the format from the file name, or
// from the first character when the name has no known extension.
func parseUsers(name string, data []byte) ([]domain.NewUser, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return parseJSON(data)
	case ".csv":
		return parseCSV(data)
	}
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '{' {
		return parseJSON(data)
	}
	return parseCSV(data)
}

func parseJSON(data []byte) ([]domain.NewUser, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	// A misspelled field such as "group" would otherwise be silently dropped.
	dec.DisallowUnknownFields()
	var f jsonFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("reading JSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("reading JSON: unexpected data after the users object")
	}
	users := make([]domain.NewUser, len(f.Users))
	for i, u := range f.Users {
		users[i] = domain.NewUser{Username: u.Username, Email: u.Email, Groups: u.Groups}
	}
	return users, nil
}

var csvColumns = []string{"username", "email", "groups"}

func parseCSV(data []byte) ([]domain.NewUser, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // BOM written by Excel
	r := csv.NewReader(bytes.NewReader(data))
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the CSV header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		h = strings.ToLower(strings.TrimSpace(h))
		if !slices.Contains(csvColumns, h) {
			return nil, fmt.Errorf("unknown CSV column %q; the columns are %s", h, strings.Join(csvColumns, ", "))
		}
		if _, dup := col[h]; dup {
			return nil, fmt.Errorf("CSV column %q appears twice", h)
		}
		col[h] = i
	}
	for _, required := range []string{"username", "email"} {
		if _, ok := col[required]; !ok {
			return nil, fmt.Errorf("the CSV header has no %q column", required)
		}
	}

	var users []domain.NewUser
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return users, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV: %w", err)
		}
		u := domain.NewUser{Username: rec[col["username"]], Email: rec[col["email"]]}
		if i, ok := col["groups"]; ok && rec[i] != "" {
			u.Groups = strings.Split(rec[i], ";")
		}
		users = append(users, u)
	}
}
