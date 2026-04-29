package utils

import (
	"os"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the bcrypt work factor used when hashing passwords and
// API keys. Defaults to 12 (a small bump over bcrypt.DefaultCost = 10);
// can be lowered for tests via ISLEY_BCRYPT_COST to keep suite runtime
// reasonable. Reads from the env at process start.
var BcryptCost = resolveBcryptCost()

func resolveBcryptCost() int {
	const fallback = 12
	v := os.Getenv("ISLEY_BCRYPT_COST")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < bcrypt.MinCost || n > bcrypt.MaxCost {
		return fallback
	}
	return n
}

// HashPassword hashes a plain text password
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	return string(hash), err
}

// CheckPasswordHash compares a plain text password with a hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
