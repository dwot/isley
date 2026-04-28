package handlers

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginRateLimiter tracks per-IP login attempts over a sliding
// window. Differs from *RateLimiter (which uses fixed windows)
// because login attempts have specific user-visible semantics —
// "5 failures in the last minute" rather than "the last 5
// failures since the most recent window reset."
//
// The body of Allow is the algorithm previously implemented by the
// free function IsLoginRateLimited; the body of Reset is the
// algorithm previously implemented by ResetLoginAttempts. Lifting
// it onto a receiver removes the package-global map and lets every
// engine (production or per-test) carry its own counter.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewLoginRateLimiter returns a sliding-window login limiter that
// tolerates `limit` attempts per `window` per key.
func NewLoginRateLimiter(limit int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{
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
func (rl *LoginRateLimiter) Allow(key string) bool {
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
func (rl *LoginRateLimiter) Reset() {
	rl.mu.Lock()
	rl.attempts = make(map[string][]time.Time)
	rl.mu.Unlock()
}

// inject is a test-only helper used by the handlers/auth_test.go
// suite to seed pre-existing attempts for a key without going
// through Allow. Production code does not call it.
func (rl *LoginRateLimiter) inject(key string, ts ...time.Time) {
	rl.mu.Lock()
	rl.attempts[key] = append(rl.attempts[key], ts...)
	rl.mu.Unlock()
}

// RateLimiterService owns the per-engine rate limiters that
// production handlers consult. One instance per running app;
// constructed by app.NewEngine and made available to handlers via
// Gin context. Tests construct their own per-test instance via
// testutil.WithRateLimiterService so every test can call
// t.Parallel().
//
// The service exposes two limiters because the handlers package
// has two distinct rate-limited surfaces with different policies
// and different implementations: the ingest endpoint uses the
// fixed-window *RateLimiter struct, the login endpoint uses a
// sliding-window per-IP attempt counter. The service holds
// whatever shape each one needs and lets handlers reach the right
// one through one accessor.
type RateLimiterService struct {
	mu     sync.RWMutex
	ingest *RateLimiter
	login  *LoginRateLimiter
}

const contextKeyRateLimiterService = "rateLimiterService"

// defaultIngestPerMinute is the fixed-window allowance the default
// ingest limiter falls back to when the caller does not pass one in.
// Mirrors the value the package-global IngestRateLimiter used before
// Phase 4.1.
const defaultIngestPerMinute = 60

// NewRateLimiterService returns a service holding the supplied
// limiters. A nil ingest or login argument is replaced with a
// default-policy instance so the engine always has something to
// reach for. Production callers normally pass nil for both; tests
// that exercise specific 429 branches pass tightened policies.
func NewRateLimiterService(ingest *RateLimiter, login *LoginRateLimiter) *RateLimiterService {
	if ingest == nil {
		ingest = NewRateLimiter(defaultIngestPerMinute, time.Minute)
	}
	if login == nil {
		login = NewLoginRateLimiter(MaxLoginAttempts, time.Minute)
	}
	return &RateLimiterService{ingest: ingest, login: login}
}

// Ingest returns the current ingest limiter. Callers should treat
// the returned pointer as a read handle — mutate state via Allow,
// not by replacing the limiter directly. SetIngest is the swap path.
func (s *RateLimiterService) Ingest() *RateLimiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ingest
}

// Login returns the current login limiter. Same contract as Ingest.
func (s *RateLimiterService) Login() *LoginRateLimiter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.login
}

// SetIngest swaps the underlying ingest limiter atomically.
// Production code does not call this; tests do, when they need a
// per-test policy (e.g. low allowance for 429 exercises).
func (s *RateLimiterService) SetIngest(r *RateLimiter) {
	s.mu.Lock()
	s.ingest = r
	s.mu.Unlock()
}

// SetLogin swaps the underlying login limiter atomically. Same
// contract as SetIngest.
func (s *RateLimiterService) SetLogin(r *LoginRateLimiter) {
	s.mu.Lock()
	s.login = r
	s.mu.Unlock()
}

// RateLimiterServiceFromContext extracts the *RateLimiterService
// the engine middleware injected into the Gin context. Mirrors
// BackupServiceFromContext / ConfigStoreFromContext.
func RateLimiterServiceFromContext(c *gin.Context) *RateLimiterService {
	return c.MustGet(contextKeyRateLimiterService).(*RateLimiterService)
}

// SetRateLimiterServiceOnContext is the small helper the engine
// middleware uses to bind a service to a request context.
func SetRateLimiterServiceOnContext(c *gin.Context, s *RateLimiterService) {
	c.Set(contextKeyRateLimiterService, s)
}
