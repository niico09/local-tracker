package domain

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"time"
)

// UserID identifies a profile. Values are positive; SystemActorID is reserved.
type UserID int64

const (
	// PinIterations is the PBKDF2 work factor mandated by the design.
	PinIterations = 600_000
	pinSaltBytes  = 16
	pinKeyBytes   = 32
)

// User is one of the two profiles created at first-run setup.
type User struct {
	ID        UserID
	Name      string
	PinHash   []byte
	PinSalt   []byte
	PinIter   int
	CreatedAt time.Time
}

// HashPIN derives a PBKDF2-HMAC-SHA256 key with a fresh random salt.
func HashPIN(pin string) (hash, salt []byte, iter int, err error) {
	salt = make([]byte, pinSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, 0, err
	}
	key, err := pbkdf2.Key(sha256.New, pin, salt, PinIterations, pinKeyBytes)
	if err != nil {
		return nil, nil, 0, err
	}
	return key, salt, PinIterations, nil
}

// VerifyPIN compares a candidate PIN against stored values in constant time.
func VerifyPIN(pin string, hash, salt []byte, iter int) bool {
	if len(hash) == 0 || iter <= 0 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, pin, salt, iter, len(hash))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(key, hash) == 1
}

// HashToken returns SHA-256(token). Only this digest is ever persisted.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
