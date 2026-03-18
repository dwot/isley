package handlers

import (
	"database/sql"

	"github.com/gin-gonic/gin"
)

// contextKeyDB is the key under which the database middleware stores *sql.DB.
const contextKeyDB = "db"

// DBFromContext extracts the *sql.DB that the database middleware injected
// into the Gin context.  Every handler that receives a *gin.Context should
// call this instead of model.GetDB() so the connection is obtained once per
// request rather than once per function call.
//
// Usage:
//
//	func MyHandler(c *gin.Context) {
//	    db := handlers.DBFromContext(c)
//	    ...
//	}
func DBFromContext(c *gin.Context) *sql.DB {
	return c.MustGet(contextKeyDB).(*sql.DB)
}
