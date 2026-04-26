package handlers_test

// HTTP-layer tests for handlers/plant_status_update.go (UpdatePlantStatus
// at POST /plant/status). The integration suite already covers the
// happy path and "no-op when same status" branch; this file fills in
// auth gating and the missing-field branches.

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func psuAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func psuReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func psuDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlantStatusUpdateHTTP_RequiresAuth(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/plant/status", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer psuDrain(resp)
	assert.Containsf(t,
		[]int{http.StatusUnauthorized, http.StatusForbidden},
		resp.StatusCode,
		"unauthenticated POST /plant/status should be rejected (got %d)", resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Validation branches
// ---------------------------------------------------------------------------

func TestPlantStatusUpdateHTTP_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-json-key"
	psuAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(psuReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer psuDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantStatusUpdateHTTP_RejectsMissingPlantID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-missing-pid-key"
	psuAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(psuReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`{"status_id":1}`), "application/json"))
	require.NoError(t, err)
	defer psuDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantStatusUpdateHTTP_RejectsMissingStatusID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-missing-sid-key"
	psuAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(psuReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`{"plant_id":1}`), "application/json"))
	require.NoError(t, err)
	defer psuDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
