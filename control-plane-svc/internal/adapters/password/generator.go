// Package password generates initial passwords.
package password

import "crypto/rand"

// Generator returns random passwords that the user must change at first login.
type Generator struct{}

// New returns 26 base32 characters (A-Z, 2-7), about 130 bits.
func (Generator) New() (string, error) {
	return rand.Text(), nil
}
