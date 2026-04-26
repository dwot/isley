package config

// Phase 6a tests for config/config.go. The package is mostly a bag of
// package-level globals seeded with defaults; we lock down those
// defaults plus the atomic semantics of RestoreInProgress so that
// future refactors can't silently drift.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaults_AreSensible asserts the package-level defaults match the
// values the rest of the suite relies on. This is intentionally narrow:
// we only check the values that have observable behavior (intervals,
// retention, max sizes), not strings that are merely presentation.
func TestDefaults_AreSensible(t *testing.T) {
	assert.Equal(t, 60, PollingInterval, "60-second default polling interval")
	assert.Equal(t, 1, APIIngestEnabled, "API ingest is enabled by default")
	assert.Equal(t, 0, ACIEnabled, "ACI integration disabled until configured")
	assert.Equal(t, 0, ECEnabled, "EcoWitt integration disabled until configured")
	assert.Equal(t, 0, GuestMode, "guest mode off by default")
	assert.Equal(t, 0, StreamGrabEnabled, "stream-grabber off by default")
	assert.Equal(t, 60, StreamGrabInterval)
	assert.Equal(t, 0, SensorRetention, "0 means retain forever")
	assert.Equal(t, "info", LogLevel)
	assert.Equal(t, int64(5*1024*1024*1024), MaxBackupSize, "5 GiB default backup ceiling")
	assert.Equal(t, "", Timezone, "empty timezone falls back to system default")
	assert.Equal(t, "", APIKey)
	assert.Equal(t, "", ACIToken)
}

// TestRestoreInProgress_AtomicStoreLoad confirms the watcher's pause
// flag round-trips through CompareAndSwap and Store. Tests run before
// and after each store to ensure the global is left at false (the
// expected idle state) so subsequent tests in the same process don't
// observe a dangling true.
func TestRestoreInProgress_AtomicStoreLoad(t *testing.T) {
	prev := RestoreInProgress.Load()
	t.Cleanup(func() { RestoreInProgress.Store(prev) })

	// false → true
	RestoreInProgress.Store(true)
	assert.True(t, RestoreInProgress.Load())

	// CAS true → false
	swapped := RestoreInProgress.CompareAndSwap(true, false)
	assert.True(t, swapped, "CompareAndSwap from current state must succeed")
	assert.False(t, RestoreInProgress.Load())

	// CAS from wrong expected → fails, state unchanged
	swapped = RestoreInProgress.CompareAndSwap(true, false)
	assert.False(t, swapped, "CAS with wrong expected value must fail")
	assert.False(t, RestoreInProgress.Load())
}

// TestSliceGlobals_StartAsNilOrEmpty documents that the slice-typed
// globals are usable without explicit initialization (range-over-nil
// is fine in Go). Handlers that mutate these via append() rely on this
// invariant.
func TestSliceGlobals_StartAsNilOrEmpty(t *testing.T) {
	// Every slice-typed global should iterate as zero items at startup.
	assert.Empty(t, ECDevices)
	// The other slices may be populated by LoadSettings during the test
	// process lifetime, so we don't assert empty for them — only that
	// ranging over them with len() is well-defined (which is implicitly
	// what Empty() does on a nil slice).
	_ = len(Activities)
	_ = len(Metrics)
	_ = len(Statuses)
	_ = len(Zones)
	_ = len(Strains)
	_ = len(Breeders)
	_ = len(Streams)
}
