package handlers

// API key generation, hashing and comparison.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"isley/logger"
	"isley/utils"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func GenerateAPIKey() string {
	// Generate a random 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// HashAPIKey returns a bcrypt hash of the given plaintext API key.
// bcrypt is preferred over fast hashes like SHA-256 for secret storage
// because its adaptive cost factor resists brute-force attacks.
func HashAPIKey(plaintext string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), utils.BcryptCost)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to hash API key")
		return ""
	}
	return string(hash)
}

// CheckAPIKey compares a plaintext API key against a stored hash.
// It supports bcrypt hashes (preferred), legacy SHA-256 hex digests,
// and plaintext matches for backward compatibility.
// Returns (match bool, legacy bool) — legacy is true when the key matched
// via SHA-256 or plaintext so the caller can auto-upgrade to bcrypt.
func CheckAPIKey(plaintext, stored string) (match bool, legacy bool) {
	// Try bcrypt first (hashes start with "$2a$" or "$2b$")
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plaintext)) == nil, false
	}
	// Fall back to SHA-256 hex comparison for legacy hashes (64 hex chars)
	if len(stored) == 64 {
		h := sha256.Sum256([]byte(plaintext))
		incomingHex := hex.EncodeToString(h[:])
		if subtle.ConstantTimeCompare([]byte(incomingHex), []byte(stored)) == 1 {
			return true, true
		}
		return false, false
	}
	// Fall back to direct comparison for very old plaintext keys
	if subtle.ConstantTimeCompare([]byte(plaintext), []byte(stored)) == 1 {
		return true, true
	}
	return false, false
}
