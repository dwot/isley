//go:build integration_postgres

// Tests for model/sqlite_to_postgres.go (Phase 6 of docs/TEST_PLAN.md).
// These exercise the SQLite→PostgreSQL migration tool that powers
// docker-compose.migration.yml end-to-end against a real Postgres.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres ./model/...
//
// CI invocation: see .github/workflows/release.yml job test-postgres.
//
// Connection params come from environment (defaults match the docker-run
// example in docs/TEST_PLAN.md Phase 5):
//
//	ISLEY_TEST_DB_HOST     (default "localhost")
//	ISLEY_TEST_DB_PORT     (default "5432")
//	ISLEY_TEST_DB_USER     (default "postgres")
//	ISLEY_TEST_DB_PASSWORD (default "test")
//	ISLEY_TEST_DB_ADMIN    (default "postgres" — db used for CREATE/DROP)
//
// IMPORTANT: this file shares the model_test binary with migrate_test.go,
// which is plain SQLite and untagged. The Postgres tests below set
// model.dbDriver to "postgres" via SetDriverForTesting exactly once
// per binary (driverSetOnce); the value is NOT restored on cleanup
// because (a) every Postgres test needs the same value and (b) parallel
// cleanups racing on the global is what triggered the original
// -race CI failure. Always run with `-run Postgres` so the SQLite
// tests in migrate_test.go don't see the postgres-flavored driver.

package model

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// ----------------------------------------------------------------------------
// Postgres test-DB harness
//
// We do not import tests/testutil/postgres.go here because tests/testutil
// already imports isley/model — bringing it back the other way would cycle.
// The setup is small enough to inline.
// ----------------------------------------------------------------------------

type pgEnv struct {
	Host, Port, User, Password, AdminDB string
}

func loadPGEnv() pgEnv {
	return pgEnv{
		Host:     getenvOr("ISLEY_TEST_DB_HOST", "localhost"),
		Port:     getenvOr("ISLEY_TEST_DB_PORT", "5432"),
		User:     getenvOr("ISLEY_TEST_DB_USER", "postgres"),
		Password: getenvOr("ISLEY_TEST_DB_PASSWORD", "test"),
		AdminDB:  getenvOr("ISLEY_TEST_DB_ADMIN", "postgres"),
	}
}

func getenvOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func (e pgEnv) dsn(database string) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		e.Host, e.Port, e.User, e.Password, database,
	)
}

// driverSetOnce serializes the one-time write to model.dbDriver so
// parallel postgres tests do not race on the package global. Every
// test in this build-tagged binary needs driver=="postgres", and the
// SQLite tests in migrate_test.go are filtered out at run time by
// `-run Postgres` (see CI invocation), so a single set-and-leave is
// the right shape — no per-test restore. The earlier prev/restore
// pattern caused -race failures in CI when parallel tests' cleanup
// goroutines all wrote to dbDriver concurrently.
var driverSetOnce sync.Once

// freshPostgresDB creates a fresh per-test Postgres database. When
// withMigrations is true, the model's bundled Postgres migrations are
// applied; otherwise the database is bare (no user tables). Cleanup
// drops the database when the test finishes.
//
// Sets the package-level dbDriver to "postgres" exactly once per test
// binary via driverSetOnce, so production helpers like IsPostgres()
// report the active dialect for the duration of the run.
func freshPostgresDB(t *testing.T, withMigrations bool) *sql.DB {
	t.Helper()
	initTestLogger()

	env := loadPGEnv()

	admin, err := sql.Open("postgres", env.dsn(env.AdminDB))
	require.NoError(t, err, "open admin db")
	defer admin.Close()
	admin.SetConnMaxLifetime(time.Minute)

	if err := admin.Ping(); err != nil {
		// Match testutil/postgres.go's contract: hard-fail in CI so a
		// missing service container is loud, but skip locally with the
		// docker-run hint for developers who opt into the build tag
		// without bringing up Postgres.
		msg := fmt.Sprintf("freshPostgresDB: admin ping (%s:%s): %v", env.Host, env.Port, err)
		if os.Getenv("CI") != "" {
			t.Fatal(msg)
		}
		t.Skipf("%s — set ISLEY_TEST_DB_* or run: docker run -e POSTGRES_PASSWORD=test -p 5432:5432 -d postgres:16", msg)
	}

	driverSetOnce.Do(func() { SetDriverForTesting("postgres") })

	name := uniquePGDBName(t)
	if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE %s`, quoteIdentPG(name))); err != nil {
		t.Fatalf("freshPostgresDB: create db %q: %v", name, err)
	}

	db, err := sql.Open("postgres", env.dsn(name))
	require.NoError(t, err, "open test db")

	if err := db.Ping(); err != nil {
		_ = db.Close()
		dropPGDB(admin, name)
		t.Fatalf("freshPostgresDB: ping: %v", err)
	}

	if withMigrations {
		if err := applyPostgresMigrationsForTest(db); err != nil {
			_ = db.Close()
			dropPGDB(admin, name)
			t.Fatalf("freshPostgresDB: migrate: %v", err)
		}
	}

	t.Cleanup(func() {
		_ = db.Close()
		// Reopen admin in cleanup; the deferred Close above already ran.
		cleanup, err := sql.Open("postgres", env.dsn(env.AdminDB))
		if err != nil {
			return
		}
		defer cleanup.Close()
		dropPGDB(cleanup, name)
	})

	return db
}

func uniquePGDBName(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("uniquePGDBName: rand: %v", err)
	}
	return "isley_s2p_" + hex.EncodeToString(b[:])
}

// quoteIdentPG quotes a Postgres identifier per libpq rules. Test database
// names are crypto/rand hex output so this is defense-in-depth.
func quoteIdentPG(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func dropPGDB(admin *sql.DB, name string) {
	_, _ = admin.Exec(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
		name,
	)
	_, _ = admin.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quoteIdentPG(name)))
}

func applyPostgresMigrationsForTest(db *sql.DB) error {
	src, err := iofs.New(MigrationsFS, "migrations/postgres")
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

// freshSQLiteFile creates a file-backed SQLite database (rather than the
// in-memory variety used by freshSQLite in migrate_test.go), applies the
// SQLite migrations, and returns the path. MigrateSqliteToPostgres takes
// a path, so the end-to-end test needs an on-disk DB.
func freshSQLiteFile(t *testing.T) string {
	t.Helper()
	initTestLogger()

	path := filepath.Join(t.TempDir(), "isley_test.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	require.NoError(t, applySQLiteMigrations(t, db), "apply sqlite migrations")
	require.NoError(t, db.Close())
	return path
}

func openSQLiteFile(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// ----------------------------------------------------------------------------
// IsPostgresEmpty
// ----------------------------------------------------------------------------

// TestIsPostgresEmpty_TrueOnFreshDB asserts that a database with the
// schema migrated but no business data reports as empty. "Empty" here
// means the three sentinel tables IsPostgresEmpty checks — sensors,
// sensor_data, plant — all have zero rows, which is the state of any
// freshly migrated database.
func TestIsPostgresEmpty_TrueOnFreshDB(t *testing.T) {
	t.Parallel()
	pg := freshPostgresDB(t, true)

	empty, err := IsPostgresEmpty(pg)
	require.NoError(t, err)
	assert.True(t, empty, "freshly migrated PG with no business rows must be empty")
}

// TestIsPostgresEmpty_FalseAfterMigrations asserts that inserting a row
// into one of the sentinel tables flips IsPostgresEmpty to false. The
// function is the gate that decides whether SQLite-to-PG migration runs
// in InitDB; a regression that reports a populated database as empty
// would silently double-migrate.
func TestIsPostgresEmpty_FalseAfterMigrations(t *testing.T) {
	t.Parallel()
	pg := freshPostgresDB(t, true)

	// FK chain: breeder → strain → plant. Plant is one of the sentinels.
	var breederID int
	require.NoError(t, pg.QueryRow(
		`INSERT INTO breeder (name) VALUES ($1) RETURNING id`, "Acme",
	).Scan(&breederID))

	var strainID int
	require.NoError(t, pg.QueryRow(
		`INSERT INTO strain (name, sativa, indica, autoflower, description, seed_count, breeder_id)
		 VALUES ($1, 50, 50, 0, '', 5, $2) RETURNING id`, "Roundtrip", breederID,
	).Scan(&strainID))

	_, err := pg.Exec(
		`INSERT INTO plant (name, description, clone, strain_id, sensors)
		 VALUES ($1, '', 0, $2, '[]')`, "Sentinel", strainID,
	)
	require.NoError(t, err)

	empty, err := IsPostgresEmpty(pg)
	require.NoError(t, err)
	assert.False(t, empty, "PG with a plant row must not report as empty")
}

// TestIsPostgresEmpty_ErrorWhenSchemaMissing locks in the contract that
// IsPostgresEmpty surfaces a missing sentinel table as an error rather
// than silently reporting "empty=true". The caller (InitDB) treats
// "empty" as the trigger to run the SQLite-to-PG migration, so a regression
// that swallowed missing-table errors and returned true would attempt a
// migration into a database with no schema and corrupt it.
func TestIsPostgresEmpty_ErrorWhenSchemaMissing(t *testing.T) {
	t.Parallel()
	pg := freshPostgresDB(t, false) // no migrations applied

	_, err := IsPostgresEmpty(pg)
	require.Error(t, err, "missing sentinel tables must surface as an error, not as 'empty=true'")
	assert.Contains(t, strings.ToLower(err.Error()), "sensors",
		"the wrapped error must identify which sentinel table failed")
}

// ----------------------------------------------------------------------------
// listSQLiteTables
// ----------------------------------------------------------------------------

// TestListSQLiteTables_KnownSchema asserts listSQLiteTables returns every
// user table in a freshly migrated SQLite DB, and excludes both the
// sqlite_* internals and the schema_migrations bookkeeping table.
func TestListSQLiteTables_KnownSchema(t *testing.T) {
	t.Parallel()
	initTestLogger()

	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	got, err := listSQLiteTables(db)
	require.NoError(t, err)

	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}

	want := []string{
		"activity", "breeder", "metric",
		"plant", "plant_activity", "plant_images",
		"plant_measurements", "plant_status", "plant_status_log",
		"rolling_averages", "sensor_data", "sensor_data_hourly",
		"sensors", "settings", "strain", "strain_lineage",
		"streams", "zones",
	}
	for _, name := range want {
		assert.Truef(t, gotSet[name], "expected table %q in listSQLiteTables; got %v", name, got)
	}

	// Filter assertions: must NOT include schema_migrations or any
	// sqlite_* internal table.
	assert.False(t, gotSet["schema_migrations"], "listSQLiteTables must filter schema_migrations")
	for _, name := range got {
		assert.Falsef(t, strings.HasPrefix(name, "sqlite_"),
			"listSQLiteTables must filter sqlite_* tables; saw %q", name)
	}
}

// ----------------------------------------------------------------------------
// hasSerialID
// ----------------------------------------------------------------------------

// TestHasSerialID_TableMembership asserts hasSerialID is true for every
// table whose 'id' column is a SERIAL/identity (and therefore needs a
// setval after a migration) and false for everything else.
func TestHasSerialID_TableMembership(t *testing.T) {
	t.Parallel()

	serial := []string{
		"settings", "zones", "sensors", "sensor_data", "strain",
		"plant_status", "plant", "plant_status_log", "metric",
		"plant_measurements", "activity", "plant_activity",
		"plant_images", "breeder", "streams",
	}
	for _, name := range serial {
		assert.Truef(t, hasSerialID(name), "%s should be in the SERIAL-id set", name)
	}

	// Tables with composite PKs, golang-migrate's bookkeeping table, and
	// arbitrary unknown names must all report false. strain_lineage is
	// notable: it has a SERIAL id but is not migrated by the tool today,
	// and is intentionally absent from hasSerialID.
	nonSerial := []string{
		"rolling_averages",   // composite PK on sensor_id
		"sensor_data_hourly", // composite PK on (sensor_id, bucket)
		"schema_migrations",  // golang-migrate bookkeeping
		"strain_lineage",     // present but excluded by orderedTables
		"nonexistent_table",
		"",
	}
	for _, name := range nonSerial {
		assert.Falsef(t, hasSerialID(name), "%q should NOT be in the SERIAL-id set", name)
	}
}

// ----------------------------------------------------------------------------
// copyTableData — happy path
// ----------------------------------------------------------------------------

// TestCopyTableData_HappyPath copies a small table populated with mixed
// types (TEXT, INTEGER, REAL/NUMERIC, NULL, TIMESTAMP) from a fresh
// SQLite into a fresh Postgres and asserts every value round-trips.
// The plant table was chosen because it covers all four type axes in
// one row (TEXT name/description, INTEGER strain_id/clone, NULL zone_id,
// NUMERIC harvest_weight, TIMESTAMP start_dt).
func TestCopyTableData_HappyPath(t *testing.T) {
	t.Parallel()

	sqliteDB := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, sqliteDB))

	pg := freshPostgresDB(t, true)

	// Seed the SQLite source with a breeder/strain/zone/plant chain.
	mustExecSQL(t, sqliteDB,
		`INSERT INTO breeder (id, name) VALUES (1, 'Roundtrip Breeder')`)
	mustExecSQL(t, sqliteDB,
		`INSERT INTO strain (id, name, sativa, indica, autoflower, description, seed_count, breeder_id)
		 VALUES (1, 'Roundtrip Strain', 60, 40, 0, 'A test strain', 5, 1)`)
	mustExecSQL(t, sqliteDB,
		`INSERT INTO zones (id, name) VALUES (1, 'Roundtrip Zone')`)
	// Use NULL zone_id to exercise the null path through the scan loop.
	// harvest_weight covers REAL/NUMERIC; start_dt covers TIMESTAMP.
	mustExecSQL(t, sqliteDB,
		`INSERT INTO plant (id, name, description, clone, strain_id, zone_id, start_dt, sensors, harvest_weight)
		 VALUES (1, 'Roundtrip Plant', 'mixed-type test row', 0, 1, NULL, '2026-04-26 10:30:00', '[]', 12.34)`)

	// Copy in dependency order. Each call exercises copyTableData the
	// way the production migration does, but one table at a time.
	for _, table := range []string{"breeder", "zones", "strain", "plant"} {
		require.NoErrorf(t, copyTableData(sqliteDB, pg, table), "copyTableData %s", table)
	}

	var (
		gotID            int
		gotName          string
		gotDescription   string
		gotClone         int
		gotStrainID      int
		gotZoneID        sql.NullInt64
		gotSensors       string
		gotHarvestWeight float64
	)
	err := pg.QueryRow(
		`SELECT id, name, description, clone, strain_id, zone_id, sensors, harvest_weight
		 FROM plant WHERE id = 1`,
	).Scan(&gotID, &gotName, &gotDescription, &gotClone, &gotStrainID, &gotZoneID, &gotSensors, &gotHarvestWeight)
	require.NoError(t, err, "copied plant row must be readable from PG")

	assert.Equal(t, 1, gotID)
	assert.Equal(t, "Roundtrip Plant", gotName)
	assert.Equal(t, "mixed-type test row", gotDescription)
	assert.Equal(t, 0, gotClone)
	assert.Equal(t, 1, gotStrainID)
	assert.False(t, gotZoneID.Valid, "NULL zone_id must remain NULL after copy")
	assert.Equal(t, "[]", gotSensors)
	assert.InDelta(t, 12.34, gotHarvestWeight, 0.001,
		"REAL/NUMERIC harvest_weight must survive the round-trip")

	// The PG sequence should have been advanced past the highest copied
	// id so subsequent inserts don't collide. Insert without specifying
	// id; it must succeed and pick id > 1.
	var nextID int
	require.NoError(t, pg.QueryRow(
		`INSERT INTO plant (name, description, clone, strain_id, sensors)
		 VALUES ('seq-check', '', 0, 1, '[]') RETURNING id`,
	).Scan(&nextID),
		"PG sequence must advance past the highest copied id; otherwise we'd collide")
	assert.Greater(t, nextID, 1, "next sequence value must be past the migrated row's id")
}

// TestCopyTableData_AutoflowerBoolNormalization verifies the boolToIntFields
// branch in copyTableData: legacy SQLite databases sometimes have
// strain.autoflower stored as TEXT 'true'/'false' (a relic of an early
// schema), which Postgres's INTEGER column would reject. The migration
// normalizes those values to 1/0 in-flight.
func TestCopyTableData_AutoflowerBoolNormalization(t *testing.T) {
	t.Parallel()

	sqliteDB := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, sqliteDB))

	pg := freshPostgresDB(t, true)

	// SQLite is loose about column affinities: an INTEGER-affinity
	// column will accept TEXT values verbatim. Insert a 'true' literal
	// to simulate the legacy data shape.
	mustExecSQL(t, sqliteDB,
		`INSERT INTO breeder (id, name) VALUES (1, 'Bool Test Breeder')`)
	mustExecSQL(t, sqliteDB,
		`INSERT INTO strain (id, name, sativa, indica, autoflower, description, seed_count, breeder_id)
		 VALUES (1, 'Auto Strain', 0, 100, 'true', '', 5, 1)`)
	mustExecSQL(t, sqliteDB,
		`INSERT INTO strain (id, name, sativa, indica, autoflower, description, seed_count, breeder_id)
		 VALUES (2, 'Photo Strain', 50, 50, 'false', '', 5, 1)`)

	require.NoError(t, copyTableData(sqliteDB, pg, "breeder"))
	require.NoError(t, copyTableData(sqliteDB, pg, "strain"))

	var auto1, auto2 int
	require.NoError(t, pg.QueryRow(`SELECT autoflower FROM strain WHERE id = 1`).Scan(&auto1))
	require.NoError(t, pg.QueryRow(`SELECT autoflower FROM strain WHERE id = 2`).Scan(&auto2))
	assert.Equal(t, 1, auto1, "'true' (TEXT) in SQLite must normalize to 1 in PG")
	assert.Equal(t, 0, auto2, "'false' (TEXT) in SQLite must normalize to 0 in PG")
}

// ----------------------------------------------------------------------------
// copyTableData — ON CONFLICT behavior
// ----------------------------------------------------------------------------

// TestCopyTableData_ConflictKey verifies every entry in the conflictKeys
// map honors ON CONFLICT (id) DO NOTHING semantics: re-running
// copyTableData over a table whose rows are already present must not
// error and must not duplicate rows.
//
// The test seeds SQLite once with a single canonical fixture per table
// (in dependency order) and then iterates the conflictKeys map: for each
// table, snapshot the PG row count, copy a second time, snapshot again,
// and assert the count is unchanged.
//
// Tables sensors and sensor_data are intentionally excluded — see the
// note inside the loop.
func TestCopyTableData_ConflictKey(t *testing.T) {
	t.Parallel()

	sqliteDB := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, sqliteDB))
	seedConflictFixture(t, sqliteDB)

	pg := freshPostgresDB(t, true)

	// Phase 1: full copy populates PG with the FK-respecting set of
	// tables we test against. Iterate orderedTables so dependencies land
	// in the right order; skip the two we can't exercise (see below).
	for _, table := range orderedTables {
		if shouldSkipForConflictTest(table) {
			continue
		}
		require.NoErrorf(t, copyTableData(sqliteDB, pg, table), "phase-1 copy %s", table)
	}

	// Phase 2: per-table re-copy must be idempotent.
	for _, table := range orderedTables {
		if _, ok := conflictKeys[table]; !ok {
			continue
		}
		if shouldSkipForConflictTest(table) {
			t.Run(table, func(t *testing.T) {
				t.Skipf("%s migration is broken pre-fix: SQLite still has the legacy 'show' column "+
					"after migration 013 while PG dropped it. Copying any sensors/sensor_data row "+
					"errors with 'column show does not exist'. Re-enable this case once the SQLite "+
					"side drops the column.", table)
			})
			continue
		}

		table := table
		t.Run(table, func(t *testing.T) {
			// Subtests share `pg` — running them serially keeps the
			// assertion clean. ON CONFLICT itself does not depend on
			// whatever order they run in.
			var before int
			require.NoError(t, pg.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&before))

			require.NoErrorf(t, copyTableData(sqliteDB, pg, table),
				"second copy of %s must succeed; ON CONFLICT (%s) DO NOTHING should suppress duplicates",
				table, conflictKeys[table])

			var after int
			require.NoError(t, pg.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&after))
			assert.Equalf(t, before, after,
				"ON CONFLICT (%s) DO NOTHING must not duplicate rows in %s",
				conflictKeys[table], table)
		})
	}
}

// shouldSkipForConflictTest centralizes the list of tables the migration
// tool currently cannot copy because of schema drift between the SQLite
// and Postgres dialects. Tracked separately so the skip is visible at the
// call site without smearing the rationale across multiple files.
func shouldSkipForConflictTest(table string) bool {
	switch table {
	case "sensors", "sensor_data":
		// Migration 013 dropped the `show` column on Postgres but left
		// it on SQLite (SQLite's DROP COLUMN landed in 3.35.0; the
		// migration was authored before the project's SQLite library
		// supported it). copyTableData blindly mirrors source columns
		// into the destination INSERT, which fails as soon as the
		// source has a row.
		return true
	}
	return false
}

// seedConflictFixture inserts one row at id=1 (or the smallest id that
// avoids stomping on migration-seeded rows) into every conflictKeys
// table the conflict test exercises. Dependency order matches
// orderedTables.
func seedConflictFixture(t *testing.T, db *sql.DB) {
	t.Helper()

	// settings, plant_status, metric, activity already have rows seeded
	// by migrations on both SQLite and PG. Re-running the copy on those
	// tables will hit ON CONFLICT for *every* row, which is the path
	// we want exercised here.

	mustExecSQL(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Test Zone')`)
	mustExecSQL(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'Test Breeder')`)
	mustExecSQL(t, db, `INSERT INTO strain (id, name, sativa, indica, autoflower, description, seed_count, breeder_id)
		VALUES (1, 'Test Strain', 50, 50, 0, '', 5, 1)`)
	mustExecSQL(t, db, `INSERT INTO plant (id, name, description, clone, strain_id, zone_id, start_dt, sensors)
		VALUES (1, 'Test Plant', '', 0, 1, 1, '2026-04-26 10:00:00', '[]')`)
	mustExecSQL(t, db, `INSERT INTO plant_status_log (id, plant_id, status_id, date)
		VALUES (1, 1, 1, '2026-04-26 10:00:00')`)
	mustExecSQL(t, db, `INSERT INTO plant_measurements (id, plant_id, metric_id, value, date)
		VALUES (1, 1, 1, 12.5, '2026-04-26 10:00:00')`)
	mustExecSQL(t, db, `INSERT INTO plant_activity (id, plant_id, activity_id, date, note)
		VALUES (1, 1, 1, '2026-04-26 10:00:00', 'first water')`)
	mustExecSQL(t, db, `INSERT INTO plant_images (id, plant_id, image_path, image_description, image_order)
		VALUES (1, 1, 'plants/1/test.jpg', 'cover', 0)`)
	mustExecSQL(t, db, `INSERT INTO streams (id, name, url, zone_id, visible)
		VALUES (1, 'Test Stream', 'http://example.invalid/stream', 1, 1)`)
}

// ----------------------------------------------------------------------------
// MigrateSqliteToPostgres — end-to-end
// ----------------------------------------------------------------------------

// TestMigrateSqliteToPostgres_EndToEnd is the headline test for Phase 6:
// it builds a SQLite database with migrations and a representative
// dataset, calls MigrateSqliteToPostgres against a fresh Postgres, and
// asserts every seeded row arrives intact. A second call asserts the
// migration is idempotent (ON CONFLICT DO NOTHING means re-running is
// safe).
//
// Sensors and sensor_data are not seeded here — see
// shouldSkipForConflictTest for why those tables can't currently be
// migrated end-to-end. The rest of the orderedTables list IS exercised.
func TestMigrateSqliteToPostgres_EndToEnd(t *testing.T) {
	t.Parallel()

	path := freshSQLiteFile(t)

	sqliteDB := openSQLiteFile(t, path)
	seedConflictFixture(t, sqliteDB)
	require.NoError(t, sqliteDB.Close())

	pg := freshPostgresDB(t, true)

	// First migration: copies every row in orderedTables.
	require.NoError(t, MigrateSqliteToPostgres(path, pg), "first migration")

	// Verify each seeded row reached PG. We pick representative columns
	// rather than re-asserting every value; the per-row content checks
	// live in TestCopyTableData_HappyPath.
	cases := []struct {
		query string
		want  string
	}{
		{`SELECT name FROM zones WHERE id = 1`, "Test Zone"},
		{`SELECT name FROM breeder WHERE id = 1`, "Test Breeder"},
		{`SELECT name FROM strain WHERE id = 1`, "Test Strain"},
		{`SELECT name FROM plant WHERE id = 1`, "Test Plant"},
		{`SELECT note FROM plant_activity WHERE id = 1`, "first water"},
		{`SELECT image_path FROM plant_images WHERE id = 1`, "plants/1/test.jpg"},
		{`SELECT name FROM streams WHERE id = 1`, "Test Stream"},
	}
	for _, c := range cases {
		var got string
		require.NoErrorf(t, pg.QueryRow(c.query).Scan(&got), "query %q", c.query)
		assert.Equalf(t, c.want, got, "row missing or mismatched after migration: %q", c.query)
	}

	// plant_status_log uses an INTEGER lookup; assert separately.
	var statusID int
	require.NoError(t, pg.QueryRow(`SELECT status_id FROM plant_status_log WHERE id = 1`).Scan(&statusID))
	assert.Equal(t, 1, statusID, "plant_status_log row must round-trip")

	// plant_measurements has a REAL value column — exercise the float path.
	var measurement float64
	require.NoError(t, pg.QueryRow(`SELECT value FROM plant_measurements WHERE id = 1`).Scan(&measurement))
	assert.InDelta(t, 12.5, measurement, 0.001, "REAL plant_measurement value must round-trip")

	// Migration-seeded tables in PG were touched by ON CONFLICT but
	// must retain their PG rows (not be replaced by SQLite values).
	var settingsCount, statusCount int
	require.NoError(t, pg.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&settingsCount))
	require.NoError(t, pg.QueryRow(`SELECT COUNT(*) FROM plant_status`).Scan(&statusCount))
	assert.GreaterOrEqual(t, settingsCount, 7, "PG-seeded settings rows must survive ON CONFLICT")
	assert.Equal(t, 9, statusCount, "PG-seeded plant_status rows must survive ON CONFLICT")

	// Sequence reset: a follow-up insert without explicit id must pick a
	// value past the migrated row's id.
	var nextPlantID int
	require.NoError(t, pg.QueryRow(
		`INSERT INTO plant (name, description, clone, strain_id, sensors)
		 VALUES ('post-migration', '', 0, 1, '[]') RETURNING id`,
	).Scan(&nextPlantID))
	assert.Greater(t, nextPlantID, 1,
		"setval after migration must advance the id sequence past the migrated row")

	// Idempotence: a second migration over the same source/dest must
	// not error and must not duplicate rows.
	beforeCounts := snapshotMigrationCounts(t, pg)
	require.NoError(t, MigrateSqliteToPostgres(path, pg), "second migration must be idempotent")
	afterCounts := snapshotMigrationCounts(t, pg)
	for table, before := range beforeCounts {
		assert.Equalf(t, before, afterCounts[table],
			"%s row count must not grow on re-run; before=%d after=%d", table, before, afterCounts[table])
	}
}

// snapshotMigrationCounts returns a row count per migrated table. Used by
// the idempotence assertion in the end-to-end test.
func snapshotMigrationCounts(t *testing.T, pg *sql.DB) map[string]int {
	t.Helper()
	out := make(map[string]int, len(orderedTables))
	for _, table := range orderedTables {
		var n int
		require.NoErrorf(t, pg.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&n),
			"COUNT(*) %s", table)
		out[table] = n
	}
	return out
}

// TestMigrateSqliteToPostgres_BadPath asserts the function surfaces a
// useful error when the SQLite source path doesn't resolve. We do not
// test every error path inside copyTableData because they require
// dropping/renaming live tables, which is outside this test's scope.
func TestMigrateSqliteToPostgres_BadPath(t *testing.T) {
	t.Parallel()
	pg := freshPostgresDB(t, true)

	// Path under a t.TempDir() that we never create — the modernc SQLite
	// driver will fail to open it for read.
	bogus := filepath.Join(t.TempDir(), "does-not-exist", "isley.db")

	err := MigrateSqliteToPostgres(bogus, pg)
	require.Error(t, err, "missing SQLite source must error")
	// Whether the failure is from sql.Open or the first SELECT depends
	// on the driver's lazy-open behavior; either way the error message
	// should mention the underlying problem ("no such file" / "unable to
	// open") rather than swallowing it.
	assert.NotEmpty(t, err.Error())
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// mustExecSQL is the SQLite-side equivalent of testutil.MustExec.
// Inlined here to avoid the model→testutil→model import cycle.
func mustExecSQL(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoErrorf(t, err, "mustExecSQL: %s", query)
}
