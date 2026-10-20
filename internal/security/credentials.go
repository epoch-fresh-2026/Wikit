// Package security implements cryptographic dependencies outside Core.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Secrets struct{}

func (Secrets) Secret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("entropy unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func (Secrets) ID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("entropy unavailable")
	}
	raw[6] = (raw[6] & 15) | 64
	raw[8] = (raw[8] & 63) | 128
	h := hex.EncodeToString(raw[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

type Passwords struct {
	slots chan struct{}
	dummy string
}

func NewPasswords() (*Passwords, error) {
	p := &Passwords{slots: make(chan struct{}, 2)}
	var err error
	p.dummy, err = p.Hash("invalid-account-password")
	return p, err
}
func (p *Passwords) Hash(password string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", errors.New("entropy unavailable")
	}
	p.slots <- struct{}{}
	defer func() { <-p.slots }()
	key := argon2.IDKey([]byte(password), salt[:], 3, 64*1024, 4, 32)
	return "$argon2id$v=19$m=65536,t=3,p=4$" + base64.RawStdEncoding.EncodeToString(salt[:]) + "$" + base64.RawStdEncoding.EncodeToString(key), nil
}
func (p *Passwords) Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	valid := len(parts) == 6
	if valid {
		valid = parts[1] == "argon2id" && parts[2] == "v=19" && parts[3] == "m=65536,t=3,p=4"
	}
	if !valid {
		parts = strings.Split(p.dummy, "$")
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(parts[4])
	expected, keyErr := base64.RawStdEncoding.DecodeString(parts[5])
	if saltErr != nil || keyErr != nil || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	p.slots <- struct{}{}
	defer func() { <-p.slots }()
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1 && valid
}
