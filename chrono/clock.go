package chrono

import (
	"sync"
	"time"
)

// Clock says what time it is. Production code takes a Clock instead of
// calling time.Now, so tests can pin time.
type Clock interface {
	Now() time.Time
}

// System returns the real clock.
func System() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// FixedClock is a Clock for tests: it returns a set instant until moved.
type FixedClock struct {
	mu  sync.Mutex
	now time.Time
}

// Fixed returns a FixedClock reading t.
func Fixed(t time.Time) *FixedClock { return &FixedClock{now: t} }

func (f *FixedClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the clock to t.
func (f *FixedClock) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance moves the clock forward by d.
func (f *FixedClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
