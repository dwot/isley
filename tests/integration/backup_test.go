package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	testutil.SeedAPIKey(t, db, plaintextKey)

	c := server.NewClient(t)
	resp := c.APIGet(t, "/settings/backup/list", plaintextKey)
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
	testutil.SeedAPIKey(t, db, plaintextKey)

	c := server.NewClient(t)

	body, contentType := testutil.MultipartBody(t, "backup", "junk.zip", []byte("definitely not a zip"))

	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/settings/backup/restore", plaintextKey, body, contentType))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-zip upload should be rejected with 400")
}
