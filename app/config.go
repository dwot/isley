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

	"isley/config"
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

	// GuestMode mirrors the Store's GuestMode == 1: when true, Basic
	// routes are added to the public group instead of the protected
	// group. Production reads it from the Store after LoadSettings runs;
	// tests pass it explicitly.
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

	// UploadDir is the root directory where user-uploaded plant images
	// and logo files are written. Defaults to "uploads" when empty.
	// Tests pass t.TempDir() (via testutil.WithUploadDir) so per-test
	// upload writes do not pollute the repo's uploads/ tree and parallel
	// tests cannot collide on the same on-disk paths.
	UploadDir string

	// StreamDir is the root directory where stream snapshot images are
	// written by AddStreamHandler's initial GrabWebcamImage call. Defaults
	// to filepath.Join(UploadDir, "streams") when empty. Tests pass
	// t.TempDir() (via testutil.WithStreamDir) so per-test stream writes
	// stay isolated.
	StreamDir string

	// FrameDir is the root directory where the watcher's grabber writes
	// stream frame snapshots. Production currently shares this with
	// StreamDir (both write to uploads/streams/<file>); the field is kept
	// distinct so tests for the grabber can pass an isolated tempdir
	// without affecting handler-side stream writes. Defaults to the same
	// value as StreamDir when empty. NewEngine itself does not consume
	// FrameDir — it is read by the watcher goroutine and surfaced here so
	// production main can thread one configuration value through both
	// subsystems.
	FrameDir string

	// LogsDir is the directory the GetLogs / DownloadLogs handlers read
	// app.log and access.log from. Defaults to "logs" when empty.
	// Tests pass t.TempDir() (via testutil.WithLogsDir) so log-file
	// tests can write fixture logs without touching the repo's logs/.
	LogsDir string

	// BackupService, if non-nil, is wired into the engine instead of
	// having NewEngine construct one from DB+DataDir. Used by tests that
	// want a handle to the service so they can deterministically flip
	// in-progress state without racing a real goroutine. Production
	// leaves this nil and lets NewEngine build the service.
	BackupService *handlers.BackupService

	// ConfigStore, if non-nil, is the per-engine runtime configuration
	// store handlers read at request time. Tests construct one and
	// pre-populate the few fields they care about (e.g. APIIngestEnabled,
	// ACIToken) so the body of the test stays focused. Production leaves
	// this nil and NewEngine constructs a default; main.go is then
	// responsible for calling handlers.LoadSettings to populate it from
	// the DB before traffic arrives.
	ConfigStore *config.Store

	// RateLimiterService, if non-nil, is the per-engine service that
	// owns the ingest and login rate limiters. Tests pass an instance
	// with a tightened or relaxed policy when they need to deterministically
	// drive the 429 branch (or sidestep it). Production leaves this nil
	// and NewEngine constructs a default service with the documented
	// limits (60/min ingest, MaxLoginAttempts/min login).
	RateLimiterService *handlers.RateLimiterService

	// SensorCacheService, if non-nil, is the per-engine service that
	// owns the grouped-sensor and per-query LRU chart caches. Tests
	// pass a fresh instance so each test starts from an empty cache
	// and parallel tests cannot clobber each other's cached responses.
	// Production leaves this nil and NewEngine constructs a default
	// service whose grouped TTL closure reads from the engine's
	// *config.Store.
	SensorCacheService *handlers.SensorCacheService
}
