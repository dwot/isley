// Package app builds the Gin engine for isley. It is consumed by both
// main (production) and tests/testutil (in-process tests). Construction
// is dependency-injected: nothing in this package reaches for env vars,
// model.GetDB(), or other process-global state — callers pass everything
// in via Config. Background services (watcher, grabber) are NOT started
// here; the caller owns goroutines and lifecycle.
package app

import (
	"database/sql"
	"io/fs"

	"isley/handlers"
)

// Config bundles everything NewEngine needs. Each caller (production
// main.go or the test harness) builds its own Config; nothing is read
// from environment variables or package globals inside NewEngine.
type Config struct {
	// DB is injected into the Gin context so handlers can resolve it via
	// handlers.DBFromContext(c). Required.
	DB *sql.DB

	// Assets is the file system that holds web/templates, web/static,
	// utils/fonts, and VERSION. In production this is the embedded FS
	// declared in main.go; in tests it is os.DirFS(repoRoot).
	Assets fs.FS

	// Version is the human-readable version string baked into templates
	// (e.g. "Isley 0.1.43-sqlite").
	Version string

	// SessionSecret keys the gorilla/sessions cookie store.
	// Required and must be at least 32 bytes for production; tests can
	// use a short fixed value.
	SessionSecret []byte

	// SecureCookies sets the Secure flag on session cookies. Production
	// reads ISLEY_SECURE_COOKIES; tests default to false.
	SecureCookies bool

	// GuestMode mirrors config.GuestMode == 1: when true, Basic routes
	// are added to the public group instead of the protected group.
	GuestMode bool

	// TrustedProxies is forwarded to Gin's SetTrustedProxies so that
	// c.ClientIP() returns the real client IP behind a reverse proxy.
	// Tests typically pass nil to disable trust entirely.
	TrustedProxies []string

	// DataDir is the root directory where backup archives, scratch files,
	// and other on-disk state live. Defaults to "data" when empty.
	// Production passes "data" (or whatever the deploy uses); tests pass
	// t.TempDir() so each test has its own isolated tree.
	DataDir string

	// BackupService, if non-nil, is wired into the engine instead of
	// having NewEngine construct one from DB+DataDir. Used by tests that
	// want a handle to the service so they can deterministically flip
	// in-progress state without racing a real goroutine. Production
	// leaves this nil and lets NewEngine build the service.
	BackupService *handlers.BackupService
}
