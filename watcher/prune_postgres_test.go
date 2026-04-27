//go:build integration_postgres

// Postgres-flavored prune tests. The default-build counterparts in
// watcher_test.go exercise the SQLite branch of PruneSensorData (which
// uses datetime('now', 'localtime', '-N days') and the SQLite-only
// VACUUM / ANALYZE / PRAGMA optimize maintenance). This file verifies
// that the Postgres branch — `DELETE FROM sensor_data WHERE create_dt
// < NOW() - INTERVAL 'N days'` — actually executes against real PG and
// the maintenance block is correctly gated off.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./watcher/...

package watcher

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// insertReadingAtPG mirrors insertReadingAt in watcher_test.go but uses
// RETURNING id rather than res.LastInsertId() (lib/pq does not implement
// LastInsertId). The follow-up UPDATE forces create_dt to the requested
// wall-clock so the prune query has a deterministic input regardless of
// when the test ran.
func insertReadingAtPG(t *testing.T, db *sql.DB, sensorID int, value float64, ts time.Time) {
	t.Helper()
	var id int
	require.NoError(t,
		db.QueryRow(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2) RETURNING id`, sensorID, value).Scan(&id),
	)
	_, err := db.Exec(
		`UPDATE sensor_data SET create_dt = $1 WHERE id = $2`,
		ts.Format("2006-01-02 15:04:05"), id,
	)
	require.NoError(t, err)
}

// TestPruneSensorData_RetentionDisabled_Postgres asserts that retention=0
// short-circuits before reaching the dialect branch — the row survives.
// Mirrors the SQLite test but lets us confirm the production code's early
// return doesn't accidentally evaluate the Postgres INTERVAL string with
// a literal 0 and produce weird behavior.
func TestPruneSensorData_RetentionDisabled_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := seedSensor(t, db, "x", "y", "z")
	insertReadingAtPG(t, db, id, 1.0, time.Now().AddDate(0, 0, -100))

	w := newTestWatcher(t, db)
	require.NoError(t, w.PruneSensorData())

	assert.Equal(t, 1, countSensorData(t, db, id), "row must survive when retention is disabled")
}

// TestPruneSensorData_DeletesOnlyOldRows_Postgres exercises the actual
// Postgres prune SQL. The Postgres branch uses NOW() - INTERVAL '%d days'
// and skips the SQLite maintenance block; a typo in either the INTERVAL
// literal or the gating IsSQLite() check fails this test.
func TestPruneSensorData_DeletesOnlyOldRows_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	id := seedSensor(t, db, "x", "y", "z")

	insertReadingAtPG(t, db, id, 1.0, time.Now().AddDate(0, 0, -60)) // outside
	insertReadingAtPG(t, db, id, 2.0, time.Now().Add(-1*time.Hour))  // inside

	w := newTestWatcher(t, db)
	w.SensorRetention = func() int { return 30 }

	require.NoError(t, w.PruneSensorData())

	assert.Equal(t, 1, countSensorData(t, db, id), "only the inside-window row should remain")

	var keptValue float64
	require.NoError(t, db.QueryRow(
		`SELECT value FROM sensor_data WHERE sensor_id = $1`, id,
	).Scan(&keptValue))
	assert.InDelta(t, 2.0, keptValue, 0.0001)
}
