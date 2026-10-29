package utils

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrMissingKeyID      = errors.New("jwt has no key id")
	ErrUnknownKeyID      = errors.New("unknown jwt key id")
	ErrIssuerKeyMismatch = errors.New("jwt issuer does not match its signing key")
)

type AccessClaims struct {
	Role  string `json:"role"`
	Realm string `json:"realm"`
	jwt.RegisteredClaims
}

func SignJWT(claims jwt.Claims, keyID string, privateKey ed25519.PrivateKey) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = keyID

	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

func ParseJWT(tokenString string, publicKeys map[string]ed25519.PublicKey, claims jwt.Claims) error {
	keyFunc := func(token *jwt.Token) (any, error) {
		keyID, ok := token.Header["kid"].(string)
		if !ok || keyID == "" {
			return nil, ErrMissingKeyID
		}
		key, found := publicKeys[keyID]
		if !found {
			return nil, fmt.Errorf("%w %q", ErrUnknownKeyID, keyID)
		}
		// Claims are decoded but not yet trusted here; the signature is still
		// checked with this key right after, so a forged iss can't slip through.
		if iss, err := token.Claims.GetIssuer(); err != nil || iss != keyID {
			return nil, fmt.Errorf("%w: iss %q, kid %q", ErrIssuerKeyMismatch, iss, keyID)
		}
		return key, nil
	}

	_, err := jwt.ParseWithClaims(tokenString, claims, keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return fmt.Errorf("parse jwt: %w", err)
	}
	return nil
}
