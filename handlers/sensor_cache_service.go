package handlers

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"isley/model/types"
)

// SensorCacheService owns the per-engine sensor chart caches.
// Two distinct caches share this service:
//
//   - Grouped: a single-bucket cache feeding the sensors-tab chart
//     endpoint. Refreshed against a TTL derived from PollingInterval.
//     The whole bucket is replaced atomically on refresh.
//   - Data: a per-query LRU cache feeding the dashboard chart
//     endpoint. Each cached entry is keyed by the chart's parameters
//     and evicted in insertion order once the bucket exceeds
//     dataMaxSize.
//
// Both buckets live behind the same service so production main
// constructs one instance and threads it once. Tests get their own
// per-test instance via NewTestServer; the per-engine scoping is
// what unblocks t.Parallel() on the chart-exercising tests.
type SensorCacheService struct {
	groupedMu          sync.RWMutex
	grouped            map[string]map[string][]map[string]interface{}
	groupedLastUpdated time.Time
	groupedTTL         func() time.Duration

	dataMu      sync.Mutex
	data        map[string]sensorDataEntry
	dataOrder   []string
	dataMaxSize int
}

type sensorDataEntry struct {
	data      []types.SensorData
	timestamp time.Time
}

const (
	contextKeySensorCache    = "sensorCacheService"
	defaultSensorDataMaxSize = 256
)

// NewSensorCacheService constructs a service. groupedTTL is the
// closure handlers consult on refresh — typically reading
// PollingInterval()/10 seconds off the engine's *config.Store so a
// settings change takes effect on the next cache miss without
// re-wiring the service. Pass a nil closure to default to a one-second
// TTL (production-equivalent for an unset PollingInterval). dataMaxSize
// caps the LRU bucket; pass 0 for the default (256).
func NewSensorCacheService(groupedTTL func() time.Duration, dataMaxSize int) *SensorCacheService {
	if groupedTTL == nil {
		groupedTTL = func() time.Duration { return time.Second }
	}
	if dataMaxSize <= 0 {
		dataMaxSize = defaultSensorDataMaxSize
	}
	return &SensorCacheService{
		grouped:     nil,
		groupedTTL:  groupedTTL,
		data:        make(map[string]sensorDataEntry, dataMaxSize),
		dataMaxSize: dataMaxSize,
	}
}

// --- Grouped cache ---

// GroupedGet returns the cached grouped-sensor payload if it is still
// within TTL. The boolean is false when the cache is empty or stale;
// callers should refresh via GroupedPut.
func (s *SensorCacheService) GroupedGet() (map[string]map[string][]map[string]interface{}, bool) {
	s.groupedMu.RLock()
	defer s.groupedMu.RUnlock()
	if s.grouped == nil {
		return nil, false
	}
	if time.Since(s.groupedLastUpdated) >= s.groupedTTL() {
		return nil, false
	}
	return s.grouped, true
}

// GroupedPut atomically replaces the grouped-sensor bucket and stamps
// the last-updated time used to evaluate TTL on subsequent reads.
func (s *SensorCacheService) GroupedPut(data map[string]map[string][]map[string]interface{}) {
	s.groupedMu.Lock()
	s.grouped = data
	s.groupedLastUpdated = time.Now()
	s.groupedMu.Unlock()
}

// GroupedReset clears the grouped bucket. Production code does not
// call this; tests do, between cases that share a service.
func (s *SensorCacheService) GroupedReset() {
	s.groupedMu.Lock()
	s.grouped = nil
	s.groupedLastUpdated = time.Time{}
	s.groupedMu.Unlock()
}

// --- Data cache ---

// DataGet returns the cached chart payload for key alongside its
// insertion timestamp. The boolean is false when the key is absent;
// callers should issue a fresh query and follow up with DataPut.
func (s *SensorCacheService) DataGet(key string) ([]types.SensorData, time.Time, bool) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	entry, ok := s.data[key]
	if !ok {
		return nil, time.Time{}, false
	}
	return entry.data, entry.timestamp, true
}

// DataPut inserts or updates a cache entry for key, evicting the
// oldest entries when the cache exceeds dataMaxSize. Existing keys
// are updated in place without disturbing insertion order.
func (s *SensorCacheService) DataPut(key string, data []types.SensorData) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	if _, exists := s.data[key]; !exists {
		s.dataOrder = append(s.dataOrder, key)
	}
	s.data[key] = sensorDataEntry{
		data:      data,
		timestamp: time.Now().In(time.Local),
	}
	for len(s.data) > s.dataMaxSize {
		oldest := s.dataOrder[0]
		s.dataOrder = s.dataOrder[1:]
		delete(s.data, oldest)
	}
}

// DataReset clears the LRU bucket. Production code does not call this;
// tests do, between cases that share a service.
func (s *SensorCacheService) DataReset() {
	s.dataMu.Lock()
	s.data = make(map[string]sensorDataEntry, s.dataMaxSize)
	s.dataOrder = nil
	s.dataMu.Unlock()
}

// DataLen returns the number of entries currently in the LRU bucket.
// Used by tests to assert on eviction; production code does not call it.
func (s *SensorCacheService) DataLen() int {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	return len(s.data)
}

// DataHas reports whether key is present in the LRU bucket. Used by
// tests; production code reads via DataGet.
func (s *SensorCacheService) DataHas(key string) bool {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	_, ok := s.data[key]
	return ok
}

// DataOrder returns a copy of the insertion-order slice. Used by
// tests to assert eviction order; production code does not call it.
func (s *SensorCacheService) DataOrder() []string {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()
	out := make([]string, len(s.dataOrder))
	copy(out, s.dataOrder)
	return out
}

// DataMaxSize returns the configured LRU capacity.
func (s *SensorCacheService) DataMaxSize() int {
	return s.dataMaxSize
}

// SensorCacheServiceFromContext extracts the *SensorCacheService the
// engine middleware injected into the Gin context. Mirrors
// RateLimiterServiceFromContext / BackupServiceFromContext.
func SensorCacheServiceFromContext(c *gin.Context) *SensorCacheService {
	return c.MustGet(contextKeySensorCache).(*SensorCacheService)
}

// SetSensorCacheServiceOnContext is the small helper the engine
// middleware uses to bind a service to a request context.
func SetSensorCacheServiceOnContext(c *gin.Context, s *SensorCacheService) {
	c.Set(contextKeySensorCache, s)
}
