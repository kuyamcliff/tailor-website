// Package tokens generates opaque random tokens and their storage hashes.
// Raw tokens are only ever given to the client; the database stores SHA-256 hashes.
package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// New returns a URL-safe random token with n bytes of entropy.
func New(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func Hash(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// Matches compares a raw token to a stored hash in constant time.
func Matches(token string, hash []byte) bool {
	if token == "" || len(hash) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(Hash(token), hash) == 1
}
