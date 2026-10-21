package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

type HashAndSalt struct {
	Hash string //base64(RawStd)
	Salt string //base64(RawStd)
}

func GenerateRandomSecret(length uint32) ([]byte, error) {
	secret := make([]byte, length)

	_, err := rand.Read(secret)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

type Argon2idHashConfig struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

func Argon2idHashConfigDefault() Argon2idHashConfig {
	return Argon2idHashConfig{
		Time:    3,
		Memory:  12288,
		Threads: 1,
		KeyLen:  64,
		SaltLen: 16,
	}
}

type Argon2idHash struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

func NewArgon2idHash(config Argon2idHashConfig) Argon2idHash {
	return Argon2idHash{
		time:    config.Time,
		saltLen: config.SaltLen,
		memory:  config.Memory,
		threads: config.Threads,
		keyLen:  config.KeyLen,
	}
}

func (a *Argon2idHash) GenerateSalt() ([]byte, error) {
	return GenerateRandomSecret(a.saltLen)
}

func (a *Argon2idHash) GeneratePasswordHash(password, salt []byte) (HashAndSalt, error) {
	var err error
	// If salt is not provided generate a salt of
	// the configured salt length.
	if len(salt) == 0 {
		salt, err = GenerateRandomSecret(a.saltLen)
	}
	if err != nil {
		return HashAndSalt{}, err
	}
	// Generate hash
	hash := argon2.IDKey(password, salt, a.time, a.memory, a.threads, a.keyLen)
	// Return the generated hash and salt used for storage.
	return HashAndSalt{
			Hash: base64.RawStdEncoding.EncodeToString(hash),
			Salt: base64.RawStdEncoding.EncodeToString(salt)},
		nil
}

func (a *Argon2idHash) Compare(encodedHash, encodedSalt, password string) error {
	hash, err := base64.RawStdEncoding.DecodeString(encodedHash)
	if err != nil {
		return fmt.Errorf("decode hash: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(encodedSalt)
	if err != nil {
		return fmt.Errorf("decode salt: %w", err)
	}
	if len(salt) != int(a.saltLen) || len(hash) != int(a.keyLen) {
		return errors.New("malformed stored hash or salt")
	}

	got := argon2.IDKey([]byte(password), salt, a.time, a.memory, a.threads, a.keyLen)
	if subtle.ConstantTimeCompare(got, hash) != 1 { // not bytes.Equal
		return errors.New("password does not match")
	}
	return nil
}
