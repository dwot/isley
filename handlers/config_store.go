package handlers

import (
	"github.com/gin-gonic/gin"

	"isley/config"
)

// contextKeyConfigStore is the key under which the engine middleware
// stores the per-engine *config.Store.
const contextKeyConfigStore = "configStore"

// ConfigStoreFromContext extracts the *config.Store that the engine
// middleware injected into the Gin context. Mirrors DBFromContext and
// BackupServiceFromContext: handlers that need to read or mutate
// runtime configuration call this instead of touching package globals.
func ConfigStoreFromContext(c *gin.Context) *config.Store {
	return c.MustGet(contextKeyConfigStore).(*config.Store)
}

// SetConfigStoreOnContext is the small helper the engine middleware
// uses to bind a Store to a request context.
func SetConfigStoreOnContext(c *gin.Context, s *config.Store) {
	c.Set(contextKeyConfigStore, s)
}
