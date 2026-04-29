//go:build integration_postgres

// Package integration's *_postgres files run against a real PostgreSQL
// instance instead of the in-memory SQLite the rest of the suite uses.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./tests/integration/...
//
// CI invocation: see .github/workflows/release.yml job test-postgres
// and .gitlab-ci.yml job test-postgres.
//
// Connection params come from environment (with defaults for the local
// docker-run example in docs/TEST_PLAN.md Phase 5):
//
//	ISLEY_TEST_DB_HOST     (default "localhost")
//	ISLEY_TEST_DB_PORT     (default "5432")
//	ISLEY_TEST_DB_USER     (default "postgres")
//	ISLEY_TEST_DB_PASSWORD (default "test")
//	ISLEY_TEST_DB_ADMIN    (default "postgres" — db used for CREATE/DROP)
//
// IMPORTANT: This file shares a Go test binary with the SQLite integration
// tests. The Postgres tests set model.dbDriver to "postgres" via
// SetDriverForTesting; running them in the same process as SQLite tests
// would clobber that global. Always invoke with `-run Postgres` so only
// these tests execute under the integration_postgres tag.
package integration

import (
	"sort"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestPostgres_Smoke opens the test DB and runs SELECT 1. If this fails,
// every other Postgres test is going to fail too — keeping it first makes
// CI failures point at "Postgres isn't reachable" instead of a deeper
// migration or schema issue.
func TestPostgres_Smoke(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)

	var n int
	require.NoError(t, db.QueryRow("SELECT 1").Scan(&n))
	assert.Equal(t, 1, n)

	// Sanity-check the dialect: production code branches on this and a
	// silent regression would be diagnosed somewhere far from the cause.
	var version string
	require.NoError(t, db.QueryRow("SHOW server_version").Scan(&version))
	assert.NotEmpty(t, version, "server_version must be non-empty on a real Postgres")
}

// TestPostgres_Migrations applies the migrations bundled in
// model/migrations/postgres/ on a fresh database and asserts the expected
// schema is in place. Catches regressions where a new migration adds the
// table on SQLite but forgets the Postgres counterpart (or vice-versa).
func TestPostgres_Migrations(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)

	rows, err := db.Query(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type   = 'BASE TABLE'
		ORDER BY table_name
	`)
	require.NoError(t, err)
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	sort.Strings(got)

	// Same tables the SQLite harness asserts on (tests/testutil/db_test.go),
	// so a divergence between the two migration trees fails *somewhere*.
	want := []string{
		"activity",
		"breeder",
		"plant",
		"plant_activity",
		"plant_images",
		"plant_measurements",
		"plant_status",
		"plant_status_log",
		"sensor_data",
		"sensors",
		"settings",
		"strain",
		"strain_lineage",
		"streams",
		"zones",
	}
	for _, table := range want {
		assert.Containsf(t, got, table, "expected table %q after migrations; got %v", table, got)
	}

	// The schema_migrations bookkeeping table should exist and report a
	// non-zero current version once Up() has run.
	var version int
	var dirty bool
	require.NoError(t, db.QueryRow(`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty))
	assert.False(t, dirty, "schema_migrations must not be dirty after a clean migrate up")
	assert.Greater(t, version, 0, "schema_migrations.version should be set after migrations run")
}

// TestPostgres_IsolatedBetweenCalls is the Postgres analogue of
// tests/testutil/db_test.go::TestNewTestDB_IsolatedBetweenCalls. It
// guarantees that two NewTestPostgresDB calls in the same test process
// see independent state — the per-test database creation is the
// isolation primitive the rest of the Postgres suite relies on.
func TestPostgres_IsolatedBetweenCalls(t *testing.T) {
	t.Parallel()

	a := testutil.NewTestPostgresDB(t)
	b := testutil.NewTestPostgresDB(t)

	_, err := a.Exec(`INSERT INTO settings (name, value) VALUES ('canary', 'a-only')`)
	require.NoError(t, err)

	var count int
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'canary'`).Scan(&count))
	assert.Zero(t, count, "rows written to db A must not appear in db B")
}
