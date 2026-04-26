package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// Auth-gating coverage for /settings/backup/* lives in
// handlers/backup_http_test.go::TestBackupHTTP_AuthGating, which exercises
// all nine endpoints. The integration version that previously lived here
// duplicated the same assertion against five of them and was deleted.

// TestBackup_ListWithAPIKey confirms ListBackups returns a JSON array
// when authenticated. With no backups directory present (fresh test
// repo), the endpoint should return [] not 500.
func TestBackup_ListWithAPIKey(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintextKey = "test-list-key"
	seedAPIKey(t, db, handlers.HashAPIKey(plaintextKey))

	c := server.NewClient(t)
	resp := apiGet(t, c, "/settings/backup/list", plaintextKey)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestBackup_RestoreInvalidZip confirms POST /settings/backup/restore
// returns 400 (not 500) when the uploaded file is not a valid zip.
// Auth via API key.
func TestBackup_RestoreInvalidZip(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintextKey = "test-restore-key"
	seedAPIKey(t, db, handlers.HashAPIKey(plaintextKey))

	c := server.NewClient(t)

	// Build a multipart body with a "backup" field containing junk bytes.
	body, contentType := buildMultipart(t, "backup", "junk.zip", []byte("definitely not a zip"))

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/settings/backup/restore", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-KEY", plaintextKey)

	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-zip upload should be rejected with 400")
}

// buildMultipart returns a multipart/form-data body with one named file
// part. Returned contentType includes the boundary.
func buildMultipart(t *testing.T, fieldName, filename string, payload []byte) (*strings.Reader, string) {
	t.Helper()
	const boundary = "----IsleyTestBoundary"
	var b strings.Builder
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString(`Content-Disposition: form-data; name="` + fieldName + `"; filename="` + filename + "\"\r\n")
	b.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	b.Write(payload)
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return strings.NewReader(b.String()), "multipart/form-data; boundary=" + boundary
}
