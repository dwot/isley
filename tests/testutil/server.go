package testutil

import (
	"database/sql"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"isley/app"
	"isley/chrono"
	"isley/config"
	"isley/handlers"
	"isley/ratelimit"
	"isley/service"
)

// RepoFS returns an fs.FS rooted at the repository's go.mod directory,
// suitable for app.Config.Assets in tests that construct an engine
// directly (the integration suite reaches the same FS via NewTestServer).
// Fails the test if the repo root cannot be located.
func RepoFS(t *testing.T) fs.FS {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("RepoFS: locate repo root: %v", err)
	}
	return os.DirFS(root)
}

// TestServer wraps an httptest.Server backed by an in-process Gin engine
// constructed via app.NewEngine. Each test gets its own server, its own
// cookie jar, and (typically) its own NewTestDB.
type TestServer struct {
	*httptest.Server

	DB      *sql.DB
	Assets  string // absolute path to the repo root used as the assets fs
	DataDir string // root data directory used by the engine (e.g. for backups)

	// UploadDir, StreamDir, LogsDir, FrameDir mirror the dirs the engine
	// was constructed with so tests that need to assert against
	// disk-side artefacts (e.g. "the upload landed under <UploadDir>/
	// plants/...") can read the same path the handler wrote to.
	// FrameDir is included for symmetry even though the test server does
	// not run the watcher's grabber; tests that exercise the grabber
	// directly read this field to know which directory the engine was
	// built around.
	UploadDir string
	StreamDir string
	LogsDir   string
	FrameDir  string

	// BackupService is the per-engine service that owns backup/restore
	// status state. Tests can call its Set*InProgress methods to
	// deterministically exercise 409 branches without racing a real
	// goroutine.
	BackupService *handlers.BackupService

	// ConfigStore is the per-engine *config.Store that handlers read at
	// request time. Tests pre-populate fields via WithConfigStore (or
	// access this handle directly after construction) instead of mutating
	// process-global config.
	ConfigStore *config.Store

	// SensorCacheService is the per-engine *handlers.SensorCacheService
	// that owns the chart and grouped-sensor caches. Tests reach for
	// this handle when they need to assert against cache state directly
	// (e.g. force-eviction in mid-test) rather than going through the
	// HTTP layer; the per-engine scope means each parallel test sees
	// its own buckets.
	SensorCacheService *handlers.SensorCacheService

	// Engine is the Gin engine the server wraps, for tests that inspect
	// its routes.
	Engine *gin.Engine
}

// ServerOption tunes NewTestServer. Today we only need GuestMode; the
// pattern is here so future options (clock, watcher start) drop in
// cleanly.
type ServerOption func(*serverOptions)

type serverOptions struct {
	guestMode          bool
	dataDir            string
	uploadDir          string
	streamDir          string
	logsDir            string
	configStore        *config.Store
	rateLimiterService *ratelimit.Service
	clock              chrono.Clock
	dialect            service.Dialect
}

// WithClock gives the engine's service layer (and /api/v1) a clock, e.g.
// chrono.Fixed, so "now", "today" and plant ages are deterministic.
func WithClock(c chrono.Clock) ServerOption {
	return func(o *serverOptions) { o.clock = c }
}

// WithDialect tells the engine which database db is. Tests over
// NewTestPostgresDB pass service.Postgres; the default follows the model
// package's driver setting.
func WithDialect(d service.Dialect) ServerOption {
	return func(o *serverOptions) { o.dialect = d }
}

// WithGuestMode boots the engine with guest-mode semantics
// (basic routes are public, mirroring the Store's GuestMode == 1).
func WithGuestMode() ServerOption {
	return func(o *serverOptions) { o.guestMode = true }
}

// WithDataDir overrides the engine's data directory (where backup
// archives and other on-disk state live). Tests typically pass
// t.TempDir() so each run has an isolated tree and parallel tests
// cannot collide on backups/uploads/scratch paths.
func WithDataDir(dir string) ServerOption {
	return func(o *serverOptions) { o.dataDir = dir }
}

// WithUploadDir overrides the engine's upload root (where plant images
// and uploaded logos are written). Tests typically pass t.TempDir()
// instead of t.Chdir to scope upload writes to an isolated tree
// without mutating the process-global working directory — which is
// what unblocks t.Parallel() on these tests.
func WithUploadDir(dir string) ServerOption {
	return func(o *serverOptions) { o.uploadDir = dir }
}

// WithStreamDir overrides the engine's stream-snapshot root (where
// AddStreamHandler's initial GrabWebcamImage call writes the first
// frame). Defaults to filepath.Join(UploadDir, "streams") when unset
// — matching the engine's ResolvePathDefaults behavior.
func WithStreamDir(dir string) ServerOption {
	return func(o *serverOptions) { o.streamDir = dir }
}

// WithLogsDir overrides the engine's logs directory (where GetLogs and
// DownloadLogs read app.log / access.log from). Tests writing log
// fixtures pass t.TempDir() so the fixture lands in an isolated tree
// and tests do not need to t.Chdir.
func WithLogsDir(dir string) ServerOption {
	return func(o *serverOptions) { o.logsDir = dir }
}

// WithConfigStore overrides the per-engine *config.Store. Tests that
// need a code path conditional on runtime config (e.g. ACIToken set,
// APIIngestEnabled = 1) construct a Store, call its Set* methods, and
// pass it here so the handler reads the desired value without mutating
// any process-global. Pass nil to use a default-constructed Store.
func WithConfigStore(s *config.Store) ServerOption {
	return func(o *serverOptions) { o.configStore = s }
}

// WithRateLimiterService overrides the per-engine
// *ratelimit.Service. Tests that want to drive the 429
// branch on /login or /api/sensors/ingest construct a service with a
// tightened policy and pass it here; tests that want to sidestep the
// limiter entirely construct one with a generous limit. Pass nil to
// use the default service (60/min ingest, MaxLoginAttempts/min login).
// Each test gets its own service instance, which is what makes
// t.Parallel() safe.
func WithRateLimiterService(s *ratelimit.Service) ServerOption {
	return func(o *serverOptions) { o.rateLimiterService = s }
}

// NewTestServer constructs a Gin engine wired to db and serves it via
// httptest.NewServer. The server is shut down automatically when the
// test finishes. Background services (watcher, grabber) are NOT started;
// the smoke test does not need them and starting them would couple the
// test to global config state.
func NewTestServer(t *testing.T, db *sql.DB, opts ...ServerOption) *TestServer {
	t.Helper()
	ensureProcessInitialized()

	var options serverOptions
	for _, opt := range opts {
		opt(&options)
	}

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("NewTestServer: locate repo root: %v", err)
	}

	backupSvc := handlers.NewBackupService(db, options.dataDir)

	configStore := options.configStore
	if configStore == nil {
		configStore = config.NewStore()
		// A fixed instance zone, so stored times read the same on any
		// machine (CI runs in UTC, a laptop may not).
		configStore.SetTimezone("UTC")
	}

	rateLimiterSvc := options.rateLimiterService
	if rateLimiterSvc == nil {
		rateLimiterSvc = ratelimit.NewService(nil, nil)
	}

	// Mirror the engine's production TTL closure so tests see the same
	// staleness rules — the closure reads PollingInterval off the
	// per-test Store, so a test that pins PollingInterval via
	// WithConfigStore controls the TTL exactly.
	store := configStore
	sensorCacheSvc := handlers.NewSensorCacheService(func() time.Duration {
		interval := handlers.DefaultPollingIntervalSeconds
		if store != nil {
			interval = store.PollingInterval()
		}
		if interval <= 0 {
			interval = handlers.DefaultPollingIntervalSeconds
		}
		return time.Duration(interval/10) * time.Second
	}, 0)

	engineCfg := app.Config{
		DB:      db,
		Assets:  os.DirFS(root),
		Version: "isley-test",
		// A fixed secret keeps cookies stable across tests in a process.
		SessionSecret:      []byte("isley-test-session-secret-32-by!"),
		SecureCookies:      false,
		GuestMode:          options.guestMode,
		TrustedProxies:     nil,
		DataDir:            options.dataDir,
		UploadDir:          options.uploadDir,
		StreamDir:          options.streamDir,
		LogsDir:            options.logsDir,
		BackupService:      backupSvc,
		ConfigStore:        configStore,
		RateLimiterService: rateLimiterSvc,
		SensorCacheService: sensorCacheSvc,
		Clock:              options.clock,
		Dialect:            options.dialect,
	}
	// Resolve defaults so the TestServer's exported path fields reflect
	// the same values the engine middleware injects into request context.
	resolved := app.ResolvePathDefaults(engineCfg)

	engine, err := app.NewEngine(engineCfg)
	if err != nil {
		t.Fatalf("NewTestServer: app.NewEngine: %v", err)
	}

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	return &TestServer{
		Server:             srv,
		DB:                 db,
		Assets:             root,
		DataDir:            options.dataDir,
		UploadDir:          resolved.UploadDir,
		StreamDir:          resolved.StreamDir,
		LogsDir:            resolved.LogsDir,
		FrameDir:           resolved.FrameDir,
		BackupService:      backupSvc,
		ConfigStore:        configStore,
		SensorCacheService: sensorCacheSvc,
		Engine:             engine,
	}
}

// repoRoot walks up from this source file until it finds the directory
// containing go.mod, then returns its absolute path. Tests rely on this
// to make os.DirFS point at the live web/ and utils/ trees rather than
// requiring a relative-path convention.
func repoRoot() (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	dir := filepath.Dir(here)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
