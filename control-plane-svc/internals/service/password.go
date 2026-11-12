package service

import (
	"crypto/rand"
	"math/big"
)

const (
	temporaryPasswordLength = 20
	passwordLower           = "abcdefghijkmnopqrstuvwxyz"
	passwordUpper           = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	passwordDigits          = "23456789"
	passwordSymbols         = "!@#$%*-_+=?"
)

// GenerateTemporaryPassword returns a random password with at least one
// lowercase letter, uppercase letter, digit and symbol, so it passes the
// usual realm password policies. Look-alike characters (l, I, O, 0, 1) are
// left out because an operator may have to read it to the user.
func GenerateTemporaryPassword() (string, error) {
	classes := []string{passwordLower, passwordUpper, passwordDigits, passwordSymbols}
	all := passwordLower + passwordUpper + passwordDigits + passwordSymbols

	password := make([]byte, 0, temporaryPasswordLength)
	for _, class := range classes {
		c, err := randomChar(class)
		if err != nil {
			return "", err
		}
		password = append(password, c)
	}
	for len(password) < temporaryPasswordLength {
		c, err := randomChar(all)
		if err != nil {
			return "", err
		}
		password = append(password, c)
	}

	// Fisher-Yates shuffle, so the guaranteed characters are not always first.
	for i := len(password) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		password[i], password[j.Int64()] = password[j.Int64()], password[i]
	}
	return string(password), nil
}

func randomChar(charset string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	if err != nil {
		return 0, err
	}
	return charset[n.Int64()], nil
}
