package testutil

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"isley/app"
	"isley/config"
	"isley/handlers"
)

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
}

// ServerOption tunes NewTestServer. Today we only need GuestMode; the
// pattern is here so future options (clock, watcher start) drop in
// cleanly.
type ServerOption func(*serverOptions)

type serverOptions struct {
	guestMode     bool
	sessionSecret []byte
	dataDir       string
	uploadDir     string
	streamDir     string
	logsDir       string
	frameDir      string
	configStore   *config.Store
}

// WithGuestMode boots the engine with guest-mode semantics
// (basic routes are public, mirroring the Store's GuestMode == 1).
func WithGuestMode() ServerOption {
	return func(o *serverOptions) { o.guestMode = true }
}

// WithSessionSecret overrides the default 32-byte test session secret.
// Useful for verifying cookie cross-instance behavior.
func WithSessionSecret(secret []byte) ServerOption {
	return func(o *serverOptions) { o.sessionSecret = secret }
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

// WithFrameDir overrides the configured frame directory (where the
// watcher's grabber writes stream frame snapshots in production). The
// engine itself does not write to this directory — it is recorded on
// the resulting TestServer so tests that exercise the watcher's Grab
// function can read it from one place rather than threading a literal.
func WithFrameDir(dir string) ServerOption {
	return func(o *serverOptions) { o.frameDir = dir }
}

// WithConfigStore overrides the per-engine *config.Store. Tests that
// need a code path conditional on runtime config (e.g. ACIToken set,
// APIIngestEnabled = 1) construct a Store, call its Set* methods, and
// pass it here so the handler reads the desired value without mutating
// any process-global. Pass nil to use a default-constructed Store.
func WithConfigStore(s *config.Store) ServerOption {
	return func(o *serverOptions) { o.configStore = s }
}

// NewTestServer constructs a Gin engine wired to db and serves it via
// httptest.NewServer. The server is shut down automatically when the
// test finishes. Background services (watcher, grabber) are NOT started;
// the smoke test does not need them and starting them would couple the
// test to global config state.
func NewTestServer(t *testing.T, db *sql.DB, opts ...ServerOption) *TestServer {
	t.Helper()
	ensureProcessInitialized()

	options := serverOptions{
		// Deterministic 32-byte secret keeps cookies stable across
		// tests in the same process. Tests that care about rotation
		// pass WithSessionSecret.
		sessionSecret: []byte("isley-test-session-secret-32-by!"),
	}
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
	}

	engineCfg := app.Config{
		DB:             db,
		Assets:         os.DirFS(root),
		Version:        "isley-test",
		SessionSecret:  options.sessionSecret,
		SecureCookies:  false,
		GuestMode:      options.guestMode,
		TrustedProxies: nil,
		DataDir:        options.dataDir,
		UploadDir:      options.uploadDir,
		StreamDir:      options.streamDir,
		FrameDir:       options.frameDir,
		LogsDir:        options.logsDir,
		BackupService:  backupSvc,
		ConfigStore:    configStore,
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
		Server:        srv,
		DB:            db,
		Assets:        root,
		DataDir:       options.dataDir,
		UploadDir:     resolved.UploadDir,
		StreamDir:     resolved.StreamDir,
		LogsDir:       resolved.LogsDir,
		FrameDir:      resolved.FrameDir,
		BackupService: backupSvc,
		ConfigStore:   configStore,
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
