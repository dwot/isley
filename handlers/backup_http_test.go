package handlers_test

// HTTP-layer tests for the nine backup endpoints in handlers/backup.go.
// Phase 4b of docs/TEST_PLAN.md: every handler gets at least one happy
// path plus explicit auth/validation/error coverage. Repo extraction is
// not required at this layer — these tests drive the live HTTP surface
// through testutil.NewTestServer.
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
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/model"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// File-local helpers — duplicated from tests/integration/auth_test.go so
// this test file stays self-contained and doesn't import from a peer test
// package (which Go forbids).
// ---------------------------------------------------------------------------

// upsertBackupSetting writes a name/value row to the settings table,
// replacing any prior row with the same name.
func upsertBackupSetting(t *testing.T, db *sql.DB, name, value string) {
	t.Helper()
	var existingID int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = $1`, name).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(`INSERT INTO settings (name, value) VALUES ($1, $2)`, name, value)
	case err == nil:
		_, err = db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, value, existingID)
	}
	require.NoError(t, err)
}

// seedBackupAPIKey installs a hashed api_key row so X-API-KEY satisfies
// AuthMiddlewareApi without going through the login flow.
func seedBackupAPIKey(t *testing.T, db *sql.DB, plaintext string) {
	t.Helper()
	upsertBackupSetting(t, db, "api_key", handlers.HashAPIKey(plaintext))
}

// apiReq builds a request with the X-API-KEY header set, ready to send
// through testutil.Client.Do. Body may be nil.
func apiReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	require.NoError(t, err)
	if apiKey != "" {
		req.Header.Set("X-API-KEY", apiKey)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

// drainAndClose closes the response body to avoid leaked file descriptors
// when the test does not need to read it.
func drainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// resetBackupGlobals clears the singletons before AND after each test so
// goroutines spawned by an earlier test (or this test) cannot leak into
// the next one. Tests that mutate currentBackup/currentRestore must call
// this in t.Cleanup.
func resetBackupGlobals(t *testing.T) {
	t.Helper()
	handlers.ResetBackupStatusForTesting()
	handlers.ResetRestoreStatusForTesting()
	t.Cleanup(func() {
		handlers.ResetBackupStatusForTesting()
		handlers.ResetRestoreStatusForTesting()
	})
}

// buildMultipartBody builds a multipart/form-data body with one file
// part. Returned contentType includes the boundary header.
func buildMultipartBody(t *testing.T, fieldName, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

// validBackupZip returns the raw bytes of a minimal valid zip archive
// containing a backup.json file with an empty payload. Lets ImportBackup
// progress past the zip-parse and JSON-decode validations.
func validBackupZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, err = w.Write([]byte(`{"manifest":{"version":"test","driver":"sqlite","tables":0,"files":0}}`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Auth gating — every endpoint must reject unauthenticated traffic with
// either 401 (AuthMiddlewareApi) or 403 (CSRF middleware on POST/DELETE
// without the X-API-KEY bypass).
// ---------------------------------------------------------------------------

func TestBackupHTTP_AuthGating(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

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
			defer drainAndClose(resp)
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
// CreateBackup spawns a goroutine and returns immediately. The goroutine
// itself fails harmlessly because model.GetDB() returns nil in tests
// (handlers.BuildBackupArchive guards against this and returns an error),
// so no files are written.
func TestBackupHTTP_CreateBackup_Accepted(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "create-backup-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/create", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)

	assert.Equal(t, http.StatusAccepted, resp.StatusCode, "first CreateBackup should return 202")

	var body struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body.Message, "202 response should carry a message")
}

// TestBackupHTTP_CreateBackup_ConflictWhenInProgress verifies CreateBackup
// returns 409 when another backup is in flight. Uses
// SetBackupInProgressForTesting to flip the singleton without spawning a
// real goroutine.
func TestBackupHTTP_CreateBackup_ConflictWhenInProgress(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "create-conflict-key"
	seedBackupAPIKey(t, db, apiKey)

	handlers.SetBackupInProgressForTesting()

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/create", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"second CreateBackup while first is in flight should be 409")
}

// ---------------------------------------------------------------------------
// GetBackupStatus
// ---------------------------------------------------------------------------

func TestBackupHTTP_GetBackupStatus_ReturnsCurrent(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "status-key"
	seedBackupAPIKey(t, db, apiKey)

	// Flip in-progress so we can assert the body actually reflects state.
	handlers.SetBackupInProgressForTesting()

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/status", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body handlers.BackupStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.InProgress, "in_progress should mirror the singleton")
}

// ---------------------------------------------------------------------------
// ListBackups
// ---------------------------------------------------------------------------

// TestBackupHTTP_ListBackups_EmptyWhenNoDir verifies ListBackups gracefully
// returns [] when data/backups does not exist (or contains no zips). It
// must NOT 500.
func TestBackupHTTP_ListBackups_EmptyWhenNoDir(t *testing.T) {
	resetBackupGlobals(t)
	// Working directory does not contain a data/backups under our test
	// process — but on dev machines it might. Either way, the response
	// shape must be an array and the status must be 200.

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "list-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/list", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Body is a JSON array of objects (possibly empty).
	var body []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	// Either empty or populated; we don't assert content because the dev
	// repo's data/backups can hold real backups. The contract is "no 500".
	_ = body
}

// TestBackupHTTP_ListBackups_ListsZipsWithMetadata writes a fake zip into
// a per-test data/backups directory (via t.Chdir to a temp dir) and
// asserts the response includes its metadata. t.Chdir is safe here
// because the test does not call t.Parallel and Go runs sequential tests
// strictly before parallel ones in the same package.
func TestBackupHTTP_ListBackups_ListsZipsWithMetadata(t *testing.T) {
	resetBackupGlobals(t)

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "backups"), 0o755))
	const fixture = "isley-backup-fixture.zip"
	require.NoError(t, os.WriteFile(filepath.Join("data", "backups", fixture), []byte("zip-bytes"), 0o644))
	// Non-zip files in the directory should be ignored by ListBackups.
	require.NoError(t, os.WriteFile(filepath.Join("data", "backups", "ignore-me.txt"), []byte("nope"), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "list-zips-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/list", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
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
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "dl-bad-name-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/notazip.txt", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"download of a non-.zip name should be rejected before disk I/O")
}

func TestBackupHTTP_DownloadBackup_NotFound(t *testing.T) {
	resetBackupGlobals(t)

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "backups"), 0o755))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "dl-missing-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/missing.zip", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"download of a non-existent .zip should be 404")
}

func TestBackupHTTP_DownloadBackup_HappyPath(t *testing.T) {
	resetBackupGlobals(t)

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "backups"), 0o755))

	const filename = "isley-backup-happy.zip"
	const payload = "fake-zip-payload"
	require.NoError(t, os.WriteFile(filepath.Join("data", "backups", filename), []byte(payload), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "dl-happy-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/download/"+filename, apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)

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
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "del-bad-name-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/notazip.txt", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_DeleteBackup_NotFound(t *testing.T) {
	resetBackupGlobals(t)

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "backups"), 0o755))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "del-missing-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/missing.zip", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBackupHTTP_DeleteBackup_HappyPath(t *testing.T) {
	resetBackupGlobals(t)

	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join("data", "backups"), 0o755))

	const filename = "isley-backup-todelete.zip"
	path := filepath.Join("data", "backups", filename)
	require.NoError(t, os.WriteFile(path, []byte("doomed"), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "del-happy-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodDelete, c.BaseURL+"/settings/backup/"+filename, apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "file should be gone after DELETE")
}

// ---------------------------------------------------------------------------
// ImportBackup
// ---------------------------------------------------------------------------

func TestBackupHTTP_ImportBackup_RejectsMissingFileField(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "import-nofile-key"
	seedBackupAPIKey(t, db, apiKey)

	// Multipart body with WRONG field name — handler looks for "backup".
	body, ct := buildMultipartBody(t, "wrong_field_name", "x.zip", []byte("anything"))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_ImportBackup_RejectsNonZipPayload(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "import-junk-key"
	seedBackupAPIKey(t, db, apiKey)

	body, ct := buildMultipartBody(t, "backup", "junk.zip", []byte("definitely not a zip"))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_RejectsZipWithoutManifest verifies a
// well-formed zip that lacks backup.json is rejected with 400.
func TestBackupHTTP_ImportBackup_RejectsZipWithoutManifest(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "import-no-manifest-key"
	seedBackupAPIKey(t, db, apiKey)

	// Build a zip that contains files but no backup.json.
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("uploads/cat.png")
	require.NoError(t, err)
	_, err = w.Write([]byte("PNG..."))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	body, ct := buildMultipartBody(t, "backup", "no-manifest.zip", zipBuf.Bytes())

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_RejectsMalformedManifest verifies a zip
// whose backup.json is not valid JSON is rejected with 400.
func TestBackupHTTP_ImportBackup_RejectsMalformedManifest(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "import-bad-json-key"
	seedBackupAPIKey(t, db, apiKey)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, err = w.Write([]byte("not-json-{{{"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	body, ct := buildMultipartBody(t, "backup", "bad.zip", zipBuf.Bytes())

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_ImportBackup_ConflictWhenRestoreInProgress verifies the
// 409 path. Sets currentRestore.InProgress directly so we don't depend on
// a real restore goroutine racing the second request.
func TestBackupHTTP_ImportBackup_ConflictWhenRestoreInProgress(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "import-conflict-key"
	seedBackupAPIKey(t, db, apiKey)

	handlers.SetRestoreInProgressForTesting()

	body, ct := buildMultipartBody(t, "backup", "anything.zip", validBackupZip(t))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"import while restore in flight should be 409")
}

// ---------------------------------------------------------------------------
// GetRestoreStatus
// ---------------------------------------------------------------------------

func TestBackupHTTP_GetRestoreStatus_ReturnsCurrent(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "restore-status-key"
	seedBackupAPIKey(t, db, apiKey)

	handlers.SetRestoreInProgressForTesting()

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/restore/status", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body handlers.RestoreStatus
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.InProgress, "in_progress should mirror the singleton")
}

// ---------------------------------------------------------------------------
// DownloadSQLiteDB
// ---------------------------------------------------------------------------

// TestBackupHTTP_DownloadSQLiteDB_RejectsWhenPostgres flips the driver
// global to postgres for the duration of the test and asserts the
// endpoint returns 400 ("SQLite-only"). Restoring the driver in cleanup
// is essential — the global is shared with every other test in the
// process.
func TestBackupHTTP_DownloadSQLiteDB_RejectsWhenPostgres(t *testing.T) {
	resetBackupGlobals(t)

	prev := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prev) })

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-dl-pg-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"raw SQLite download must refuse when driver != sqlite")
}

// TestBackupHTTP_DownloadSQLiteDB_NotFound exercises the missing-file
// path. Sets ISLEY_DB_FILE to a path inside t.TempDir() that is
// guaranteed not to exist.
func TestBackupHTTP_DownloadSQLiteDB_NotFound(t *testing.T) {
	resetBackupGlobals(t)

	missing := filepath.Join(t.TempDir(), "missing.db")
	t.Setenv("ISLEY_DB_FILE", missing)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-dl-missing-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBackupHTTP_DownloadSQLiteDB_HappyPath(t *testing.T) {
	resetBackupGlobals(t)

	dbFile := filepath.Join(t.TempDir(), "isley.db")
	const payload = "SQLite format 3\x00rest-of-the-file"
	require.NoError(t, os.WriteFile(dbFile, []byte(payload), 0o644))
	t.Setenv("ISLEY_DB_FILE", dbFile)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-dl-happy-key"
	seedBackupAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodGet, c.BaseURL+"/settings/backup/sqlite/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainAndClose(resp)

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
	resetBackupGlobals(t)

	prev := model.GetDriver()
	model.SetDriverForTesting("postgres")
	t.Cleanup(func() { model.SetDriverForTesting(prev) })

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-up-pg-key"
	seedBackupAPIKey(t, db, apiKey)

	body, ct := buildMultipartBody(t, "database", "fake.db", []byte("ignored"))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_UploadSQLiteDB_RejectsMissingFileField(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-up-nofile-key"
	seedBackupAPIKey(t, db, apiKey)

	// Wrong field name — handler looks for "database".
	body, ct := buildMultipartBody(t, "wrong_field", "fake.db", []byte("SQLite format 3\x00..."))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestBackupHTTP_UploadSQLiteDB_RejectsBadMagic verifies a payload with
// the wrong leading bytes is rejected with 400 before a goroutine runs.
func TestBackupHTTP_UploadSQLiteDB_RejectsBadMagic(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-up-magic-key"
	seedBackupAPIKey(t, db, apiKey)

	// 16 bytes that do NOT start with "SQLite format 3\x00".
	bad := []byte("NotASqliteHeader" + "...")
	body, ct := buildMultipartBody(t, "database", "fake.db", bad)

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBackupHTTP_UploadSQLiteDB_ConflictWhenRestoreInProgress(t *testing.T) {
	resetBackupGlobals(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "sqlite-up-conflict-key"
	seedBackupAPIKey(t, db, apiKey)

	handlers.SetRestoreInProgressForTesting()

	// Body shape doesn't matter — the InProgress check fires before
	// the multipart parser is even consulted.
	body, ct := buildMultipartBody(t, "database", "fake.db", []byte("SQLite format 3\x00..."))

	c := server.NewClient(t)
	resp, err := c.Do(apiReq(t, http.MethodPost, c.BaseURL+"/settings/backup/sqlite/upload", apiKey, body, ct))
	require.NoError(t, err)
	defer drainAndClose(resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Quick sanity: each helper-built apiReq uses the apiKey we passed.
// Caught in CI when a typo ("X-API-Key") drops auth silently.
// ---------------------------------------------------------------------------

func TestBackupHTTP_apiReqSetsHeader(t *testing.T) {
	t.Parallel()
	req := apiReq(t, http.MethodGet, "http://example/", "abc", nil, "")
	assert.Equal(t, "abc", req.Header.Get("X-API-KEY"))
}
