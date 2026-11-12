package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"isley/logger"
)

// ensureLoggerForTests wires logger.Log to a discard sink so production
// code paths that log unconditionally do not nil-panic in unit tests.
// The handlers package can't import tests/testutil (that would cycle
// through model→handlers), so we mirror the harness logic here.
var loggerOnce sync.Once

func ensureLoggerForTests() {
	loggerOnce.Do(func() {
		l := logrus.New()
		l.SetOutput(io.Discard)
		l.SetLevel(logrus.PanicLevel)
		logger.Log = l
	})
}

// ---------------------------------------------------------------------------
// ValidatePasswordComplexity
// ---------------------------------------------------------------------------

func TestValidatePasswordComplexity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, password string
		wantOK         bool
	}{
		{"empty", "", false},
		{"shorter than 8", "1234567", false},
		{"exactly 8", "12345678", true},
		{"comfortably long", "a-quite-long-passphrase", true},
		{"unicode 8 runes but 8 bytes", "abcdefgh", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidatePasswordComplexity(tc.password)
			if tc.wantOK {
				assert.Empty(t, got, "expected no error message for password %q", tc.password)
			} else {
				assert.NotEmpty(t, got, "expected an error message for password %q", tc.password)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GenerateCSRFToken
// ---------------------------------------------------------------------------

func TestGenerateCSRFToken(t *testing.T) {
	ensureLoggerForTests()
	t.Parallel()

	tok := GenerateCSRFToken()
	assert.NotEmpty(t, tok, "token must not be empty")
	assert.Len(t, tok, 64, "32 bytes hex-encoded is 64 chars")

	if _, err := hex.DecodeString(tok); err != nil {
		t.Fatalf("token %q is not valid hex: %v", tok, err)
	}

	// Successive calls return distinct values.
	other := GenerateCSRFToken()
	assert.NotEqual(t, tok, other, "consecutive tokens must differ")
}

// ---------------------------------------------------------------------------
// HashAPIKey / CheckAPIKey
// ---------------------------------------------------------------------------

func TestHashAPIKey_RoundTripsWithBcrypt(t *testing.T) {
	ensureLoggerForTests()
	t.Parallel()

	const key = "test-api-key-123"
	hash := HashAPIKey(key)

	require.NotEmpty(t, hash, "hash must not be empty")
	assert.Truef(t,
		bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)) == nil,
		"bcrypt.CompareHashAndPassword should accept the key",
	)
}

func TestCheckAPIKey_BcryptMatch(t *testing.T) {
	ensureLoggerForTests()
	t.Parallel()

	const key = "good-key"
	hash := HashAPIKey(key)

	match, legacy := CheckAPIKey(key, hash)
	assert.True(t, match, "bcrypt key should match")
	assert.False(t, legacy, "bcrypt match must not be flagged legacy")

	mismatch, legacy2 := CheckAPIKey("wrong-key", hash)
	assert.False(t, mismatch, "wrong key against bcrypt hash must not match")
	assert.False(t, legacy2)
}

func TestCheckAPIKey_LegacySHA256Match(t *testing.T) {
	t.Parallel()

	const key = "old-sha-key"
	digest := sha256.Sum256([]byte(key))
	stored := hex.EncodeToString(digest[:]) // 64 hex chars triggers the SHA-256 branch

	match, legacy := CheckAPIKey(key, stored)
	assert.True(t, match, "SHA-256 hex match expected")
	assert.True(t, legacy, "match must be flagged legacy so callers can upgrade")

	miss, legacy2 := CheckAPIKey("nope", stored)
	assert.False(t, miss)
	assert.False(t, legacy2)
}

func TestCheckAPIKey_PlaintextMatch(t *testing.T) {
	t.Parallel()

	// "stored" is neither a bcrypt hash nor a 64-char hex digest, so the
	// plaintext branch wins.
	const key = "plaintext-legacy"

	match, legacy := CheckAPIKey(key, key)
	assert.True(t, match, "plaintext compare expected to match")
	assert.True(t, legacy, "plaintext match should be flagged legacy")

	miss, legacy2 := CheckAPIKey("other", key)
	assert.False(t, miss)
	assert.False(t, legacy2)
}
