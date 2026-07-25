// Package security implements the fixed security mechanisms described in
// refatoracao/08-seguranca.md: token generation, constant-time comparisons,
// the standard response headers, and the login rate limiter.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// NewToken returns a cryptographically random, base64url-encoded token of n
// raw bytes. Session and CSRF tokens both use n=32 (ver 08-seguranca.md).
func NewToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// VerifyPassword compares candidate against the configured ADMIN_PASSWORD in
// constant time (ver 08-seguranca.md, "Comparação em tempo constante").
func VerifyPassword(candidate, actual string) bool {
	return ConstantTimeEqual(candidate, actual)
}

// ConstantTimeEqual reports whether a and b are equal without leaking their
// contents through timing (used for the password check and for the CSRF
// double-submit comparison).
func ConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
