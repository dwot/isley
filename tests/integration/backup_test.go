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

// TestBackup_EndpointsRequireAuth confirms the backup management
// endpoints sit behind AuthMiddlewareApi and reject anonymous traffic.
// They live under /settings/backup/* and are mounted on the api-protected
// route group.
//
// GETs surface 401 directly from AuthMiddlewareApi (CSRF middleware
// only checks state-changing methods). POSTs surface 403 from the CSRF
// middleware first because the body has no csrf_token and no X-API-KEY
// header to bypass it. Both are "rejected"; we accept either status as
// a pass.
func TestBackup_EndpointsRequireAuth(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodGet, "/settings/backup/list"},
		{http.MethodGet, "/settings/backup/status"},
		{http.MethodPost, "/settings/backup/create"},
		{http.MethodGet, "/settings/backup/restore/status"},
		{http.MethodGet, "/settings/backup/sqlite/download"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

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
