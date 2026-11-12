package apiv1

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/scope"
)

// Key is the API key a request authenticated with.
type Key struct {
	ID     int
	Name   string
	Scopes scope.Set
}

// KeyVerifier checks a presented key. It returns nil, nil when the key is
// not valid.
type KeyVerifier func(ctx context.Context, plaintext string) (*Key, error)

const contextKeyAuth = "apiv1.key"

// presentedKey returns the key from "Authorization: Bearer" (preferred) or
// "X-API-KEY".
func presentedKey(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		scheme, value, ok := strings.Cut(h, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(value)
		}
		return ""
	}
	return strings.TrimSpace(c.GetHeader("X-API-KEY"))
}

// requireKey authenticates every request with an API key.
func requireKey(verify KeyVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		plaintext := presentedKey(c)
		if plaintext == "" {
			c.Header("WWW-Authenticate", `Bearer realm="isley"`)
			abortWith(c, http.StatusUnauthorized, CodeUnauthorized,
				"an API key is required: send it in an Authorization: Bearer header", nil)
			return
		}
		key, err := verify(c.Request.Context(), plaintext)
		if err != nil {
			writeError(c, err)
			return
		}
		if key == nil {
			c.Header("WWW-Authenticate", `Bearer realm="isley", error="invalid_token"`)
			abortWith(c, http.StatusUnauthorized, CodeUnauthorized, "the API key is not valid", nil)
			return
		}
		c.Set(contextKeyAuth, key)
		started := time.Now()
		c.Next()
		logWrite(c, key, time.Since(started))
	}
}

// logWrite records who changed what: every request that isn't a read, with
// the key's name, as MCP writes are recorded. Bodies aren't logged (images,
// notes); the path and status say what happened.
func logWrite(c *gin.Context, key *Key, took time.Duration) {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return
	}
	entry := logger.Log.WithFields(map[string]any{
		"key_id": key.ID, "key_name": key.Name, "method": c.Request.Method, "path": c.Request.URL.Path,
		"status": c.Writer.Status(), "took": took.String(),
	})
	if c.Writer.Status() >= http.StatusBadRequest {
		entry.Warn("API v1 write refused")
		return
	}
	entry.Info("API v1 write")
}

// requireScope rejects keys without the scope. Route groups declare their
// scope with it; handlers never check scopes themselves.
func requireScope(required scope.Scope) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, _ := c.Get(contextKeyAuth)
		k, ok := key.(*Key)
		if !ok || !k.Scopes.Allows(required) {
			abortWith(c, http.StatusForbidden, CodeInsufficientScope,
				"this API key does not have the "+string(required)+" scope",
				map[string]any{"required": string(required)})
			return
		}
		c.Next()
	}
}
