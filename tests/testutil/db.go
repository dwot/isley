// Package testutil provides shared fixtures for the new test harness
// described in docs/TEST_PLAN.md.
//
// The harness is constructor-only: every helper takes a *testing.T,
// returns fresh state, and registers any cleanup it needs. Tests opt in
// to whatever they require (DB, server, fakes) per test, with no
// init-time side effects and no globals shared between tests. That is
// the property that lets `go test -race -parallel` work.
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"

	"isley/model"
)

// dbSeq makes each NewTestDB call use a unique shared-cache name so that
// parallel tests do not collide. SQLite's `cache=shared` mode treats two
// connections to the same name as the same database; using a per-call
// counter guarantees isolation between tests while still letting the
// migrate library and the application share state inside a single test.
var dbSeq atomic.Uint64

// NewTestDB returns a fresh in-memory SQLite database with all SQLite
// migrations applied. Each call returns a brand-new schema; the database
// is closed automatically when the test finishes.
//
// Safe to call from inside t.Parallel().
func NewTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ensureProcessInitialized()

	// Unique per call so concurrent tests do not see each other's tables
	// even though they use the shared-cache backend.
	name := fmt.Sprintf("isley-test-%d-%d", testPID(), dbSeq.Add(1))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(1)", name)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("NewTestDB: open: %v", err)
	}

	// Per audit #13 / TEST_PLAN Phase 4: do not cap MaxOpenConns. The
	// shared-cache DSN keeps the in-memory database alive across
	// connections, and a single-conn cap deadlocks any code path that
	// pins one connection (e.g. handlers/backup.go runRestore) while
	// another part of the same goroutine, or the request driving it,
	// needs a second connection from the pool.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("NewTestDB: ping: %v", err)
	}

	if err := restoreTemplate(db); err != nil {
		_ = db.Close()
		t.Fatalf("NewTestDB: migrate: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

// MustExec runs db.Exec(query, args...) and fails the test if it errors.
// Use it for seed setup where an error is a test-author bug, not an
// assertion target. The discarded sql.Result is intentional — callers
// that need LastInsertId should use one of the Seed* helpers (which
// return ids) or call db.Exec directly.
func MustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoErrorf(t, err, "MustExec: %s", query)
}

// The migrated schema is built once per test process and copied into
// each new database with SQLite's backup API. Running every migration
// for every test cost ~300ms of CPU apiece under -race and grew with
// each migration; the copy is a page copy of a small database.
var (
	templateOnce sync.Once
	templateDSN  string
	templateErr  error
	// templateMu serialises reads of the template so concurrent restores
	// never contend for its shared-cache locks.
	templateMu sync.Mutex
)

func migratedTemplate() (string, error) {
	templateOnce.Do(func() {
		templateDSN = fmt.Sprintf("file:isley-test-template-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", testPID())
		db, err := sql.Open("sqlite", templateDSN)
		if err != nil {
			templateErr = err
			return
		}
		// Never closed: the open handle keeps the shared in-memory
		// template alive for the life of the process.
		db.SetMaxIdleConns(1)
		if err := applyMigrations(db); err != nil {
			templateErr = err
			_ = db.Close()
		}
	})
	return templateDSN, templateErr
}

// restoreTemplate copies the migrated template into db.
func restoreTemplate(db *sql.DB) error {
	dsn, err := migratedTemplate()
	if err != nil {
		return err
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	templateMu.Lock()
	defer templateMu.Unlock()
	return conn.Raw(func(dc any) error {
		r, ok := dc.(interface {
			NewRestore(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("driver connection %T cannot restore", dc)
		}
		b, err := r.NewRestore(dsn)
		if err != nil {
			return err
		}
		for {
			more, err := b.Step(-1)
			if err != nil {
				_ = b.Finish()
				return err
			}
			if !more {
				break
			}
		}
		return b.Finish()
	})
}

func applyMigrations(db *sql.DB) error {
	src, err := iofs.New(model.MigrationsFS, "migrations/sqlite")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}

	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("sqlite driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("new migrate: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
