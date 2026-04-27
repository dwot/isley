package config

import (
	"sync"

	"isley/model/types"
)

// Store carries the runtime-mutable configuration the application reads
// at request time. One instance per running app; constructed by
// app.NewEngine and made available to handlers via Gin context.
//
// All exported state is mutex-guarded; callers may freely read and write
// across goroutines. Tests construct their own Store per test;
// production wires up a single instance from the DB at startup. This
// per-engine scoping is what lets handler tests run with t.Parallel()
// without colliding on global config state.
type Store struct {
	mu sync.RWMutex

	pollingInterval    int
	aciEnabled         int
	ecEnabled          int
	aciToken           string
	ecDevices          []string
	activities         []types.Activity
	metrics            []types.Metric
	statuses           []types.Status
	zones              []types.Zone
	strains            []types.Strain
	breeders           []types.Breeder
	streams            []types.Stream
	sensorRetention    int
	guestMode          int
	streamGrabEnabled  int
	streamGrabInterval int
	apiKey             string
	apiIngestEnabled   int
	logLevel           string
	maxBackupSize      int64
	timezone           string
}

// Defaults mirrors the historical package-global initial values. Tests
// that build a Store directly inherit these; the production path goes
// through NewStore + handler.LoadSettings which overwrites with DB
// values.
const (
	defaultPollingInterval  = 60
	defaultAPIIngestEnabled = 1
	defaultLogLevel         = "info"
	defaultMaxBackupSize    = int64(5 * 1024 * 1024 * 1024) // 5 GB
)

// NewStore returns a Store seeded with the package-level defaults.
func NewStore() *Store {
	return &Store{
		pollingInterval:  defaultPollingInterval,
		apiIngestEnabled: defaultAPIIngestEnabled,
		logLevel:         defaultLogLevel,
		maxBackupSize:    defaultMaxBackupSize,
	}
}

// ----------------------------------------------------------------------
// Scalar getters/setters
// ----------------------------------------------------------------------

func (s *Store) PollingInterval() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pollingInterval
}

func (s *Store) SetPollingInterval(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pollingInterval = v
}

func (s *Store) ACIEnabled() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aciEnabled
}

func (s *Store) SetACIEnabled(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aciEnabled = v
}

func (s *Store) ECEnabled() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ecEnabled
}

func (s *Store) SetECEnabled(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ecEnabled = v
}

func (s *Store) ACIToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aciToken
}

func (s *Store) SetACIToken(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aciToken = v
}

func (s *Store) SensorRetention() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sensorRetention
}

func (s *Store) SetSensorRetention(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensorRetention = v
}

func (s *Store) GuestMode() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.guestMode
}

func (s *Store) SetGuestMode(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guestMode = v
}

func (s *Store) StreamGrabEnabled() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streamGrabEnabled
}

func (s *Store) SetStreamGrabEnabled(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamGrabEnabled = v
}

func (s *Store) StreamGrabInterval() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streamGrabInterval
}

func (s *Store) SetStreamGrabInterval(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streamGrabInterval = v
}

func (s *Store) APIKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiKey
}

func (s *Store) SetAPIKey(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKey = v
}

func (s *Store) APIIngestEnabled() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiIngestEnabled
}

func (s *Store) SetAPIIngestEnabled(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiIngestEnabled = v
}

func (s *Store) LogLevel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logLevel
}

func (s *Store) SetLogLevel(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logLevel = v
}

func (s *Store) MaxBackupSize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxBackupSize
}

func (s *Store) SetMaxBackupSize(v int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxBackupSize = v
}

func (s *Store) Timezone() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.timezone
}

func (s *Store) SetTimezone(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timezone = v
}

// ----------------------------------------------------------------------
// Slice getters/setters
//
// Getters return a snapshot copy so callers can read without holding the
// lock; setters install a new slice. Append helpers are provided for the
// slices that handlers mutate by append, so the read-modify-write step
// stays atomic under concurrent handler calls in the same engine.
// ----------------------------------------------------------------------

func (s *Store) ECDevices() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.ecDevices))
	copy(out, s.ecDevices)
	return out
}

func (s *Store) SetECDevices(v []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ecDevices = append(s.ecDevices[:0:0], v...)
}

func (s *Store) Activities() []types.Activity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Activity, len(s.activities))
	copy(out, s.activities)
	return out
}

func (s *Store) SetActivities(v []types.Activity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activities = append(s.activities[:0:0], v...)
}

func (s *Store) AppendActivity(a types.Activity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activities = append(s.activities, a)
}

func (s *Store) Metrics() []types.Metric {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Metric, len(s.metrics))
	copy(out, s.metrics)
	return out
}

func (s *Store) SetMetrics(v []types.Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics[:0:0], v...)
}

func (s *Store) AppendMetric(m types.Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics, m)
}

func (s *Store) Statuses() []types.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Status, len(s.statuses))
	copy(out, s.statuses)
	return out
}

func (s *Store) SetStatuses(v []types.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses = append(s.statuses[:0:0], v...)
}

func (s *Store) Zones() []types.Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Zone, len(s.zones))
	copy(out, s.zones)
	return out
}

func (s *Store) SetZones(v []types.Zone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones = append(s.zones[:0:0], v...)
}

func (s *Store) AppendZone(z types.Zone) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones = append(s.zones, z)
}

func (s *Store) Strains() []types.Strain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Strain, len(s.strains))
	copy(out, s.strains)
	return out
}

func (s *Store) SetStrains(v []types.Strain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strains = append(s.strains[:0:0], v...)
}

func (s *Store) Breeders() []types.Breeder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Breeder, len(s.breeders))
	copy(out, s.breeders)
	return out
}

func (s *Store) SetBreeders(v []types.Breeder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breeders = append(s.breeders[:0:0], v...)
}

func (s *Store) AppendBreeder(b types.Breeder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breeders = append(s.breeders, b)
}

func (s *Store) Streams() []types.Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.Stream, len(s.streams))
	copy(out, s.streams)
	return out
}

func (s *Store) SetStreams(v []types.Stream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams = append(s.streams[:0:0], v...)
}
