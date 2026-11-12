package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// LoginLimiter
// ---------------------------------------------------------------------------

// LoginLimiter.Allow returns true while attempts in the window
// remain at or below the limit and false once the limit is exceeded.
func TestLoginLimiter_LocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	rl := NewLogin(MaxLoginAttempts, time.Minute)
	const ip = "10.0.0.1"

	// First MaxLoginAttempts calls must each be allowed.
	for i := 1; i <= MaxLoginAttempts; i++ {
		assert.Truef(t, rl.Allow(ip), "attempt %d/%d should be allowed", i, MaxLoginAttempts)
	}

	// The next call must be rejected.
	assert.False(t, rl.Allow(ip), "attempt MaxLoginAttempts+1 should be blocked")
}

func TestLoginLimiter_PerIPIsolation(t *testing.T) {
	t.Parallel()

	rl := NewLogin(MaxLoginAttempts, time.Minute)

	// Burn one IP past its limit.
	for i := 0; i < MaxLoginAttempts+1; i++ {
		rl.Allow("10.0.0.1")
	}

	// A different IP must still be allowed.
	assert.True(t, rl.Allow("10.0.0.2"), "second IP should not be affected by first IP's lockout")
}

func TestLoginLimiter_OldAttemptsDropOff(t *testing.T) {
	t.Parallel()

	rl := NewLogin(MaxLoginAttempts, time.Minute)
	const ip = "10.0.0.3"

	// Inject MaxLoginAttempts+2 attempts that are 2 minutes old —
	// outside the 1-minute window. They should not count toward the
	// limit when a fresh attempt arrives.
	old := time.Now().Add(-2 * time.Minute)
	rl.inject(ip, old, old, old, old, old, old, old)

	// A fresh attempt joins an empty effective window.
	assert.True(t, rl.Allow(ip), "stale attempts should be ignored")
}

// TestLoginLimiter_ResetClearsState verifies Reset wipes per-key
// state so a previously-blocked key can immediately Allow again. Production
// code does not call Reset; this is the test-only path.
func TestLoginLimiter_ResetClearsState(t *testing.T) {
	t.Parallel()

	rl := NewLogin(2, time.Minute)
	const ip = "10.0.0.4"
	for i := 0; i < 3; i++ {
		rl.Allow(ip)
	}
	require.False(t, rl.Allow(ip), "fourth attempt is expected to be blocked")

	rl.Reset()
	assert.True(t, rl.Allow(ip), "after Reset the same key should be allowed")
}
