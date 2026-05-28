// Package apisecret generates and verifies the opaque client_secret returned
// by API-key creation. The secret is high-entropy random, so a single SHA-256
// is sufficient (matching the refresh_tokens.token_hash precedent); bcrypt is
// reserved for low-entropy human passwords.
package apisecret

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// secretBytes is the entropy of the opaque secret before hex-encoding.
const secretBytes = 32

// Generate returns a fresh opaque secret: "trakrf_" + 64 hex chars.
// The prefix aids secret scanning and log greppability.
func Generate() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api secret: %w", err)
	}
	return "trakrf_" + hex.EncodeToString(b), nil
}

// Hash returns the SHA-256 hex digest of the secret (64 chars).
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Verify reports whether presented hashes to storedHash, in constant time.
func Verify(presented, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(Hash(presented)), []byte(storedHash)) == 1
}
