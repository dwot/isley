package testutil

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"isley/app"
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

	// BackupService is the per-engine service that owns backup/restore
	// status state. Tests can call its Set*InProgress methods to
	// deterministically exercise 409 branches without racing a real
	// goroutine.
	BackupService *handlers.BackupService
}

// ServerOption tunes NewTestServer. Today we only need GuestMode; the
// pattern is here so future options (clock, watcher start) drop in
// cleanly.
type ServerOption func(*serverOptions)

type serverOptions struct {
	guestMode     bool
	sessionSecret []byte
	dataDir       string
}

// WithGuestMode boots the engine with config.GuestMode == 1 semantics
// (basic routes are public).
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

	engine, err := app.NewEngine(app.Config{
		DB:             db,
		Assets:         os.DirFS(root),
		Version:        "isley-test",
		SessionSecret:  options.sessionSecret,
		SecureCookies:  false,
		GuestMode:      options.guestMode,
		TrustedProxies: nil,
		DataDir:        options.dataDir,
		BackupService:  backupSvc,
	})
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
		BackupService: backupSvc,
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
