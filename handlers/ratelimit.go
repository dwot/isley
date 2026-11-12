package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"isley/logger"

	"github.com/gin-gonic/gin"
)

// ingestRateKey is the limiter key for an ingest request: the API key when
// one is sent, otherwise the client IP. logKey is safe to log.
func ingestRateKey(c *gin.Context) (key, logKey string) {
	if apiKey := c.GetHeader("X-API-KEY"); apiKey != "" {
		return "key:" + apiKey, "key:" + redactAPIKey(apiKey)
	}
	key = "ip:" + c.ClientIP()
	return key, key
}

// abortRateLimited sends the 429 with a Retry-After header, so clients can
// wait exactly as long as the window has left instead of guessing.
func abortRateLimited(c *gin.Context, logKey string, retryAfter time.Duration) {
	logger.Log.WithField("key", logKey).Warn("Rate limit exceeded")
	c.Header("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error": T(c, "api_rate_limit_exceeded"),
	})
}

// IngestRateLimitMiddleware returns a Gin middleware that rate-limits
// inbound ingest requests. It reads the per-engine ingest limiter from
// the RateLimiterService on the request context and keys on X-API-KEY
// if present, otherwise on the client IP. A request counts once here; the
// ingest handler counts the rest of a batch's readings after parsing it.
func IngestRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, logKey := ingestRateKey(c)
		if ok, retryAfter := RateLimiterServiceFromContext(c).Ingest().AllowN(key, 1); !ok {
			abortRateLimited(c, logKey, retryAfter)
			return
		}

		c.Next()
	}
}

var rateLimitRedactionKey = []byte(func() string {
	if v := os.Getenv("RATE_LIMIT_REDACTION_KEY"); v != "" {
		return v
	}
	return "default-rate-limit-redaction-key"
}())

// redactAPIKey returns a stable, non-reversible identifier for an API key
// so rate-limit logs can correlate offending callers without leaking the
// raw secret. Returns the first 12 hex chars of HMAC-SHA-256(key).
func redactAPIKey(key string) string {
	mac := hmac.New(sha256.New, rateLimitRedactionKey)
	mac.Write([]byte(key))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)[:12]
}
