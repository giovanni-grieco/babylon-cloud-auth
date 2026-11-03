// Package config reads the realm catalog. The file holds connection details;
// each secret is read from the environment variable the file names, so the
// file can be committed.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Keycloak struct {
	BaseURL   string `json:"baseURL"`
	Realm     string `json:"realm"`
	ClientID  string `json:"clientID"`
	SecretEnv string `json:"secretEnv"`
	// Secret is filled from SecretEnv by Load.
	Secret string `json:"-"`
}

type Realm struct {
	// ID must equal the alias of this realm's identity provider in the central Keycloak.
	ID   string `json:"id"`
	Name string `json:"name"`
	Keycloak
}

type Config struct {
	Central Keycloak `json:"central"`
	Realms  []Realm  `json:"realms"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}

	var errs []error
	if err := c.Central.resolve("central"); err != nil {
		errs = append(errs, err)
	}
	seen := map[string]bool{}
	for i := range c.Realms {
		r := &c.Realms[i]
		if r.ID == "" {
			errs = append(errs, fmt.Errorf("realm %d has no id", i+1))
			continue
		}
		if seen[r.ID] {
			errs = append(errs, fmt.Errorf("realm %q appears twice", r.ID))
		}
		seen[r.ID] = true
		if r.Name == "" {
			r.Name = r.ID
		}
		if err := r.resolve("realm " + r.ID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(c.Realms) == 0 {
		errs = append(errs, errors.New("no realms configured"))
	}
	if err := errors.Join(errs...); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (k *Keycloak) resolve(where string) error {
	if k.BaseURL == "" || k.Realm == "" || k.ClientID == "" || k.SecretEnv == "" {
		return fmt.Errorf("%s needs baseURL, realm, clientID and secretEnv", where)
	}
	k.Secret = os.Getenv(k.SecretEnv)
	if k.Secret == "" {
		return fmt.Errorf("%s: environment variable %s is empty", where, k.SecretEnv)
	}
	return nil
}
