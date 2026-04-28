package handlers

// +parallel:serial — handlers/sensor_data.go sensorDataCache package-global
//
// TestSDCachePut_* mutate the package-global sensorDataCache map and
// sdCacheOrder slice. The withCleanCache helper snapshots and restores
// them on cleanup; running these tests in parallel would race on the
// snapshot/restore. The cache is a separate concern from the
// handlers/sensors.go chart cache that Phase 4.2 of TEST_PLAN_2.md
// covers — lifting it into per-engine state is a follow-on cleanup not
// scoped to any phase yet.

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// generateCacheKey
// ---------------------------------------------------------------------------

func TestGenerateCacheKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                              string
		sensor, minutes, start, end, want string
	}{
		{"all empty", "", "", "", "", "|||"},
		{"sensor only", "1", "", "", "", "1|||"},
		{"minutes branch", "1", "60", "", "", "1|60||"},
		{"date range branch", "1", "", "2026-01-01", "2026-01-31", "1||2026-01-01|2026-01-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, generateCacheKey(tc.sensor, tc.minutes, tc.start, tc.end))
		})
	}
}

// ---------------------------------------------------------------------------
// timeConversion
// ---------------------------------------------------------------------------

func TestTimeConversion(t *testing.T) {
	t.Parallel()

	t.Run("date-only is padded to midnight then converted to UTC", func(t *testing.T) {
		t.Parallel()
		got, err := timeConversion("2026-04-25")
		require.NoError(t, err)
		// Input is parsed as UTC by time.Parse(LayoutDB, ...), so the
		// UTC-formatted output is identical to the parsed input.
		assert.Equal(t, "2026-04-25 00:00:00", got)
	})

	t.Run("datetime input is converted to UTC", func(t *testing.T) {
		t.Parallel()
		got, err := timeConversion("2026-04-25 10:30:00")
		require.NoError(t, err)
		assert.Equal(t, "2026-04-25 10:30:00", got, "input is already in LayoutDB form, UTC normalization is a no-op")
	})

	t.Run("invalid input returns error", func(t *testing.T) {
		t.Parallel()
		// Inputs of length 10 get padded with " 00:00:00" before
		// parsing, so the returned string on error reflects the
		// post-padding form. We only assert the error is returned —
		// the exact value of the returned string is an implementation
		// detail callers shouldn't rely on.
		_, err := timeConversion("not-a-date")
		assert.Error(t, err)
	})

	t.Run("non-10-char invalid input returns error", func(t *testing.T) {
		t.Parallel()
		_, err := timeConversion("totally bogus value")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// sdCachePut LRU eviction
// ---------------------------------------------------------------------------

// withCleanCache snapshots and resets the package-global sensor-data
// cache so a test starts from a known empty state. The previous
// contents are restored via t.Cleanup so other tests in the same
// process aren't affected.
func withCleanCache(t *testing.T) {
	t.Helper()
	sdCacheMutex.Lock()
	prevMap := sensorDataCache
	prevOrder := sdCacheOrder
	sensorDataCache = make(map[string]cachedEntry, maxCacheEntries)
	sdCacheOrder = nil
	sdCacheMutex.Unlock()

	t.Cleanup(func() {
		sdCacheMutex.Lock()
		sensorDataCache = prevMap
		sdCacheOrder = prevOrder
		sdCacheMutex.Unlock()
	})
}

func TestSDCachePut_StoresAndRetrievesEntries(t *testing.T) {
	withCleanCache(t)

	sdCacheMutex.Lock()
	sdCachePut("k1", cachedEntry{timestamp: time.Now()})
	sdCachePut("k2", cachedEntry{timestamp: time.Now()})
	sdCacheMutex.Unlock()

	sdCacheMutex.RLock()
	defer sdCacheMutex.RUnlock()
	assert.Contains(t, sensorDataCache, "k1")
	assert.Contains(t, sensorDataCache, "k2")
	assert.Equal(t, []string{"k1", "k2"}, sdCacheOrder, "order tracks insertions")
}

func TestSDCachePut_EvictsOldestPastCapacity(t *testing.T) {
	withCleanCache(t)

	// Fill exactly to capacity.
	sdCacheMutex.Lock()
	for i := 0; i < maxCacheEntries; i++ {
		sdCachePut("k"+strconv.Itoa(i), cachedEntry{})
	}
	require.Len(t, sensorDataCache, maxCacheEntries)
	sdCacheMutex.Unlock()

	// One more — oldest ("k0") should evict.
	sdCacheMutex.Lock()
	sdCachePut("overflow", cachedEntry{})
	sdCacheMutex.Unlock()

	sdCacheMutex.RLock()
	defer sdCacheMutex.RUnlock()
	assert.Len(t, sensorDataCache, maxCacheEntries, "size stays at capacity")
	assert.NotContains(t, sensorDataCache, "k0", "oldest key evicted")
	assert.Contains(t, sensorDataCache, "overflow")
	assert.Contains(t, sensorDataCache, "k"+strconv.Itoa(maxCacheEntries-1), "newest pre-overflow key still present")
}

func TestSDCachePut_UpdateInPlace(t *testing.T) {
	withCleanCache(t)

	sdCacheMutex.Lock()
	sdCachePut("same", cachedEntry{timestamp: time.Unix(1, 0)})
	sdCachePut("same", cachedEntry{timestamp: time.Unix(2, 0)})
	sdCacheMutex.Unlock()

	sdCacheMutex.RLock()
	defer sdCacheMutex.RUnlock()
	assert.Equal(t, time.Unix(2, 0), sensorDataCache["same"].timestamp,
		"second put should overwrite the entry's payload")
	assert.Equal(t, []string{"same"}, sdCacheOrder,
		"updating an existing key must NOT duplicate the order entry")
}

// ---------------------------------------------------------------------------
// Concurrency smoke check — sdCachePut is safe under the mutex.
// ---------------------------------------------------------------------------

func TestSDCachePut_ConcurrentWritersDoNotPanic(t *testing.T) {
	withCleanCache(t)

	const writers = 8
	const perWriter = 32

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				sdCacheMutex.Lock()
				sdCachePut("w"+strconv.Itoa(w)+"-"+strconv.Itoa(i), cachedEntry{})
				sdCacheMutex.Unlock()
			}
		}()
	}
	wg.Wait()

	sdCacheMutex.RLock()
	defer sdCacheMutex.RUnlock()
	// No assertions on exact size — the LRU eviction may have trimmed
	// some; the important property is that no race or panic occurred.
	assert.LessOrEqual(t, len(sensorDataCache), maxCacheEntries)
}
