package handlers_test

// HTTP-layer tests for the 13 endpoints declared in handlers/settings.go.
// Phase 4b of docs/TEST_PLAN.md: each endpoint gets at least one happy
// path plus explicit auth/validation coverage. The handler under test
// retains its mutation of config.* globals — these tests deliberately do
// NOT assert on those globals to stay decoupled from cross-test order.
//
// Routes covered (all on the api-protected group):
//
//   POST   /settings                    → SaveSettings
//   POST   /settings/upload-logo        → UploadLogo
//   GET    /settings/logs               → GetLogs
//   GET    /settings/logs/download      → DownloadLogs
//   POST   /zones                       → AddZoneHandler
//   PUT    /zones/:id                   → UpdateZoneHandler
//   DELETE /zones/:id                   → DeleteZoneHandler
//   POST   /metrics                     → AddMetricHandler
//   PUT    /metrics/:id                 → UpdateMetricHandler
//   DELETE /metrics/:id                 → DeleteMetricHandler
//   POST   /activities                  → AddActivityHandler
//   PUT    /activities/:id              → UpdateActivityHandler
//   DELETE /activities/:id              → DeleteActivityHandler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Helpers (file-local; this test file is self-contained on purpose).
// ---------------------------------------------------------------------------

func settingsAPIKey(t *testing.T, db *sql.DB, plaintext string) {
	t.Helper()
	hashed := handlers.HashAPIKey(plaintext)
	var id int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = 'api_key'`).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(`INSERT INTO settings (name, value) VALUES ('api_key', $1)`, hashed)
	case err == nil:
		_, err = db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, hashed, id)
	}
	require.NoError(t, err)
}

// settingsReq builds a request with X-API-KEY plus optional content type.
func settingsReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

// jsonBody wraps a value as a JSON request body. Failing to encode the
// fixture is a test-author error, so it terminates the test.
func jsonBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(v))
	return &buf
}

func drainResp(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// pngFixture returns the bytes of a 1×1 PNG, used for the upload-logo
// happy path.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, image.White)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Auth gating (table-driven — every endpoint rejects unauthenticated)
// ---------------------------------------------------------------------------

func TestSettingsHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/settings"},
		{http.MethodPost, "/settings/upload-logo"},
		{http.MethodGet, "/settings/logs"},
		{http.MethodGet, "/settings/logs/download"},
		{http.MethodPost, "/zones"},
		{http.MethodPut, "/zones/1"},
		{http.MethodDelete, "/zones/1"},
		{http.MethodPost, "/metrics"},
		{http.MethodPut, "/metrics/1"},
		{http.MethodDelete, "/metrics/1"},
		{http.MethodPost, "/activities"},
		{http.MethodPut, "/activities/1"},
		{http.MethodDelete, "/activities/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer drainResp(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// SaveSettings (POST /settings)
// ---------------------------------------------------------------------------

func TestSettingsHTTP_SaveSettings_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "save-settings-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]interface{}{
		"polling_interval":      "60",
		"stream_grab_interval":  "30000",
		"sensor_retention_days": "90",
		"log_level":             "info",
		"timezone":              "America/Los_Angeles",
		"guest_mode":            false,
	})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// DB-level: each persisted key should now be readable.
	for _, k := range []string{"polling_interval", "stream_grab_interval", "sensor_retention_days", "log_level", "timezone"} {
		var v string
		err := db.QueryRow(`SELECT value FROM settings WHERE name = $1`, k).Scan(&v)
		require.NoErrorf(t, err, "setting %q must be persisted", k)
		assert.NotEmptyf(t, v, "setting %q should not be empty after save", k)
	}
}

func TestSettingsHTTP_SaveSettings_RejectsMalformedJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "save-bad-json-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := bytes.NewBufferString(`{"polling_interval": ` /* no closing brace */)
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSettingsHTTP_SaveSettings_GenerateAPIKeyReturnsPlaintext(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "save-genkey-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]interface{}{"api_key": "generate"})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Message string `json:"message"`
		APIKey  string `json:"api_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got.APIKey, 32, "generated key should be a 32-char hex string")

	// The DB stores a *hashed* form — never the plaintext.
	var stored string
	require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE name = 'api_key'`).Scan(&stored))
	assert.NotEqual(t, got.APIKey, stored, "DB must store the hash, not plaintext")
}

// ---------------------------------------------------------------------------
// AddZoneHandler / UpdateZoneHandler / DeleteZoneHandler
// ---------------------------------------------------------------------------

func TestSettingsHTTP_AddZone_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addzone-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"zone_name": "Tent A"})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/zones", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Greater(t, got.ID, 0)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM zones WHERE id = $1`, got.ID).Scan(&name))
	assert.Equal(t, "Tent A", name)
}

func TestSettingsHTTP_AddZone_RejectsBlankName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addzone-blank-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"zone_name": ""})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/zones", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSettingsHTTP_UpdateZone_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updzone-key"
	settingsAPIKey(t, db, apiKey)

	res, err := db.Exec(`INSERT INTO zones (name) VALUES ('Old')`)
	require.NoError(t, err)
	zoneID, _ := res.LastInsertId()

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"zone_name": "New"})
	resp, err := c.Do(settingsReq(t, http.MethodPut, c.BaseURL+"/zones/"+strconv.FormatInt(zoneID, 10), apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM zones WHERE id = $1`, zoneID).Scan(&name))
	assert.Equal(t, "New", name)
}

func TestSettingsHTTP_UpdateZone_RejectsBlankName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updzone-blank-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"zone_name": ""})
	resp, err := c.Do(settingsReq(t, http.MethodPut, c.BaseURL+"/zones/1", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSettingsHTTP_DeleteZone_RemovesRow(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "delzone-key"
	settingsAPIKey(t, db, apiKey)

	res, err := db.Exec(`INSERT INTO zones (name) VALUES ('Doomed')`)
	require.NoError(t, err)
	zoneID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodDelete, c.BaseURL+"/zones/"+strconv.FormatInt(zoneID, 10), apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM zones WHERE id = $1`, zoneID).Scan(&n))
	assert.Zero(t, n, "zone row should be gone")
}

// ---------------------------------------------------------------------------
// AddMetricHandler / UpdateMetricHandler / DeleteMetricHandler
// ---------------------------------------------------------------------------

func TestSettingsHTTP_AddMetric_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addmet-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"metric_name": "pH", "metric_unit": ""})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/metrics", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestSettingsHTTP_AddMetric_RejectsBlankName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addmet-blank-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"metric_name": "", "metric_unit": "in"})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/metrics", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSettingsHTTP_UpdateMetric_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updmet-key"
	settingsAPIKey(t, db, apiKey)

	res, err := db.Exec(`INSERT INTO metric (name, unit, lock) VALUES ('Old', 'cm', 0)`)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"metric_name": "Renamed", "metric_unit": "in"})
	resp, err := c.Do(settingsReq(t, http.MethodPut, c.BaseURL+"/metrics/"+strconv.FormatInt(id, 10), apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name, unit string
	require.NoError(t, db.QueryRow(`SELECT name, unit FROM metric WHERE id = $1`, id).Scan(&name, &unit))
	assert.Equal(t, "Renamed", name)
	assert.Equal(t, "in", unit)
}

// TestSettingsHTTP_DeleteMetric_RejectsLocked verifies a metric whose
// lock=true bit is set cannot be deleted (the only branch in the handler
// that returns 400). The default seeded "Height" metric is locked.
func TestSettingsHTTP_DeleteMetric_RejectsLocked(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "delmet-locked-key"
	settingsAPIKey(t, db, apiKey)

	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name = 'Height'`).Scan(&id),
		"Height metric must be present in default seed")

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodDelete, c.BaseURL+"/metrics/"+strconv.Itoa(id), apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"locked metric must be 400, not deleted")

	// And the row is still there.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM metric WHERE id = $1`, id).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestSettingsHTTP_DeleteMetric_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "delmet-key"
	settingsAPIKey(t, db, apiKey)

	// Insert a non-locked metric.
	res, err := db.Exec(`INSERT INTO metric (name, unit, lock) VALUES ('Width', 'cm', 0)`)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodDelete, c.BaseURL+"/metrics/"+strconv.FormatInt(id, 10), apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM metric WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// AddActivityHandler / UpdateActivityHandler / DeleteActivityHandler
// ---------------------------------------------------------------------------

func TestSettingsHTTP_AddActivity_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addact-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"activity_name": "Prune"})
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/activities", apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// TestSettingsHTTP_AddActivity_RejectsReservedName verifies the handler
// blocks the three reserved built-in names ("Water", "Feed", "Note").
func TestSettingsHTTP_AddActivity_RejectsReservedName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addact-reserved-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	for _, name := range []string{"Water", "Feed", "Note"} {
		t.Run(name, func(t *testing.T) {
			body := jsonBody(t, map[string]string{"activity_name": name})
			resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/activities", apiKey, body, "application/json"))
			require.NoError(t, err)
			defer drainResp(resp)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"reserved activity name %q must be rejected", name)
		})
	}
}

func TestSettingsHTTP_UpdateActivity_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updact-key"
	settingsAPIKey(t, db, apiKey)

	res, err := db.Exec(`INSERT INTO activity (name, lock) VALUES ('Old', 0)`)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	body := jsonBody(t, map[string]string{"activity_name": "New"})
	resp, err := c.Do(settingsReq(t, http.MethodPut, c.BaseURL+"/activities/"+strconv.FormatInt(id, 10), apiKey, body, "application/json"))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM activity WHERE id = $1`, id).Scan(&name))
	assert.Equal(t, "New", name)
}

func TestSettingsHTTP_DeleteActivity_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "delact-key"
	settingsAPIKey(t, db, apiKey)

	res, err := db.Exec(`INSERT INTO activity (name, lock) VALUES ('Doomed', 0)`)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodDelete, c.BaseURL+"/activities/"+strconv.FormatInt(id, 10), apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// UploadLogo (POST /settings/upload-logo)
//
// UploadLogo writes into ./uploads/logos relative to CWD. Tests use
// t.Chdir to an isolated temp dir so they neither require nor pollute
// the repo's uploads/ tree.
// ---------------------------------------------------------------------------

func TestSettingsHTTP_UploadLogo_HappyPath(t *testing.T) {
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logo-happy-key"
	settingsAPIKey(t, db, apiKey)

	body, ct := buildLogoMultipart(t, "logo.png", pngFixture(t))

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings/upload-logo", apiKey, body, ct))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// A file was written under uploads/logos/.
	entries, err := os.ReadDir(filepath.Join("uploads", "logos"))
	require.NoError(t, err)
	require.NotEmpty(t, entries, "uploads/logos must contain the new logo file")
}

func TestSettingsHTTP_UploadLogo_RejectsNonImage(t *testing.T) {
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logo-bad-key"
	settingsAPIKey(t, db, apiKey)

	// 600 bytes of 0x00 → DetectContentType returns application/octet-stream.
	body, ct := buildLogoMultipart(t, "not-an-image.png", make([]byte, 600))

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings/upload-logo", apiKey, body, ct))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-image MIME types must be rejected before disk write")
}

func TestSettingsHTTP_UploadLogo_RejectsMissingField(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logo-nofield-key"
	settingsAPIKey(t, db, apiKey)

	// Multipart body with the wrong field name.
	body, ct := buildLogoMultipart(t, "x.png", pngFixture(t))
	// Replace 'logo' field name with 'wrong'. Easiest: rebuild from scratch.
	body, ct = buildLogoMultipartCustomField(t, "wrong", "x.png", pngFixture(t))

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodPost, c.BaseURL+"/settings/upload-logo", apiKey, body, ct))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func buildLogoMultipart(t *testing.T, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	return buildLogoMultipartCustomField(t, "logo", filename, payload)
}

func buildLogoMultipartCustomField(t *testing.T, fieldName, filename string, payload []byte) (*bytes.Buffer, string) {
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

// ---------------------------------------------------------------------------
// GetLogs (GET /settings/logs) and DownloadLogs (GET /settings/logs/download)
//
// Both read from logs/app.log or logs/access.log relative to CWD. Tests
// use t.Chdir to an isolated temp dir to make the fixture deterministic.
// ---------------------------------------------------------------------------

func TestSettingsHTTP_GetLogs_HappyPath(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll("logs", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("logs", "app.log"), []byte("first\nsecond\nthird\n"), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logs-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodGet, c.BaseURL+"/settings/logs?lines=10", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Lines string `json:"lines"`
		Total int    `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Contains(t, got.Lines, "first")
	assert.Contains(t, got.Lines, "third")
}

func TestSettingsHTTP_GetLogs_FileMissingReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	// No logs/ directory at all.

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logs-missing-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodGet, c.BaseURL+"/settings/logs", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"missing log file currently surfaces as 500 (apiInternalError)")
}

func TestSettingsHTTP_DownloadLogs_HappyPath(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll("logs", 0o755))
	const payload = "log-line-one\nlog-line-two\n"
	require.NoError(t, os.WriteFile(filepath.Join("logs", "app.log"), []byte(payload), 0o644))

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logdl-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodGet, c.BaseURL+"/settings/logs/download", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "app.log")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, payload, string(body))
}

func TestSettingsHTTP_DownloadLogs_FileMissingReturns404(t *testing.T) {
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "logdl-missing-key"
	settingsAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(settingsReq(t, http.MethodGet, c.BaseURL+"/settings/logs/download?file=access", apiKey, nil, ""))
	require.NoError(t, err)
	defer drainResp(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
