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
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	dataDir := t.TempDir()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))
	apiKey := testutil.SeedAPIKey(t, db, "roundtrip-key")

	// 1. Seed a representative dataset across the FK chain. Each row
	//    sits in a distinct table so an over-eager truncate or a missing
	//    insertOrder entry is provable from the assertion alone.
	breederID := testutil.SeedBreeder(t, db, "Roundtrip Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "Roundtrip Strain")
	zoneID := testutil.SeedZone(t, db, "Roundtrip Zone")
	plantID := testutil.SeedPlant(t, db, "Roundtrip Plant", strainID, zoneID)

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

	// Sanity: the archive landed under the per-engine data dir, not
	// somewhere in CWD. Catches a regression where BackupDir() reverts
	// to a process-relative path.
	onDisk := filepath.Join(dataDir, "backups", backupName)
	info, err := os.Stat(onDisk)
	require.NoErrorf(t, err, "expected backup file at %s", onDisk)
	assert.Equal(t, int64(len(archive)), info.Size(),
		"downloaded archive size should match the file on disk")

	// 6. Wipe the seeded rows. Order respects FK constraints (children
	//    first). The settings api_key row is left in place so the next
	//    request still authenticates; the backup itself contains the
	//    api_key row so it survives the round-trip too.
	testutil.MustExec(t, db, `DELETE FROM plant`)
	testutil.MustExec(t, db, `DELETE FROM strain`)
	testutil.MustExec(t, db, `DELETE FROM breeder`)
	testutil.MustExec(t, db, `DELETE FROM zones`)

	// Sanity: the wipe actually wiped.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant`).Scan(&n))
	require.Zero(t, n, "wipe failed: plant rows remain")

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
