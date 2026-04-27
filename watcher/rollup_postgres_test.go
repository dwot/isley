//go:build integration_postgres

// Postgres-flavored rollup tests. The default-build counterparts in
// rollup_test.go exercise the SQLite branch of buildSQLiteRollupQuery
// against an in-memory SQLite. This file mirrors them against a real
// PostgreSQL so buildPostgresRollupQuery (date_trunc, INTERVAL '25 hours',
// ON CONFLICT DO UPDATE) is exercised end-to-end against the dialect it
// targets — not just asserted to be the chosen branch.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./watcher/...
//
// CI invocation: see .github/workflows/release.yml job test-postgres
// and .gitlab-ci.yml job test-postgres. Phase 7 of docs/TEST_PLAN_2.md
// added ./watcher/... to that job's path list and these tests to the
// `-run Postgres` filter set (every Test function below has "Postgres"
// in its name so the existing filter picks them up).

package watcher

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// readingsAtPG is the Postgres flavor of readingsAt in rollup_test.go. The
// SQLite version uses res.LastInsertId() which lib/pq does not support; PG
// returns the new id via RETURNING. The created row's create_dt then gets
// rewritten to the desired wall-clock so the rollup query has deterministic
// input regardless of when the test ran.
func readingsAtPG(t *testing.T, db *sql.DB, sensorID int, ts time.Time, values ...float64) {
	t.Helper()
	stamp := ts.Format("2006-01-02 15:04:05")
	for _, v := range values {
		var id int
		require.NoError(t,
			db.QueryRow(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2) RETURNING id`, sensorID, v).Scan(&id),
		)
		_, err := db.Exec(`UPDATE sensor_data SET create_dt = $1 WHERE id = $2`, stamp, id)
		require.NoError(t, err)
	}
}

// hourlyBucketPG mirrors hourlyBucket but reads bucket as a TIMESTAMP and
// matches against the parameterized string Postgres will implicitly cast
// to a TIMESTAMP. Returns the (min, max, avg, sample_count) tuple for the
// bucket so callers can assert on the aggregated values directly.
func hourlyBucketPG(t *testing.T, db *sql.DB, sensorID int, bucket string) (minV, maxV, avgV float64, n int) {
	t.Helper()
	err := db.QueryRow(
		`SELECT min_val, max_val, avg_val, sample_count
		 FROM sensor_data_hourly
		 WHERE sensor_id = $1 AND bucket = $2::timestamp`,
		sensorID, bucket,
	).Scan(&minV, &maxV, &avgV, &n)
	require.NoError(t, err, "no rollup row for sensor=%d bucket=%s", sensorID, bucket)
	return
}

// TestRefreshHourlyRollups_FullBackfillEmptyTable_Postgres mirrors the
// SQLite full-backfill test against real PG. Verifies that the
// buildPostgresRollupQuery's date_trunc('hour', ...) correctly produces
// one bucket per distinct (sensor, hour) and that ON CONFLICT does not
// fire on the first run.
func TestRefreshHourlyRollups_FullBackfillEmptyTable_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := seedSensor(t, db, "test", "dev", "temp")

	readingsAtPG(t, db, id, time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC), 10, 12)
	readingsAtPG(t, db, id, time.Date(2026, 1, 1, 11, 15, 0, 0, time.UTC), 14)
	readingsAtPG(t, db, id, time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC), 20, 22, 24)

	w := newTestWatcher(t, db)
	require.NoError(t, w.RefreshHourlyRollups())

	var rolled int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data_hourly WHERE sensor_id = $1`, id).Scan(&rolled))
	assert.Equal(t, 3, rolled, "expected one rollup row per distinct (sensor, hour) bucket")

	minV, maxV, avgV, n := hourlyBucketPG(t, db, id, "2026-01-01 10:00:00")
	assert.InDelta(t, 10.0, minV, 0.0001)
	assert.InDelta(t, 12.0, maxV, 0.0001)
	assert.InDelta(t, 11.0, avgV, 0.0001)
	assert.Equal(t, 2, n)

	minV, maxV, avgV, n = hourlyBucketPG(t, db, id, "2026-01-05 08:00:00")
	assert.InDelta(t, 20.0, minV, 0.0001)
	assert.InDelta(t, 24.0, maxV, 0.0001)
	assert.InDelta(t, 22.0, avgV, 0.0001)
	assert.Equal(t, 3, n)
}

// TestRefreshHourlyRollups_IncrementalSkipsOldData_Postgres verifies the
// 25-hour window is honored against real PG INTERVAL arithmetic. A frozen
// rollup row outside the window is preserved verbatim; new raw data inside
// the window aggregates into a fresh bucket.
func TestRefreshHourlyRollups_IncrementalSkipsOldData_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := seedSensor(t, db, "test", "dev", "temp")

	now := time.Now()

	frozenBucket := now.Add(-30 * time.Hour).Format("2006-01-02 15:00:00")
	_, err := db.Exec(
		`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		 VALUES ($1, $2::timestamp, $3, $4, $5, $6)`,
		id, frozenBucket, 99.0, 99.0, 99.0, 1,
	)
	require.NoError(t, err)

	recent := now.Add(-2 * time.Hour)
	readingsAtPG(t, db, id, recent, 10, 20, 30)

	w := newTestWatcher(t, db)
	require.NoError(t, w.RefreshHourlyRollups())

	minV, _, _, _ := hourlyBucketPG(t, db, id, frozenBucket)
	assert.InDelta(t, 99.0, minV, 0.0001, "out-of-window rollup row should be untouched")

	recentBucket := recent.Format("2006-01-02 15:00:00")
	minV, maxV, avgV, n := hourlyBucketPG(t, db, id, recentBucket)
	assert.InDelta(t, 10.0, minV, 0.0001)
	assert.InDelta(t, 30.0, maxV, 0.0001)
	assert.InDelta(t, 20.0, avgV, 0.0001)
	assert.Equal(t, 3, n)
}

// TestRefreshHourlyRollups_IncrementalUpdatesInWindowBucket_Postgres
// exercises the ON CONFLICT (sensor_id, bucket) DO UPDATE branch. The
// SQLite flavor uses INSERT OR REPLACE; Postgres needs explicit
// ON CONFLICT clauses, which only fire on the second run when a bucket
// already exists. A regression in the conflict handling would either
// duplicate rows (no constraint update) or fail outright (wrong target).
func TestRefreshHourlyRollups_IncrementalUpdatesInWindowBucket_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := seedSensor(t, db, "test", "dev", "temp")

	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	readingsAtPG(t, db, id, hourAgo, 10)

	w := newTestWatcher(t, db)
	require.NoError(t, w.RefreshHourlyRollups()) // full backfill

	bucketKey := hourAgo.Format("2006-01-02 15:00:00")
	minV, maxV, _, n := hourlyBucketPG(t, db, id, bucketKey)
	require.InDelta(t, 10.0, minV, 0.0001)
	require.InDelta(t, 10.0, maxV, 0.0001)
	require.Equal(t, 1, n)

	readingsAtPG(t, db, id, hourAgo, 30)
	require.NoError(t, w.RefreshHourlyRollups()) // incremental — must hit ON CONFLICT

	minV, maxV, _, n = hourlyBucketPG(t, db, id, bucketKey)
	assert.InDelta(t, 10.0, minV, 0.0001, "min stays at 10")
	assert.InDelta(t, 30.0, maxV, 0.0001, "max bumps to 30")
	assert.Equal(t, 2, n, "sample_count should reflect the combined readings")

	var rowsForBucket int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sensor_data_hourly WHERE sensor_id = $1 AND bucket = $2::timestamp`,
		id, bucketKey,
	).Scan(&rowsForBucket))
	assert.Equal(t, 1, rowsForBucket, "ON CONFLICT must update in place, not insert a duplicate")
}
