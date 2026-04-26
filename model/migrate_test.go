package model

// Phase 6a tests for migrate.go. The Postgres branches of MigrateDB and
// IsPostgresEmpty live behind the integration_postgres build tag and are
// covered separately by Phase 8. The tests here exercise:
//
//   - migrations apply cleanly to a fresh SQLite database
//   - re-applying migrations is idempotent (ErrNoChange)
//   - the resulting schema contains the expected tables and seed rows
//   - schema_migrations is populated with the highest migration version
//   - IsSQLite / IsPostgres / GetDriver match SetDriverForTesting
//   - DbPath honors ISLEY_DB_FILE and otherwise defaults to data/isley.db
//   - CloseDB is nil-safe
//   - BuildInClause emits the right placeholder shape per driver
//   - RunStartupMaintenance is a no-op under SQLite

import (
	"database/sql"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"isley/logger"
)

// ---------------------------------------------------------------------------
// Test-local fixtures
// ---------------------------------------------------------------------------

// initTestLogger silences the package-level logger used by InitDB,
// CloseDB, and RunStartupMaintenance so failed assertions are not buried
// under production-style log lines. Idempotent and process-wide.
var loggerOnce atomic.Bool

func initTestLogger() {
	if loggerOnce.Swap(true) {
		return
	}
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.PanicLevel)
	logger.Log = l
	logger.AccessWriter = io.Discard
}

// dbCounter makes each freshSQLite call use a unique shared-cache name so
// tests do not collide on the in-memory database when run in parallel.
var dbCounter atomic.Uint64

// freshSQLite returns an in-memory SQLite *sql.DB with no schema applied.
// The caller is responsible for applying migrations.
func freshSQLite(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:migrate-test-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", dbCounter.Add(1))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1) // keep the in-memory DB alive across the test
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	return db
}

// applySQLiteMigrations applies every migration under
// migrations/sqlite to db using golang-migrate. Mirrors what
// tests/testutil/db.go does, inlined here so model has no test-time
// dependency on testutil (which would create an import cycle).
func applySQLiteMigrations(t *testing.T, db *sql.DB) error {
	t.Helper()
	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
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
	return m.Up()
}

// userTablesT returns the names of all user tables in the database.
// (renamed from listSQLiteTables to avoid colliding with the same-named
// helper in model/sqlite_to_postgres.go.)
func userTablesT(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	return names
}

// ---------------------------------------------------------------------------
// Migration application
// ---------------------------------------------------------------------------

func TestApplyMigrations_FreshSQLite(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)

	err := applySQLiteMigrations(t, db)
	require.NoError(t, err, "first apply against a fresh DB must succeed")
}

func TestApplyMigrations_Idempotent(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)

	require.NoError(t, applySQLiteMigrations(t, db), "first Up")

	// Re-running migrations must be a no-op. golang-migrate signals this
	// with the sentinel ErrNoChange, which applySQLiteMigrations does NOT
	// special-case (so a second call returns it as the error).
	err := applySQLiteMigrations(t, db)
	assert.ErrorIs(t, err, migrate.ErrNoChange, "second apply should be a no-op")
}

func TestApplyMigrations_CreatesExpectedTables(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	got := userTablesT(t, db)
	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}

	// Core tables that handlers and watcher unconditionally rely on.
	// schema_migrations is golang-migrate's bookkeeping table.
	want := []string{
		"activity",
		"breeder",
		"metric",
		"plant",
		"plant_activity",
		"plant_images",
		"plant_measurements",
		"plant_status",
		"plant_status_log",
		"rolling_averages",
		"schema_migrations",
		"sensor_data",
		"sensor_data_hourly",
		"sensors",
		"settings",
		"strain",
		"strain_lineage",
		"streams",
		"zones",
	}
	for _, name := range want {
		assert.Truef(t, gotSet[name], "expected table %q after migrations; got tables: %v", name, sortedCopy(got))
	}
}

// TestApplyMigrations_PopulatesSchemaMigrationsVersion confirms the
// schema_migrations bookkeeping row reflects the highest applied
// migration. The version column equals the migration number (16 today,
// after 016_timezone). dirty must be false.
func TestApplyMigrations_PopulatesSchemaMigrationsVersion(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	var version int
	var dirty bool
	require.NoError(t, db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty))
	assert.GreaterOrEqual(t, version, 16, "schema_migrations.version should be at least 16 (the count of migration files)")
	assert.False(t, dirty, "a successful migration must leave dirty=false")
}

func TestApplyMigrations_SeedsBuiltInActivities(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	rows, err := db.Query(`SELECT name FROM activity ORDER BY name`)
	require.NoError(t, err)
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names[n] = true
	}
	for _, want := range []string{"Water", "Feed", "Note"} {
		assert.Truef(t, names[want], "default activity %q must be seeded", want)
	}
}

func TestApplyMigrations_SeedsLockedHeightMetric(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	var unit string
	var lock bool
	err := db.QueryRow(`SELECT unit, lock FROM metric WHERE name = 'Height'`).Scan(&unit, &lock)
	require.NoError(t, err, "Height metric must be present after migrations")
	assert.Equal(t, "in", unit)
	assert.True(t, lock, "Height is a built-in metric and must ship locked")
}

// ---------------------------------------------------------------------------
// Driver-introspection helpers
// ---------------------------------------------------------------------------

func TestDriverHelpers_RoundTripViaSetDriverForTesting(t *testing.T) {
	prev := GetDriver()
	t.Cleanup(func() { SetDriverForTesting(prev) })

	cases := []struct {
		name         string
		driver       string
		wantSQLite   bool
		wantPostgres bool
	}{
		{"sqlite", "sqlite", true, false},
		{"postgres", "postgres", false, true},
		{"empty", "", false, false},
		{"unknown", "mysql", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetDriverForTesting(tc.driver)
			assert.Equal(t, tc.driver, GetDriver())
			assert.Equal(t, tc.wantSQLite, IsSQLite())
			assert.Equal(t, tc.wantPostgres, IsPostgres())
		})
	}
}

// ---------------------------------------------------------------------------
// DbPath
// ---------------------------------------------------------------------------

func TestDbPath_DefaultsToDataIsleyDB(t *testing.T) {
	initTestLogger()
	t.Setenv("ISLEY_DB_FILE", "")
	got := DbPath()
	assert.Equal(t, "data/isley.db?_journal_mode=WAL", got)
}

func TestDbPath_HonorsEnvOverride(t *testing.T) {
	initTestLogger()
	t.Setenv("ISLEY_DB_FILE", "/tmp/elsewhere.db")
	got := DbPath()
	assert.Equal(t, "/tmp/elsewhere.db?_journal_mode=WAL", got)
}

// ---------------------------------------------------------------------------
// CloseDB / GetDB
// ---------------------------------------------------------------------------

// TestCloseDB_NilSafe verifies CloseDB does not panic when the package
// global db is nil — which is the state in any test process that has
// not called InitDB. Production calls CloseDB inside backup.go only
// after a successful InitDB, but defensive nil-check matters.
func TestCloseDB_NilSafe(t *testing.T) {
	initTestLogger()

	prev := db
	db = nil
	t.Cleanup(func() { db = prev })

	require.NoError(t, CloseDB())
}

// TestGetDB_ReturnsCurrentGlobal returns whatever is currently stored
// in the package-level db variable, with a nil error.
func TestGetDB_ReturnsCurrentGlobal(t *testing.T) {
	initTestLogger()

	prev := db
	t.Cleanup(func() { db = prev })

	// Sentinel: an open in-memory SQLite handle.
	tmp, err := sql.Open("sqlite", "file:getdb-test?mode=memory&cache=shared")
	require.NoError(t, err)
	defer tmp.Close()

	db = tmp
	got, err := GetDB()
	require.NoError(t, err)
	assert.Same(t, tmp, got)
}

// ---------------------------------------------------------------------------
// BuildInClause
// ---------------------------------------------------------------------------

func TestBuildInClause(t *testing.T) {
	cases := []struct {
		name        string
		driver      string
		items       []interface{}
		wantClause  string
		wantArgsLen int
	}{
		{"sqlite single", "sqlite", []interface{}{1}, "(?)", 1},
		{"sqlite multiple", "sqlite", []interface{}{1, 2, 3}, "(?, ?, ?)", 3},
		{"postgres single", "postgres", []interface{}{1}, "($1)", 1},
		{"postgres multiple", "postgres", []interface{}{"a", "b", "c"}, "($1, $2, $3)", 3},
		{"empty", "sqlite", []interface{}{}, "()", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := BuildInClause(tc.driver, tc.items)
			assert.Equal(t, tc.wantClause, clause)
			assert.Len(t, args, tc.wantArgsLen)
		})
	}
}

// ---------------------------------------------------------------------------
// RunStartupMaintenance
// ---------------------------------------------------------------------------

// TestRunStartupMaintenance_NoOpForSQLite confirms the maintenance
// routine returns immediately when the active driver is SQLite.
// REINDEX/REFRESH COLLATION VERSION are Postgres-only operations; the
// SQLite branch is a guard, and we lock it in here.
func TestRunStartupMaintenance_NoOpForSQLite(t *testing.T) {
	initTestLogger()

	prevDriver := GetDriver()
	prevDB := db
	t.Cleanup(func() {
		SetDriverForTesting(prevDriver)
		db = prevDB
	})

	SetDriverForTesting("sqlite")
	db = nil // would panic if the function did not return early under SQLite
	require.NotPanics(t, func() { RunStartupMaintenance() })
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// _ keeps strings imported in case future edits drop the only call site.
var _ = strings.ToLower
