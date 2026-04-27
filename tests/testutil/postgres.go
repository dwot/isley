//go:build integration_postgres

package testutil

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	"isley/model"
)

// PostgresEnv resolves the connection parameters used by the Postgres
// test harness. Defaults match the local docker-run example in
// docs/TEST_PLAN.md Phase 5 ("docker run -e POSTGRES_PASSWORD=test
// -p 5432:5432 -d postgres:16").
type PostgresEnv struct {
	Host     string
	Port     string
	User     string
	Password string
	AdminDB  string // database to connect to for CREATE/DROP DATABASE
}

// LoadPostgresEnv reads ISLEY_TEST_DB_* with sensible defaults.
func LoadPostgresEnv() PostgresEnv {
	return PostgresEnv{
		Host:     getenv("ISLEY_TEST_DB_HOST", "localhost"),
		Port:     getenv("ISLEY_TEST_DB_PORT", "5432"),
		User:     getenv("ISLEY_TEST_DB_USER", "postgres"),
		Password: getenv("ISLEY_TEST_DB_PASSWORD", "test"),
		AdminDB:  getenv("ISLEY_TEST_DB_ADMIN", "postgres"),
	}
}

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// dsn builds a libpq connection string for the given database name.
func (e PostgresEnv) dsn(database string) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		e.Host, e.Port, e.User, e.Password, database,
	)
}

// NewTestPostgresDB creates a fresh per-test PostgreSQL database, applies
// the Postgres migrations, and registers cleanup that drops the database
// when the test finishes.
//
// Each call returns a brand-new schema in its own database name, so tests
// can run in parallel against the same Postgres instance without colliding.
//
// The function calls model.SetDriverForTesting("postgres") so that
// dialect-aware production helpers (model.IsPostgres) report the correct
// dialect for the duration of the test run. Tests that mix this helper
// with NewTestDB in the same test binary should filter via `-run Postgres`
// to avoid racing the global driver state.
func NewTestPostgresDB(t *testing.T) *sql.DB {
	t.Helper()
	ensureProcessInitialized()

	env := LoadPostgresEnv()

	dbName := uniqueDBName(t)

	admin, err := sql.Open("postgres", env.dsn(env.AdminDB))
	if err != nil {
		t.Fatalf("NewTestPostgresDB: open admin: %v", err)
	}
	defer admin.Close()

	admin.SetConnMaxLifetime(time.Minute)
	if err := admin.Ping(); err != nil {
		// CI must fail loudly when Postgres is unreachable — the whole
		// point of the matrix is to catch dialect drift, and a silent
		// skip would green-light broken pipelines. Local developers who
		// opt into the build tag without bringing up Postgres get a
		// clearer skip with the docker-run hint.
		msg := fmt.Sprintf("NewTestPostgresDB: admin ping (%s:%s): %v", env.Host, env.Port, err)
		if os.Getenv("CI") != "" {
			t.Fatal(msg)
		}
		t.Skipf("%s — set ISLEY_TEST_DB_* or run: docker run -e POSTGRES_PASSWORD=test -p 5432:5432 -d postgres:16", msg)
	}

	// Defer driver-flag flip until we know Postgres is reachable. Setting
	// it earlier and then skipping leaves the global at "postgres" for
	// any later SQLite test in the same binary, which silently breaks
	// dialect-aware production helpers.
	model.SetDriverForTesting("postgres")

	if _, err := admin.Exec(fmt.Sprintf("CREATE DATABASE %s", quoteIdent(dbName))); err != nil {
		t.Fatalf("NewTestPostgresDB: create db %q: %v", dbName, err)
	}

	db, err := sql.Open("postgres", env.dsn(dbName))
	if err != nil {
		dropDB(admin, dbName)
		t.Fatalf("NewTestPostgresDB: open: %v", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		dropDB(admin, dbName)
		t.Fatalf("NewTestPostgresDB: ping: %v", err)
	}

	if err := applyPostgresMigrations(db); err != nil {
		_ = db.Close()
		dropDB(admin, dbName)
		t.Fatalf("NewTestPostgresDB: migrate: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		// Reopen admin in cleanup — the deferred Close above already ran.
		cleanup, err := sql.Open("postgres", env.dsn(env.AdminDB))
		if err != nil {
			return
		}
		defer cleanup.Close()
		dropDB(cleanup, dbName)
	})

	return db
}

func uniqueDBName(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("uniqueDBName: rand: %v", err)
	}
	return "isley_test_" + hex.EncodeToString(b[:])
}

// quoteIdent quotes a Postgres identifier per the libpq rules: wrap in
// double-quotes and double any embedded double-quote. Test database names
// come from crypto/rand hex output so this is defense-in-depth.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func dropDB(admin *sql.DB, name string) {
	// Force-disconnect any lingering connections, then drop. Postgres
	// rejects DROP DATABASE while there are open sessions on it; the
	// migrate library is the usual culprit.
	_, _ = admin.Exec(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
		name,
	)
	_, _ = admin.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", quoteIdent(name)))
}

func applyPostgresMigrations(db *sql.DB) error {
	src, err := iofs.New(model.MigrationsFS, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}

	drv, err := migratepg.WithInstance(db, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("postgres driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
	if err != nil {
		return fmt.Errorf("new migrate: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
