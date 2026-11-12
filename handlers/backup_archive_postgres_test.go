//go:build integration_postgres

// Postgres-flavored ApplyBackupToDB tests. The default-build counterparts
// in backup_archive_test.go exercise the SQLite branch (DELETE FROM,
// PRAGMA foreign_keys = OFF, no sequence reset). This file mirrors them
// against a real PostgreSQL so the Postgres-specific branches —
// TRUNCATE TABLE ... CASCADE, SET CONSTRAINTS ALL DEFERRED, and the
// setval(pg_get_serial_sequence(...)) sequence-reset loop — actually
// execute end-to-end against the dialect they target.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...
//
// Note: this file builds the BackupPayload directly rather than going
// through BuildBackupArchive → ParseBackupArchive. The dump path is
// itself dialect-aware (dumpTableFiltered branches on IsPostgres()) and
// pulling in a SQLite source under this build tag would mix dialects in
// the same process. Constructing the payload by hand keeps the test
// focused on the apply path.

package handlers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// TestApplyBackupToDB_RoundTrip_Postgres exercises the Postgres branch of
// ApplyBackupToDB end-to-end. A regression in the truncate ordering, the
// CASCADE clause, or the constraint deferral would surface here as an
// FK error or a row-count mismatch.
func TestApplyBackupToDB_RoundTrip_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)
	payload := samplePayload()

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, payload))

	expectedCounts := map[string]int{
		"settings":    2,
		"zones":       1,
		"sensors":     1,
		"sensor_data": 2,
		"breeder":     1,
		"strain":      1,
	}
	for table, want := range expectedCounts {
		var got int
		require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&got))
		assert.Equalf(t, want, got, "row count for %s after Postgres apply", table)
	}

	var v string
	require.NoError(t, dst.QueryRow(`SELECT value FROM settings WHERE name = 'canary'`).Scan(&v))
	assert.Equal(t, "green", v)

	var vpdTemp, vpdHum int
	require.NoError(t, dst.QueryRow(`SELECT vpd_temp_sensor_id, vpd_humidity_sensor_id FROM zones WHERE id = 1`).Scan(&vpdTemp, &vpdHum))
	assert.Equal(t, 1, vpdTemp, "the zone's VPD sensor links survive the restore")
	assert.Equal(t, 1, vpdHum)

	var name string
	var breederID int
	require.NoError(t, dst.QueryRow(`SELECT name, breeder_id FROM strain WHERE id = 1`).Scan(&name, &breederID))
	assert.Equal(t, "OG Test", name)
	assert.Equal(t, 1, breederID)
}

// TestApplyBackupToDB_OverwritesExistingData_Postgres asserts that
// TRUNCATE ... CASCADE wipes pre-existing rows. SQLite uses DELETE FROM;
// PG needs CASCADE because dependent FK rows would otherwise block the
// truncate. A test that forgets CASCADE fails here with a foreign-key
// violation.
func TestApplyBackupToDB_OverwritesExistingData_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)

	_, err := dst.Exec(`INSERT INTO settings (name, value) VALUES ('stale', 'should-be-gone')`)
	require.NoError(t, err)

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, samplePayload()))

	var staleCount int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'stale'`).Scan(&staleCount))
	assert.Zero(t, staleCount, "pre-existing rows must be truncated before restore")
}

// TestApplyBackupToDB_ResetsSequences_Postgres locks in the
// post-restore setval loop. After restoring rows with explicit ids, the
// next INSERT without an id must NOT collide with an imported id —
// which only holds if the sequence got bumped past MAX(id) by the
// production setval(pg_get_serial_sequence(...)) loop. SQLite has no
// equivalent because AUTOINCREMENT tracks the high-water mark via
// sqlite_sequence automatically.
func TestApplyBackupToDB_ResetsSequences_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, samplePayload()))

	// Insert without specifying id — should auto-assign past MAX(id)=1.
	var newID int
	require.NoError(t,
		dst.QueryRow(`INSERT INTO breeder (name) VALUES ($1) RETURNING id`, "Fresh").Scan(&newID),
	)
	assert.Greaterf(t, newID, 1, "sequence must be reset past the imported MAX(id); got %d", newID)
}

// TestApplyBackupToDB_PreFeatureBackupPreservesSeededCriteriaPostgres is the
// Postgres-dialect counterpart of
// TestApplyBackupToDB_PreFeatureBackupPreservesSeededCriteria in
// backup_archive_test.go (IMPORTANT 2 from the final whole-branch review).
// samplePayload never sets SmokeCriteria, matching a backup
// produced before the smoke-report feature existed, so this exercises the
// tablesToTruncate guard against a real TRUNCATE TABLE ... CASCADE rather
// than SQLite's DELETE FROM.
func TestApplyBackupToDB_PreFeatureBackupPreservesSeededCriteriaPostgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)

	var before int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM smoke_report_criterion`).Scan(&before))
	require.Equal(t, 5, before, "fresh Postgres DB must start with the five migration-seeded criteria")

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, samplePayload()))

	var after int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM smoke_report_criterion`).Scan(&after))
	assert.Equal(t, 5, after, "a payload with no smoke-report data must not empty the seeded criteria")
}

// TestApplyBackupToDB_RollingAverages_Postgres: the restore loads sensor_data
// with the per-row rolling-average trigger off (it made prod-sized restores
// take hours) and works the averages out once at the end.
func TestApplyBackupToDB_RollingAverages_Postgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), db, rollingAvgPayload()))
	assertRollingAverages(t, db)
}

// TestPostgresBatchedRestore_RollingAverages covers the same for the batched
// bulk load runRestore uses for sensor_data.
func TestPostgresBatchedRestore_RollingAverages(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	p := rollingAvgPayload()
	data := p.SensorData
	p.SensorData = nil
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), db, p))
	require.NoError(t, handlers.InsertRowsBatchedForTest(db, "sensor_data", data))
	assertRollingAverages(t, db)
}
