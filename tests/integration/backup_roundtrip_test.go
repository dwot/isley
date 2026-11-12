package integration

// End-to-end safety net for the backup feature, per Phase 4 of
// docs/TEST_PLAN.md. Every other backup test verifies a single
// endpoint in isolation; this one exercises the full user journey:
// create → list → download → wipe → restore → verify. A regression
// anywhere in that chain (the create goroutine, the manifest format,
// the truncate/insert order, the file extraction, FK ordering, etc.)
// fails this test rather than a user.
//
// The test is anchored to the per-engine BackupService introduced in
// Phase 3, so we do not depend on process-relative paths or on the
// model.GetDB() global for the async goroutines.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/handlers"
	"isley/tests/testutil"
)

// TestBackup_RoundTrip seeds production-shaped data, drives a backup
// through the HTTP API, downloads the archive, deletes the seeded
// rows, restores from the captured archive, and asserts the seeded
// rows return. Polling on completion is via the same /status endpoints
// the UI uses, so the status surface is exercised too.
func TestBackup_RoundTrip(t *testing.T) {
	t.Parallel()
	backupRoundTrip(t, testutil.NewTestDB(t))
}

// backupRoundTrip is the round trip on db, shared with the Postgres run.
func backupRoundTrip(t *testing.T, db *sql.DB) {
	t.Helper()
	dataDir := t.TempDir()
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))
	apiKey := testutil.SeedAPIKey(t, db, "roundtrip-key")
	// A second key that is NOT used for auth, so the wipe can delete it and the
	// restore has to bring it back — proving the api_keys table round-trips.
	testutil.SeedAPIKey(t, db, "extra-roundtrip-key")
	const extraKeyPrefix = "extra-ro"

	// 1. Seed a representative dataset across the FK chain. Each row
	//    sits in a distinct table so an over-eager truncate or a missing
	//    insertOrder entry is provable from the assertion alone.
	breederID := testutil.SeedBreeder(t, db, "Roundtrip Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "Roundtrip Strain")
	zoneID := testutil.SeedZone(t, db, "Roundtrip Zone")
	plantID := testutil.SeedPlant(t, db, "Roundtrip Plant", strainID, zoneID)

	// Smoke-report rows across all three tables, so a missing registration
	// in any backup table list is provable from the assertions below.
	criterionID := testutil.SeedSmokeCriterion(t, db, "Roundtrip Axis", 42)
	reportID := testutil.SeedSmokeReport(t, db, plantID, 4, map[int]int{criterionID: 3})

	// A sensor with one raw reading, and an hourly rollup that also holds an
	// hour retention has already pruned from sensor_data: the backup is the
	// only place that hour survives.
	sensorID := testutil.SeedSensor(t, db, "acinfinity", "RT1", "ACI.tempC")
	testutil.MustExec(t, db, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 21.5, '2026-11-10 12:30:00')`, sensorID)
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		VALUES ($1, '2026-03-01 08:00:00', 18, 22, 20, 60), ($1, '2026-11-10 12:00:00', 21.5, 21.5, 21.5, 1)`, sensorID)

	c := server.NewClient(t)

	// 2. Trigger the backup. CreateBackup returns 202 immediately and
	//    the work continues in a goroutine bound to server.BackupService.
	createResp, err := c.Do(testutil.APIReq(t, http.MethodPost,
		c.BaseURL+"/settings/backup/create", apiKey, nil, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, createResp.StatusCode,
		"CreateBackup should return 202 Accepted")
	testutil.DrainAndClose(createResp)

	// 3. Wait for the goroutine to finish.
	waitForBackupDone(t, c, apiKey, 30*time.Second)

	// 4. List, expect exactly one backup, capture its name.
	backups := listBackups(t, c, apiKey)
	require.Len(t, backups, 1, "exactly one backup should be present after CreateBackup")
	backupName := backups[0].Name
	require.NotEmpty(t, backupName)

	// 5. Download the archive. We hold the bytes in memory; the
	//    production restore goroutine reads the upload into memory too,
	//    so we are not bypassing any size guard.
	archive := downloadBackup(t, c, apiKey, backupName)
	require.NotEmpty(t, archive, "downloaded archive must not be empty")

	// The manifest records the running version (the test server's), not
	// "unknown" from a VERSION file lookup in the working directory.
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	assert.Equal(t, "isley-test", payload.Manifest.Version)

	// Sanity: the archive landed under the per-engine data dir, not
	// somewhere in CWD. Catches a regression where BackupDir() reverts
	// to a process-relative path.
	onDisk := filepath.Join(dataDir, "backups", backupName)
	info, err := os.Stat(onDisk)
	require.NoErrorf(t, err, "expected backup file at %s", onDisk)
	assert.Equal(t, int64(len(archive)), info.Size(),
		"downloaded archive size should match the file on disk")

	// 6. Wipe the seeded rows. Order respects FK constraints (children
	//    first). The auth key (roundtrip-key) is left in place so the
	//    follow-up requests still authenticate; the extra key is deleted
	//    here and must be brought back by the restore.
	testutil.MustExec(t, db, `DELETE FROM smoke_report_score`)
	testutil.MustExec(t, db, `DELETE FROM smoke_report`)
	testutil.MustExec(t, db, `DELETE FROM smoke_report_criterion`)
	testutil.MustExec(t, db, `DELETE FROM plant`)
	testutil.MustExec(t, db, `DELETE FROM strain`)
	testutil.MustExec(t, db, `DELETE FROM breeder`)
	testutil.MustExec(t, db, `DELETE FROM zones`)
	testutil.MustExec(t, db, `DELETE FROM api_keys WHERE prefix = $1`, extraKeyPrefix)
	// The rollup loses its history and gains a row the archive doesn't
	// have, which the restore must not keep.
	testutil.MustExec(t, db, `DELETE FROM sensor_data_hourly`)
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		VALUES ($1, '2026-11-11 00:00:00', 1, 1, 1, 1)`, sensorID)

	// Sanity: the wipe actually wiped.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant`).Scan(&n))
	require.Zero(t, n, "wipe failed: plant rows remain")
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE prefix = $1`, extraKeyPrefix).Scan(&n))
	require.Zero(t, n, "wipe failed: extra api key remains")

	// 7. Restore from the captured archive.
	body, ct := testutil.MultipartBody(t, "backup", backupName, archive)
	restoreResp, err := c.Do(testutil.APIReq(t, http.MethodPost,
		c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, restoreResp.StatusCode,
		"ImportBackup should return 202 Accepted")
	testutil.DrainAndClose(restoreResp)

	waitForRestoreDone(t, c, apiKey, 30*time.Second)

	// 8. Each seeded row should be back exactly as it was. We assert
	//    on the IDs we recorded pre-backup because Postgres TRUNCATE
	//    + sequence reset preserves them and SQLite DELETE preserves
	//    AUTOINCREMENT counters; either way, the row that comes back
	//    must carry the original id.
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM plant WHERE id = $1`, plantID).Scan(&name))
	assert.Equal(t, "Roundtrip Plant", name, "plant.name should round-trip")

	require.NoError(t, db.QueryRow(`SELECT name FROM strain WHERE id = $1`, strainID).Scan(&name))
	assert.Equal(t, "Roundtrip Strain", name, "strain.name should round-trip")

	require.NoError(t, db.QueryRow(`SELECT name FROM breeder WHERE id = $1`, breederID).Scan(&name))
	assert.Equal(t, "Roundtrip Breeder", name, "breeder.name should round-trip")

	require.NoError(t, db.QueryRow(`SELECT name FROM zones WHERE id = $1`, zoneID).Scan(&name))
	assert.Equal(t, "Roundtrip Zone", name, "zones.name should round-trip")

	// The deleted extra api key must be restored from the archive.
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE prefix = $1`, extraKeyPrefix).Scan(&n))
	assert.Equal(t, 1, n, "the extra api key should be restored from the backup")

	// The three smoke-report tables must come back. A table missing from any
	// of the ten registration points fails here and nowhere else: the feature
	// keeps working, the archive is just silently incomplete.
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_criterion WHERE id = $1`, criterionID).Scan(&n))
	assert.Equal(t, 1, n, "smoke_report_criterion should round-trip")

	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report WHERE id = $1`, reportID).Scan(&n))
	assert.Equal(t, 1, n, "smoke_report should round-trip")

	require.NoError(t, db.QueryRow(
		`SELECT value FROM smoke_report_score WHERE report_id = $1`, reportID).Scan(&n))
	assert.Equal(t, 3, n, "smoke_report_score should round-trip with its value intact")

	// The hourly rollup comes back exactly, older history included.
	rows, err := db.Query(`SELECT bucket, avg_val, sample_count FROM sensor_data_hourly WHERE sensor_id = $1 ORDER BY bucket`, sensorID)
	require.NoError(t, err)
	defer rows.Close()
	var hourly []string
	for rows.Next() {
		var bucket time.Time
		var avg float64
		var count int
		require.NoError(t, rows.Scan(&bucket, &avg, &count))
		hourly = append(hourly, fmt.Sprintf("%s avg=%g n=%d", bucket.UTC().Format("2006-01-02 15:04"), avg, count))
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"2026-03-01 08:00 avg=20 n=60", "2026-11-10 12:00 avg=21.5 n=1"}, hourly,
		"sensor_data_hourly should round-trip, including hours with no raw readings left")
}

// ---------------------------------------------------------------------------
// Helpers — file-local on purpose. waitForBackupDone / waitForRestoreDone
// are specific to this round-trip flow; they belong with the test that
// uses them rather than in testutil. listBackups and downloadBackup are
// thin enough that promoting them would be ceremony, not abstraction.
// ---------------------------------------------------------------------------

// waitForBackupDone polls /settings/backup/status until in_progress
// flips back to false. The CreateBackup handler sets the in-progress
// flag synchronously before returning 202, so the first poll cannot
// race past an unstarted goroutine.
func waitForBackupDone(t *testing.T, c *testutil.Client, apiKey string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := fetchBackupStatus(t, c, apiKey)
		if !status.InProgress {
			require.Emptyf(t, status.Error,
				"backup goroutine reported error: %s", status.Error)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("backup did not complete within %s", timeout)
}

// waitForRestoreDone is the restore-side analogue of waitForBackupDone.
func waitForRestoreDone(t *testing.T, c *testutil.Client, apiKey string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := fetchRestoreStatus(t, c, apiKey)
		if !status.InProgress {
			require.Emptyf(t, status.Error,
				"restore goroutine reported error: %s", status.Error)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("restore did not complete within %s", timeout)
}

func fetchBackupStatus(t *testing.T, c *testutil.Client, apiKey string) handlers.BackupStatus {
	t.Helper()
	resp := c.APIGet(t, "/settings/backup/status", apiKey)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var status handlers.BackupStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	return status
}

func fetchRestoreStatus(t *testing.T, c *testutil.Client, apiKey string) handlers.RestoreStatus {
	t.Helper()
	resp := c.APIGet(t, "/settings/backup/restore/status", apiKey)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var status handlers.RestoreStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&status))
	return status
}

func listBackups(t *testing.T, c *testutil.Client, apiKey string) []handlers.BackupFileInfo {
	t.Helper()
	resp := c.APIGet(t, "/settings/backup/list", apiKey)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out []handlers.BackupFileInfo
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func downloadBackup(t *testing.T, c *testutil.Client, apiKey, name string) []byte {
	t.Helper()
	resp := c.APIGet(t, "/settings/backup/download/"+name, apiKey)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return body
}

// An archive made before the UTC conversion (no time_storage setting) is
// converted when it is restored: its wall times, written in its own
// instance zone, come back as UTC, and the flag is set.
func TestBackup_RestoreLegacyArchiveConvertsTimes(t *testing.T) {
	t.Parallel()

	// The old instance: New York, times stored as New York wall time. A
	// bare database never had an engine start on it, so it is unconverted.
	old := testutil.NewTestDB(t)
	testutil.MustExec(t, old, `UPDATE settings SET value = 'America/New_York' WHERE name = 'timezone'`)
	// The restore replaces the keys with the archive's, so the archive
	// carries the key the test polls with.
	testutil.SeedAPIKey(t, old, "legacy-restore-key")
	plant := testutil.SeedPlant(t, old, "Old",
		testutil.SeedStrain(t, old, testutil.SeedBreeder(t, old, "B"), "S"), testutil.SeedZone(t, old, "Z"))
	var water int
	require.NoError(t, old.QueryRow(`SELECT id FROM activity WHERE name = 'Water'`).Scan(&water))
	testutil.MustExec(t, old, `INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'late', '2026-11-10T23:30')`,
		plant, water)
	archive, _, err := handlers.BuildBackupArchive(old, handlers.BuildArchiveOptions{SensorDays: -1})
	require.NoError(t, err)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))
	apiKey := testutil.SeedAPIKey(t, db, "legacy-restore-key")
	c := server.NewClient(t)
	body, ct := testutil.MultipartBody(t, "backup", "legacy.zip", archive)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	testutil.DrainAndClose(resp)
	waitForRestoreDone(t, c, apiKey, 30*time.Second)

	var stored chrono.Stored
	require.NoError(t, db.QueryRow(`SELECT date FROM plant_activity WHERE note = 'late'`).Scan(&stored))
	assert.Equal(t, "2026-11-11 04:30:00", stored.Time.Format("2006-01-02 15:04:05"))
	var flag string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE name = 'time_storage'`).Scan(&flag))
	assert.Equal(t, "utc", flag)
}
