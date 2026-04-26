package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// RateLimiter.Allow
// ---------------------------------------------------------------------------

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(3, time.Minute)

	for i := 1; i <= 3; i++ {
		assert.Truef(t, rl.Allow("k1"), "request %d/3 should be allowed", i)
	}
}

func TestRateLimiter_BlocksAfterLimit(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		_ = rl.Allow("k1")
	}

	assert.False(t, rl.Allow("k1"), "4th request should be blocked")
	assert.False(t, rl.Allow("k1"), "subsequent requests in window remain blocked")
}

func TestRateLimiter_PerKeyIsolation(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(2, time.Minute)
	// Burn k1 to its limit.
	assert.True(t, rl.Allow("k1"))
	assert.True(t, rl.Allow("k1"))
	assert.False(t, rl.Allow("k1"))

	// k2 has its own counter.
	assert.True(t, rl.Allow("k2"))
	assert.True(t, rl.Allow("k2"))
	assert.False(t, rl.Allow("k2"))

	// k1 should still be blocked.
	assert.False(t, rl.Allow("k1"))
}

func TestRateLimiter_NewWindowResetsCount(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(2, time.Millisecond)
	assert.True(t, rl.Allow("k1"))
	assert.True(t, rl.Allow("k1"))
	assert.False(t, rl.Allow("k1"))

	// Wait for the window to expire.
	time.Sleep(5 * time.Millisecond)

	assert.True(t, rl.Allow("k1"), "after window expiry the counter should reset")
}

// TestRateLimiter_CleanupRemovesExpiredEntries directly inspects the
// limiter's internals to assert that expired entries don't accumulate
// indefinitely. Without this check a long-running process would
// silently leak memory.
func TestRateLimiter_CleanupRemovesExpiredEntries(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(1, time.Millisecond)
	for _, k := range []string{"a", "b", "c"} {
		rl.Allow(k)
	}

	// All three entries are present immediately.
	rl.mu.Lock()
	assert.Len(t, rl.entries, 3)
	rl.mu.Unlock()

	// Wait for the window to expire then trigger cleanup via another
	// Allow call (cleanup runs at most once per window).
	time.Sleep(5 * time.Millisecond)
	rl.Allow("d")

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// "d" is fresh; a/b/c should be evicted by cleanup. Allow some
	// slack — cleanup is best-effort.
	assert.Contains(t, rl.entries, "d")
	for _, stale := range []string{"a", "b", "c"} {
		assert.NotContains(t, rl.entries, stale, "expired entry %q should be cleaned up", stale)
	}
}
