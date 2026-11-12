package model

// +parallel:serial — model.SetDriverForTesting
//
// TestDriverHelpers_RoundTripViaSetDriverForTesting and
// TestRunStartupMaintenance_NoOpForSQLite mutate the model.dbDriver
// package-global; TestCloseDB_NilSafe / TestGetDB_ReturnsCurrentGlobal
// mutate the model.db package-global; TestDbPath_* call t.Setenv
// (which forbids parallel). All of those globals are legitimate
// process-wide state and are not lifted by any later phase.
//
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
	"time"

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

func TestApplyMigrations_CreatesSmokeReportTables(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)
	require.NoError(t, applySQLiteMigrations(t, db))

	got := userTablesT(t, db)
	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}

	for _, name := range []string{"smoke_report_criterion", "smoke_report", "smoke_report_score"} {
		assert.Truef(t, gotSet[name], "expected table %q after migrations; got: %v", name, sortedCopy(got))
	}

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_criterion WHERE active = 1`).Scan(&n))
	assert.Equal(t, 5, n, "five criteria should be seeded and active")
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

// TestMigration024_ExistingKeysBecomeAdmin checks the scoped-keys upgrade:
// every key that existed before 024 could do everything, so it becomes
// admin; keys inserted afterwards without scopes get the safe default, read.
func TestMigration024_ExistingKeysBecomeAdmin(t *testing.T) {
	initTestLogger()
	db := freshSQLite(t)

	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
	require.NoError(t, err)
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(23))
	_, err = db.Exec(`INSERT INTO api_keys (name, key_hash, prefix) VALUES ('Existing', 'h', 'p')`)
	require.NoError(t, err)

	require.NoError(t, m.Migrate(24))
	var scopes string
	require.NoError(t, db.QueryRow(`SELECT scopes FROM api_keys WHERE name = 'Existing'`).Scan(&scopes))
	assert.Equal(t, "admin", scopes)

	_, err = db.Exec(`INSERT INTO api_keys (name, key_hash, prefix) VALUES ('Later', 'h2', 'p2')`)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT scopes FROM api_keys WHERE name = 'Later'`).Scan(&scopes))
	assert.Equal(t, "read", scopes)

	// And back down again.
	require.NoError(t, m.Migrate(23))
}

// TestMigration025_PlantSensorLinks checks that the JSON sensor links move
// to plant_sensor in order, dropping duplicates, dangling ids and junk,
// and that the down migration rebuilds the JSON.
func TestMigration025_PlantSensorLinks(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)

	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
	require.NoError(t, err)
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(24))

	seedPlantSensorLinks(t, db)
	require.NoError(t, m.Migrate(25))
	assertPlantSensorLinks(t, db)

	require.NoError(t, m.Migrate(24))
	var links string
	require.NoError(t, db.QueryRow(`SELECT sensors FROM plant WHERE name = 'A'`).Scan(&links))
	assert.Equal(t, "[2,1]", links)
	require.NoError(t, m.Migrate(25))
	assertPlantSensorLinks(t, db)
}

// seedPlantSensorLinks needs schema 24: plant A links sensors 2, 1, 2
// (duplicate) and 99 (gone); B has junk; C has none.
func seedPlantSensorLinks(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO breeder (id, name) VALUES (1, 'B')`,
		`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count) VALUES (1, 'S', 1, 50, 50, 0, '', 0)`,
		`INSERT INTO sensors (id, name, source, device, type) VALUES (1, 's1', 'x', 'd', 't1'), (2, 's2', 'x', 'd', 't2')`,
		`INSERT INTO plant (name, description, clone, strain_id, sensors) VALUES ('A', '', 0, 1, '[2, 1, 2, 99]')`,
		`INSERT INTO plant (name, description, clone, strain_id, sensors) VALUES ('B', '', 0, 1, 'not json')`,
		`INSERT INTO plant (name, description, clone, strain_id, sensors) VALUES ('C', '', 0, 1, '[]')`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
}

func assertPlantSensorLinks(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT p.name, ps.sensor_id FROM plant_sensor ps JOIN plant p ON p.id = ps.plant_id ORDER BY p.name, ps.position`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		var sensor int
		require.NoError(t, rows.Scan(&name, &sensor))
		got = append(got, fmt.Sprintf("%s:%d", name, sensor))
	}
	assert.Equal(t, []string{"A:2", "A:1"}, got)
	_, err = db.Exec(`SELECT sensors FROM plant`)
	assert.Error(t, err, "plant.sensors is dropped")
}

// TestMigration027_MergeDuplicateBreeders checks that breeders differing
// only in case or spaces merge into the oldest, with their strains and a
// CannaDB link, and that the name index then refuses new duplicates.
func TestMigration027_MergeDuplicateBreeders(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)

	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
	require.NoError(t, err)
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(26))

	seedDuplicateBreeders(t, db)
	require.NoError(t, m.Migrate(27))
	assertBreedersMerged(t, db)

	require.NoError(t, m.Migrate(26))
	require.NoError(t, m.Migrate(27))
}

// seedDuplicateBreeders needs schema 26: Mephisto (1) has duplicates
// "mephisto " (2, with a CannaDB link) and "MEPHISTO" (3); Ethos (4) has
// a duplicate "ethos" (5); both Ethos rows are linked, so 4 keeps its own.
func seedDuplicateBreeders(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO breeder (id, name) VALUES (1, 'Mephisto')`,
		`INSERT INTO breeder (id, name, cannadb_uri, cannadb_indexed_at) VALUES (2, 'mephisto ', 'at://m', '2026-01-01')`,
		`INSERT INTO breeder (id, name) VALUES (3, 'MEPHISTO')`,
		`INSERT INTO breeder (id, name, cannadb_uri) VALUES (4, 'Ethos', 'at://e1')`,
		`INSERT INTO breeder (id, name, cannadb_uri) VALUES (5, 'ethos', 'at://e2')`,
		`INSERT INTO breeder (id, name) VALUES (6, 'Solo')`,
		`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count) VALUES
			(1, 'A', 1, 50, 50, 0, '', 0), (2, 'B', 2, 50, 50, 0, '', 0), (3, 'C', 3, 50, 50, 0, '', 0),
			(4, 'D', 5, 50, 50, 0, '', 0), (5, 'E', 6, 50, 50, 0, '', 0)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
}

func assertBreedersMerged(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT id, name, COALESCE(cannadb_uri, ''), COALESCE(cannadb_indexed_at, '') FROM breeder ORDER BY id`)
	require.NoError(t, err)
	var got []string
	for rows.Next() {
		var id int
		var name, uri, at string
		require.NoError(t, rows.Scan(&id, &name, &uri, &at))
		got = append(got, fmt.Sprintf("%d %s %s %s", id, name, uri, strings.TrimSpace(at)))
	}
	require.NoError(t, rows.Err())
	rows.Close()
	assert.Equal(t, []string{"1 Mephisto at://m 2026-01-01", "4 Ethos at://e1 ", "6 Solo  "}, got)

	rows, err = db.Query(`SELECT id, breeder_id FROM strain ORDER BY id`)
	require.NoError(t, err)
	strains := map[int]int{}
	for rows.Next() {
		var id, b int
		require.NoError(t, rows.Scan(&id, &b))
		strains[id] = b
	}
	rows.Close()
	assert.Equal(t, map[int]int{1: 1, 2: 1, 3: 1, 4: 4, 5: 6}, strains)

	_, err = db.Exec(`INSERT INTO breeder (id, name) VALUES (100, 'SOLO')`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idx_breeder_name_lower")
	_, err = db.Exec(`INSERT INTO breeder (id, name) VALUES (101, 'Solo Two')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO breeder (id, name) VALUES (102, 'SOLO')`)
	assert.Error(t, err, "the name index refuses a new duplicate")
}

// TestMigration028_RepairHourlyRollup checks that hours with their readings
// are recomputed, while the earliest (pruned) hour and older history stay.
func TestMigration028_RepairHourlyRollup(t *testing.T) {
	t.Parallel()
	initTestLogger()
	db := freshSQLite(t)

	src, err := iofs.New(MigrationsFS, "migrations/sqlite")
	require.NoError(t, err)
	drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(27))

	seedShortRollup(t, db)
	require.NoError(t, m.Migrate(28))
	assertRollupRepaired(t, db)
	require.NoError(t, m.Migrate(27))
	require.NoError(t, m.Migrate(28))
}

// seedShortRollup needs schema 27. Readings run from 10:40 (pruning cut the
// 10:00 hour) to 12:50. The rollup has an old hour with no readings left,
// a short 10:00 row, a short 11:00 row and no 12:00 row.
func seedShortRollup(t *testing.T, db *sql.DB) {
	t.Helper()
	q := []string{
		`INSERT INTO sensors (id, name, source, device, type) VALUES (1, 's', 'x', 'd', 't')`,
		`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES
			(1, 1, '2026-11-10 10:40:00'), (1, 3, '2026-11-10 10:50:00'),
			(1, 10, '2026-11-10 11:10:00'), (1, 20, '2026-11-10 11:30:00'), (1, 30, '2026-11-10 11:50:00'),
			(1, 5, '2026-11-10 12:50:00')`,
		`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES
			(1, '2026-06-01 00:00:00', 7, 7, 7, 60),
			(1, '2026-11-10 10:00:00', 0, 4, 2, 60),
			(1, '2026-11-10 11:00:00', 30, 30, 30, 1)`,
	}
	for _, s := range q {
		_, err := db.Exec(s)
		require.NoError(t, err, s)
	}
}

func assertRollupRepaired(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT bucket, avg_val, sample_count FROM sensor_data_hourly WHERE sensor_id = 1 ORDER BY bucket`)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var bucket time.Time
		var avg float64
		var n int
		require.NoError(t, rows.Scan(&bucket, &avg, &n))
		got = append(got, fmt.Sprintf("%s avg=%g n=%d", bucket.Format("2006-01-02 15:04"), avg, n))
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{
		"2026-06-01 00:00 avg=7 n=60", // history beyond the readings: kept
		"2026-11-10 10:00 avg=2 n=60", // the pruned hour: left alone
		"2026-11-10 11:00 avg=20 n=3", // recomputed whole
		"2026-11-10 12:00 avg=5 n=1",  // filled in
	}, got)
}

// TestMigration029_RollingAvgTrigger: the rewritten trigger leaves the same
// rolling average as the window version it replaces (the mean of the 16
// readings before the newest), including for a late reading, and no longer
// scans the sensor's whole history.
func TestMigration029_RollingAvgTrigger(t *testing.T) {
	t.Parallel()
	initTestLogger()

	averages := func(version uint) (map[int]float64, string) {
		db := freshSQLite(t)
		src, err := iofs.New(MigrationsFS, "migrations/sqlite")
		require.NoError(t, err)
		drv, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
		require.NoError(t, err)
		m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
		require.NoError(t, err)
		require.NoError(t, m.Migrate(version))

		_, err = db.Exec(`INSERT INTO sensors (id, name, source, device, type) VALUES (1, 'a', 'x', 'd', 't'), (2, 'b', 'x', 'd', 't'), (3, 'c', 'x', 'd', 't')`)
		require.NoError(t, err)
		for i := 1; i <= 20; i++ {
			for s := 1; s <= 2; s++ {
				_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)`,
					s, (s-1)*100+i, fmt.Sprintf("2026-11-10 00:%02d:00", i))
				require.NoError(t, err)
			}
		}
		// A late reading for sensor 2, older than its newest.
		_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES (2, 1000, '2026-11-10 00:00:30')`)
		require.NoError(t, err)
		// A sensor with a single reading has no average yet.
		_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES (3, 5, '2026-11-10 00:01:00')`)
		require.NoError(t, err)

		got := map[int]float64{}
		rows, err := db.Query(`SELECT sensor_id, COALESCE(avg_value, -1) FROM rolling_averages`)
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var id int
			var v float64
			require.NoError(t, rows.Scan(&id, &v))
			got[id] = v
		}
		var ddl string
		require.NoError(t, db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'update_rolling_avg'`).Scan(&ddl))
		return got, ddl
	}

	before, oldDDL := averages(28)
	after, newDDL := averages(29)
	assert.Equal(t, map[int]float64{1: 11.5, 2: 111.5, 3: -1}, after)
	assert.Equal(t, before, after, "same averages as the trigger it replaces")
	assert.Contains(t, oldDDL, "OVER")
	assert.Contains(t, newDDL, "LIMIT 16 OFFSET 1")
}
