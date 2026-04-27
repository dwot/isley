package handlers_test

// HTTP-layer tests for the nine backup endpoints in handlers/backup.go.
// Each test gets its own engine + per-test data directory via
// testutil.WithDataDir so backup state never leaks between tests and
// every test can call t.Parallel().
//
// The endpoints under test (all mounted on the api-protected group):
//
//   POST   /settings/backup/create          → CreateBackup
//   GET    /settings/backup/status          → GetBackupStatus
//   GET    /settings/backup/list            → ListBackups
//   GET    /settings/backup/download/:name  → DownloadBackup
//   DELETE /settings/backup/:name           → DeleteBackup
//   POST   /settings/backup/restore         → ImportBackup
//   GET    /settings/backup/restore/status  → GetRestoreStatus
//   GET    /settings/backup/sqlite/download → DownloadSQLiteDB
//   POST   /settings/backup/sqlite/upload   → UploadSQLiteDB

import (
	"archive/zip"
	"bytes"
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
	"isley/model"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Auth gating — every endpoint must reject unauthenticated traffic with
// either 401 (AuthMiddlewareApi) or 403 (CSRF middleware on POST/DELETE
// without the X-API-KEY bypass).
// ---------------------------------------------------------------------------

func TestBackupHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/settings/backup/create"},
		{http.MethodGet, "/settings/backup/status"},
		{http.MethodGet, "/settings/backup/list"},
		{http.MethodGet, "/settings/backup/download/foo.zip"},
		{http.MethodDelete, "/settings/backup/foo.zip"},
		{http.MethodPost, "/settings/backup/restore"},
		{http.MethodGet, "/settings/backup/restore/status"},
		{http.MethodGet, "/settings/backup/sqlite/download"},
		{http.MethodPost, "/settings/backup/sqlite/upload"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer testutil.DrainAndClose(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// CreateBackup
// ---------------------------------------------------------------------------

// TestBackupHTTP_CreateBackup_Accepted verifies the happy 202 path:
// CreateBackup spawns a goroutine and returns immediately. Because the
// engine now wires a per-test BackupService bound to a real *sql.DB,
// the goroutine actually runs and produces a backup file we can assert
// on disk — proving the create flow works end-to-end, not just that the
// status code is 202.
func TestBackupHTTP_CreateBackup_Accepted(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "create-backup-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/create", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	require.Equal(t, http.StatusAccepted, resp.StatusCode, "first CreateBackup should return 202")

	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body.Message, "202 response should carry a message")

	// Wait for the async backup to complete and assert a file was written.
	waitForBackupComplete(t, server.BackupService, 10*time.Second)

	files, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	require.NoError(t, err, "backups dir should have been created by the goroutine")
	require.Len(t, files, 1, "exactly one backup zip should be on disk")
	assert.Contains(t, files[0].Name(), "isley-backup-", "filename should follow the production prefix")
}

// TestBackupHTTP_CreateBackup_ConflictWhenInProgress verifies CreateBackup
// returns 409 when another backup is in flight. Flips the per-engine
// service's in-progress flag directly so we don't race a real goroutine.
func TestBackupHTTP_CreateBackup_ConflictWhenInProgress(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "create-conflict-key"
	testutil.SeedAPIKey(t, db, apiKey)

	server.BackupService.SetBackupInProgress(true)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/create", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"second CreateBackup while first is in flight should be 409")
}

// ---------------------------------------------------------------------------
// GetBackupStatus
// ---------------------------------------------------------------------------

func TestBackupHTTP_GetBackupStatus_ReturnsCurrent(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "status-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// Flip in-progress so we can assert the body actually reflects state.
	server.BackupService.SetBackupInProgress(true)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/status", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body handlers.BackupStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.InProgress, "in_progress should mirror the service state")
}

// ---------------------------------------------------------------------------
// ListBackups
// ---------------------------------------------------------------------------

// TestBackupHTTP_ListBackups_EmptyWhenNoDir verifies ListBackups gracefully
// returns [] when the per-engine backups directory does not exist. The
// per-test DataDir guarantees the test's tree has no backups regardless
// of what the dev machine carries on disk.
func TestBackupHTTP_ListBackups_EmptyWhenNoDir(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "list-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/list", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body, "no backups directory should yield empty array")
}

// TestBackupHTTP_ListBackups_ListsZipsWithMetadata writes a fake zip into
// the per-test data dir's backups subdirectory and asserts the response
// includes its metadata.
func TestBackupHTTP_ListBackups_ListsZipsWithMetadata(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755))
	const fixture = "isley-backup-fixture.zip"
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "backups", fixture), []byte("zip-bytes"), 0o644))
	// Non-zip files in the directory should be ignored by ListBackups.
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "backups", "ignore-me.txt"), []byte("nope"), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "list-zips-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/list", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body []handlers.BackupFileInfo
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 1, "non-zip files must be filtered out")
	assert.Equal(t, fixture, body[0].Name)
	assert.Equal(t, int64(len("zip-bytes")), body[0].Size)
	assert.NotEmpty(t, body[0].CreatedAt)
}

// ---------------------------------------------------------------------------
// DownloadBackup
// ---------------------------------------------------------------------------

func TestBackupHTTP_DownloadBackup_RejectsNonZipName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "dl-bad-name-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/notazip.txt", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"download of a non-.zip name should be rejected before disk I/O")
}

func TestBackupHTTP_DownloadBackup_NotFound(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "dl-missing-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/missing.zip", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"download of a non-existent .zip should be 404")
}

func TestBackupHTTP_DownloadBackup_HappyPath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755))

	const filename = "isley-backup-happy.zip"
	const payload = "fake-zip-payload"
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "backups", filename), []byte(payload), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "dl-happy-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/"+filename, apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), filename,
		"Content-Disposition must echo the requested filename")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body))
}

// ---------------------------------------------------------------------------
// DeleteBackup
// ---------------------------------------------------------------------------

func TestBackupHTTP_DeleteBackup_RejectsNonZipName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "del-bad-name-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/notazip.txt", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_DeleteBackup_NotFound(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "del-missing-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/missing.zip", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBackupHTTP_DeleteBackup_HappyPath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "backups"), 0o755))

	const filename = "isley-backup-todelete.zip"
	path := filepath.Join(dataDir, "backups", filename)
	require.NoError(t, os.WriteFile(path, []byte("doomed"), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(dataDir))

	const apiKey = "del-happy-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/"+filename, apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "file should be gone after DELETE")
}

// ---------------------------------------------------------------------------
// ImportBackup
// ---------------------------------------------------------------------------

func TestBackupHTTP_ImportBackup_RejectsMissingFileField(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "import-nofile-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// Multipart body with WRONG field name — handler looks for "backup".
	body, ct := testutil.MultipartBody(t, "wrong_field_name", "x.zip", []byte("anything"))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_ImportBackup_RejectsNonZipPayload(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "import-junk-key"
	testutil.SeedAPIKey(t, db, apiKey)

	body, ct := testutil.MultipartBody(t, "backup", "junk.zip", []byte("definitely not a zip"))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_RejectsZipWithoutManifest verifies a
// well-formed zip that lacks backup.json is rejected with 400.
func TestBackupHTTP_ImportBackup_RejectsZipWithoutManifest(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "import-no-manifest-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// Build a zip that contains files but no backup.json.
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("uploads/cat.png")
	require.NoError(t, err)
	_, err = w.Write([]byte("PNG..."))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	body, ct := testutil.MultipartBody(t, "backup", "no-manifest.zip", zipBuf.Bytes())

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_RejectsMalformedManifest verifies a zip
// whose backup.json is not valid JSON is rejected with 400.
func TestBackupHTTP_ImportBackup_RejectsMalformedManifest(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "import-bad-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, err = w.Write([]byte("not-json-{{{"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	body, ct := testutil.MultipartBody(t, "backup", "bad.zip", zipBuf.Bytes())

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_ConflictWhenRestoreInProgress verifies the
// 409 path. Sets the per-engine service's restore-in-progress flag
// directly so we don't depend on a real restore goroutine racing the
// second request.
func TestBackupHTTP_ImportBackup_ConflictWhenRestoreInProgress(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "import-conflict-key"
	testutil.SeedAPIKey(t, db, apiKey)

	server.BackupService.SetRestoreInProgress(true)

	body, ct := testutil.MultipartBody(t, "backup", "anything.zip", testutil.MustReadFixture(t, "backup/valid_minimal.zip"))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"import while restore in flight should be 409")
}

// ---------------------------------------------------------------------------
// GetRestoreStatus
// ---------------------------------------------------------------------------

func TestBackupHTTP_GetRestoreStatus_ReturnsCurrent(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "restore-status-key"
	testutil.SeedAPIKey(t, db, apiKey)

	server.BackupService.SetRestoreInProgress(true)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/restore/status", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body handlers.RestoreStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.InProgress, "in_progress should mirror the service state")
}

// ---------------------------------------------------------------------------
// DownloadSQLiteDB
// ---------------------------------------------------------------------------

// TestBackupHTTP_DownloadSQLiteDB_RejectsWhenPostgres flips the driver
// global to postgres for the duration of the test and asserts the
// endpoint returns 400 ("SQLite-only"). Restoring the driver in cleanup
// is essential — the global is shared with every other test in the
// process, which is why this test cannot be t.Parallel(). NewTestDB
// runs first so the testutil init-once block (which sets the driver to
// "sqlite") cannot overwrite our deliberate driver swap below.
func TestBackupHTTP_DownloadSQLiteDB_RejectsWhenPostgres(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	prev := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prev) })

	const apiKey = "sqlite-dl-pg-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"raw SQLite download must refuse when driver != sqlite")
}

// TestBackupHTTP_DownloadSQLiteDB_NotFound exercises the missing-file
// path. Sets ISLEY_DB_FILE to a path inside t.TempDir() that is
// guaranteed not to exist.
func TestBackupHTTP_DownloadSQLiteDB_NotFound(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	t.Setenv("ISLEY_DB_FILE", missing)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "sqlite-dl-missing-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBackupHTTP_DownloadSQLiteDB_HappyPath(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "isley.db")
	const payload = "SQLite format 3\x00rest-of-the-file"
	require.NoError(t, os.WriteFile(dbFile, []byte(payload), 0o644))
	t.Setenv("ISLEY_DB_FILE", dbFile)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "sqlite-dl-happy-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "isley-",
		"Content-Disposition should advertise an isley-* attachment filename")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body), "downloaded bytes should match the file on disk")
}

// ---------------------------------------------------------------------------
// UploadSQLiteDB
// ---------------------------------------------------------------------------

func TestBackupHTTP_UploadSQLiteDB_RejectsWhenPostgres(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	prev := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prev) })

	const apiKey = "sqlite-up-pg-key"
	testutil.SeedAPIKey(t, db, apiKey)

	body, ct := testutil.MultipartBody(t, "database", "fake.db", []byte("ignored"))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_UploadSQLiteDB_RejectsMissingFileField(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "sqlite-up-nofile-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// Wrong field name — handler looks for "database".
	body, ct := testutil.MultipartBody(t, "wrong_field", "fake.db", []byte("SQLite format 3\x00..."))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_UploadSQLiteDB_RejectsBadMagic verifies a payload with
// the wrong leading bytes is rejected with 400 before a goroutine runs.
func TestBackupHTTP_UploadSQLiteDB_RejectsBadMagic(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "sqlite-up-magic-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// 16 bytes that do NOT start with "SQLite format 3\x00".
	bad := []byte("NotASqliteHeader" + "...")
	body, ct := testutil.MultipartBody(t, "database", "fake.db", bad)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_UploadSQLiteDB_ConflictWhenRestoreInProgress(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithDataDir(t.TempDir()))

	const apiKey = "sqlite-up-conflict-key"
	testutil.SeedAPIKey(t, db, apiKey)

	server.BackupService.SetRestoreInProgress(true)

	// Body shape doesn't matter — the InProgress check fires before
	// the multipart parser is even consulted.
	body, ct := testutil.MultipartBody(t, "database", "fake.db", []byte("SQLite format 3\x00..."))

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Quick sanity: each helper-built apiReq uses the apiKey we passed.
// Caught in CI when a typo ("X-API-Key") drops auth silently.
// ---------------------------------------------------------------------------

func TestBackupHTTP_apiReqSetsHeader(t *testing.T) {
	t.Parallel()
	req := testutil.APIReq(t, http.MethodGet, "http://example/", "abc", nil, "")
	assert.Equal(t, "abc", req.Header.Get("X-API-KEY"))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// waitForBackupComplete polls the in-process BackupService until its
// in-progress flag flips back to false, or the timeout expires. Used by
// async-create tests that need to assert post-goroutine state on disk.
func waitForBackupComplete(t *testing.T, svc *handlers.BackupService, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status := svc.BackupSnapshot()
		if !status.InProgress {
			if status.Error != "" {
				t.Fatalf("backup goroutine reported error: %s", status.Error)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("backup did not complete within %s", timeout)
}
