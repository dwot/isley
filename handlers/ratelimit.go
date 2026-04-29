package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"isley/logger"
)

// rateLimitEntry tracks the request count and window start for a single key.
type rateLimitEntry struct {
	count     int
	windowEnd time.Time
}

// RateLimiter provides a simple fixed-window rate limiter keyed by client
// identifier (IP address or API key).
type RateLimiter struct {
	mu        sync.Mutex
	entries   map[string]*rateLimitEntry
	limit     int
	window    time.Duration
	lastClean time.Time
}

// NewRateLimiter creates a rate limiter that allows `limit` requests per
// `window` duration for each unique key.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries:   make(map[string]*rateLimitEntry),
		limit:     limit,
		window:    window,
		lastClean: time.Now(),
	}
}

// cleanup removes expired entries. Called under lock periodically.
func (rl *RateLimiter) cleanup() {
	now := time.Now()
	// Only clean every window duration to avoid doing this on every request
	if now.Sub(rl.lastClean) < rl.window {
		return
	}
	for key, entry := range rl.entries {
		if now.After(entry.windowEnd) {
			delete(rl.entries, key)
		}
	}
	rl.lastClean = now
}

// Allow checks whether the given key is within the rate limit. Returns true
// if the request is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanup()

	now := time.Now()
	entry, exists := rl.entries[key]

	if !exists || now.After(entry.windowEnd) {
		// New window
		rl.entries[key] = &rateLimitEntry{
			count:     1,
			windowEnd: now.Add(rl.window),
		}
		return true
	}

	entry.count++
	return entry.count <= rl.limit
}

// IngestRateLimitMiddleware returns a Gin middleware that rate-limits
// inbound ingest requests. It reads the per-engine ingest limiter from
// the RateLimiterService on the request context and keys on X-API-KEY
// if present, otherwise on the client IP.
func IngestRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rl := RateLimiterServiceFromContext(c).Ingest()

		var key, logKey string
		if apiKey := c.GetHeader("X-API-KEY"); apiKey != "" {
			key = "key:" + apiKey
			logKey = "key:" + redactAPIKey(apiKey)
		} else {
			key = "ip:" + c.ClientIP()
			logKey = key
		}

		if !rl.Allow(key) {
			logger.Log.WithField("key", logKey).Warn("Rate limit exceeded")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": T(c, "api_rate_limit_exceeded"),
			})
			return
		}

		c.Next()
	}
}

// redactAPIKey returns a stable, non-reversible identifier for an API key
// so rate-limit logs can correlate offending callers without leaking the
// raw secret. Returns the first 12 hex chars of SHA-256(key).
func redactAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}
