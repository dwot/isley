package handlers

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/model/types"
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
// chartTime
// ---------------------------------------------------------------------------

// Range ends from the graph page are wall times in the instance zone; a
// date that ends the range takes in that whole day.
func TestChartTime(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	zones := chrono.NewZones(func() string { return "America/New_York" }, time.UTC)

	got, err := chartTime(zones, "2026-11-10", false)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 10, 0, 0, 0, 0, ny), got)
	got, err = chartTime(zones, "2026-11-10", true)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 11, 11, 0, 0, 0, 0, ny), got)
	got, err = chartTime(zones, "2026-11-10T08:30", true)
	require.NoError(t, err)
	assert.True(t, time.Date(2026, 11, 10, 13, 30, 0, 0, time.UTC).Equal(got))
	_, err = chartTime(zones, "not-a-date", false)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// SensorCacheService.DataPut LRU eviction
// ---------------------------------------------------------------------------

// newDataCache returns a SensorCacheService sized to the production
// default. The grouped TTL closure is irrelevant for these tests — only
// the data-cache half is exercised.
func newDataCache() *SensorCacheService {
	return NewSensorCacheService(nil, 0)
}

func TestSensorCache_DataPutStoresAndRetrievesEntries(t *testing.T) {
	t.Parallel()

	cache := newDataCache()
	cache.DataPut("k1", []types.SensorData{{ID: uint(1)}})
	cache.DataPut("k2", []types.SensorData{{ID: uint(2)}})

	assert.True(t, cache.DataHas("k1"))
	assert.True(t, cache.DataHas("k2"))
	assert.Equal(t, []string{"k1", "k2"}, cache.DataOrder(),
		"order tracks insertions")
}

func TestSensorCache_DataPutEvictsOldestPastCapacity(t *testing.T) {
	t.Parallel()

	const small = 4
	cache := NewSensorCacheService(nil, small)

	for i := 0; i < small; i++ {
		cache.DataPut("k"+strconv.Itoa(i), nil)
	}
	require.Equal(t, small, cache.DataLen())

	cache.DataPut("overflow", nil)

	assert.Equal(t, small, cache.DataLen(), "size stays at capacity")
	assert.False(t, cache.DataHas("k0"), "oldest key evicted")
	assert.True(t, cache.DataHas("overflow"))
	assert.True(t, cache.DataHas("k"+strconv.Itoa(small-1)),
		"newest pre-overflow key still present")
}

func TestSensorCache_DataPutUpdateInPlace(t *testing.T) {
	t.Parallel()

	cache := newDataCache()
	cache.DataPut("same", []types.SensorData{{ID: 1}})
	cache.DataPut("same", []types.SensorData{{ID: 2}})

	got, _, ok := cache.DataGet("same")
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.EqualValues(t, 2, got[0].ID,
		"second put should overwrite the entry's payload")
	assert.Equal(t, []string{"same"}, cache.DataOrder(),
		"updating an existing key must NOT duplicate the order entry")
}

// ---------------------------------------------------------------------------
// Concurrency smoke check — DataPut is safe under the service mutex.
// ---------------------------------------------------------------------------

func TestSensorCache_DataPutConcurrentWritersDoNotPanic(t *testing.T) {
	t.Parallel()

	cache := newDataCache()
	const writers = 8
	const perWriter = 32

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		w := w
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				cache.DataPut("w"+strconv.Itoa(w)+"-"+strconv.Itoa(i), nil)
			}
		}()
	}
	wg.Wait()

	// No assertions on exact size — the LRU eviction may have trimmed
	// some; the important property is that no race or panic occurred.
	assert.LessOrEqual(t, cache.DataLen(), defaultSensorDataMaxSize)
}

// TestSensorCache_DataResetClearsState verifies that DataReset removes
// every entry without affecting the configured capacity.
func TestSensorCache_DataResetClearsState(t *testing.T) {
	t.Parallel()

	cache := newDataCache()
	cache.DataPut("k1", nil)
	cache.DataPut("k2", nil)
	require.Equal(t, 2, cache.DataLen())

	cache.DataReset()
	assert.Zero(t, cache.DataLen(), "reset clears all entries")
	assert.Empty(t, cache.DataOrder(), "reset clears insertion order")
}

// TestSensorCache_GroupedRespectsTTL verifies that GroupedGet returns
// the cached payload while inside the TTL window and signals miss
// once the TTL has elapsed. Uses a small TTL so the test is fast and
// deterministic without faking time.
func TestSensorCache_GroupedRespectsTTL(t *testing.T) {
	t.Parallel()

	const ttl = 25 * time.Millisecond
	cache := NewSensorCacheService(func() time.Duration { return ttl }, 0)

	payload := map[string]map[string][]map[string]interface{}{
		"Z": {"D": []map[string]interface{}{{"id": 1}}},
	}
	cache.GroupedPut(payload)

	got, ok := cache.GroupedGet()
	require.True(t, ok, "fresh write should be served from cache")
	require.Contains(t, got, "Z")

	time.Sleep(ttl + 10*time.Millisecond)

	_, ok = cache.GroupedGet()
	assert.False(t, ok, "stale entry should miss cache and force refresh")
}

// TestSensorCache_GroupedResetEvictsCurrentEntry covers the test-only
// reset path used by integration tests that share a service across
// cases.
func TestSensorCache_GroupedResetEvictsCurrentEntry(t *testing.T) {
	t.Parallel()

	cache := NewSensorCacheService(func() time.Duration { return time.Hour }, 0)
	cache.GroupedPut(map[string]map[string][]map[string]interface{}{"Z": nil})
	_, ok := cache.GroupedGet()
	require.True(t, ok)

	cache.GroupedReset()
	_, ok = cache.GroupedGet()
	assert.False(t, ok, "reset must drop the cached payload")
}
