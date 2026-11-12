// Package ratelimit limits how often a client may do something: push
// sensor readings (a fixed window per key) or try to log in (a sliding
// window per IP).
package ratelimit

import (
	"sync"
	"time"
)

// MaxLoginAttempts is the number of failed login attempts within the
// window before further attempts are blocked.
const MaxLoginAttempts = 5

// entry tracks the request count and window start for a single key.
type entry struct {
	count     int
	windowEnd time.Time
}

// Limiter provides a simple fixed-window rate limiter keyed by client
// identifier (IP address or API key).
type Limiter struct {
	mu        sync.Mutex
	entries   map[string]*entry
	limit     int
	window    time.Duration
	lastClean time.Time
}

// New creates a rate limiter that allows `limit` requests per
// `window` duration for each unique key.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		entries:   make(map[string]*entry),
		limit:     limit,
		window:    window,
		lastClean: time.Now(),
	}
}

// cleanup removes expired entries. Called under lock periodically.
func (rl *Limiter) cleanup() {
	now := time.Now()
	// Only clean every window duration to avoid doing this on every request
	if now.Sub(rl.lastClean) < rl.window {
		return
	}
	for key, e := range rl.entries {
		if now.After(e.windowEnd) {
			delete(rl.entries, key)
		}
	}
	rl.lastClean = now
}

// Allow checks whether the given key is within the rate limit. Returns true
// if the request is allowed.
func (rl *Limiter) Allow(key string) bool {
	ok, _ := rl.AllowN(key, 1)
	return ok
}

// AllowN counts n requests against the key at once (a batch ingest counts
// each reading). When the key is over its limit it also returns how long
// until the current window ends.
func (rl *Limiter) AllowN(key string, n int) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanup()

	now := time.Now()
	e, exists := rl.entries[key]

	if !exists || now.After(e.windowEnd) {
		// New window
		e = &entry{windowEnd: now.Add(rl.window)}
		rl.entries[key] = e
	}

	// Only charge the window when the call fits. Charging a rejected call
	// would let one oversized batch lock the key out until the window ends.
	if e.count+n > rl.limit {
		return false, e.windowEnd.Sub(now)
	}
	e.count += n
	return true, 0
}

// LoginLimiter tracks per-IP login attempts over a sliding
// window. Differs from *Limiter (which uses fixed windows)
// because login attempts have specific user-visible semantics —
// "5 failures in the last minute" rather than "the last 5
// failures since the most recent window reset." Each engine has its
// own, so tests don't share counters.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewLogin returns a sliding-window login limiter that
// tolerates `limit` attempts per `window` per key.
func NewLogin(limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow records a new attempt for key and returns true if the count
// of attempts within the trailing window remains at or below the
// configured limit. Once the limit is exceeded, Allow returns false
// for subsequent attempts until older entries fall outside the
// window.
func (rl *LoginLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)
	attempts := rl.attempts[key]
	var recent []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	rl.attempts[key] = recent
	return len(recent) <= rl.limit
}

// Reset clears all per-key state. Production code does not call
// this; tests do, between cases.
func (rl *LoginLimiter) Reset() {
	rl.mu.Lock()
	rl.attempts = make(map[string][]time.Time)
	rl.mu.Unlock()
}

// inject seeds attempts for a key without going through Allow, for
// tests. Production code does not call it.
func (rl *LoginLimiter) inject(key string, ts ...time.Time) {
	rl.mu.Lock()
	rl.attempts[key] = append(rl.attempts[key], ts...)
	rl.mu.Unlock()
}

// Service owns the per-engine rate limiters that
// production handlers consult. One instance per running app;
// constructed by app.NewEngine and made available to handlers via
// Gin context. Tests construct their own per-test instance via
// testutil.WithRateLimiterService so every test can call
// t.Parallel().
//
// The service exposes two limiters because the handlers package
// has two distinct rate-limited surfaces with different policies
// and different implementations: the ingest endpoint uses the
// fixed-window *Limiter struct, the login endpoint uses a
// sliding-window per-IP attempt counter. The service holds
// whatever shape each one needs and lets handlers reach the right
// one through one accessor.
type Service struct {
	mu     sync.RWMutex
	ingest *Limiter
	login  *LoginLimiter
}

// IngestPerMinute is the default ingest allowance: readings per key per
// minute. The largest ingest batch is the same, so a full batch fits.
const IngestPerMinute = 60

// NewService returns a service holding the supplied
// limiters. A nil ingest or login argument is replaced with a
// default-policy instance so the engine always has something to
// reach for. Production callers normally pass nil for both; tests
// that exercise specific 429 branches pass tightened policies.
func NewService(ingest *Limiter, login *LoginLimiter) *Service {
	if ingest == nil {
		ingest = New(IngestPerMinute, time.Minute)
	}
	if login == nil {
		login = NewLogin(MaxLoginAttempts, time.Minute)
	}
	return &Service{ingest: ingest, login: login}
}

// Ingest returns the current ingest limiter. Callers should treat
// the returned pointer as a read handle — mutate state via Allow,
// not by replacing the limiter directly. SetIngest is the swap path.
func (s *Service) Ingest() *Limiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ingest
}

// Login returns the current login limiter. Same contract as Ingest.
func (s *Service) Login() *LoginLimiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.login
}

// SetIngest swaps the underlying ingest limiter atomically.
// Production code does not call this; tests do, when they need a
// per-test policy (e.g. low allowance for 429 exercises).
func (s *Service) SetIngest(r *Limiter) {
	s.mu.Lock()
	s.ingest = r
	s.mu.Unlock()
}

// SetLogin swaps the underlying login limiter atomically. Same
// contract as SetIngest.
func (s *Service) SetLogin(r *LoginLimiter) {
	s.mu.Lock()
	s.login = r
	s.mu.Unlock()
}
