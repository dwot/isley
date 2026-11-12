package handlers

import (
	"github.com/gin-gonic/gin"

	"isley/ratelimit"
)

const contextKeyRateLimiterService = "rateLimiterService"

// RateLimiterServiceFromContext extracts the *ratelimit.Service
// the engine middleware injected into the Gin context. Mirrors
// BackupServiceFromContext / ConfigStoreFromContext.
func RateLimiterServiceFromContext(c *gin.Context) *ratelimit.Service {
	return c.MustGet(contextKeyRateLimiterService).(*ratelimit.Service)
}

// SetRateLimiterServiceOnContext is the small helper the engine
// middleware uses to bind a service to a request context.
func SetRateLimiterServiceOnContext(c *gin.Context, s *ratelimit.Service) {
	c.Set(contextKeyRateLimiterService, s)
}
