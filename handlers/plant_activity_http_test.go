package handlers_test

// HTTP-layer tests for handlers/plant_activity.go that complement
// tests/integration/plant_activity_test.go. The integration suite
// covers create/edit/delete/multi happy paths plus a few validation
// branches; this file adds:
//
//   - auth gating across the four api-protected endpoints
//   - EditActivity bad-JSON branch
//   - DeleteActivity no-op behavior on missing id
//   - RecordMultiPlantActivity bad-JSON branch

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

func paAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func paReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func paDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlantActivityHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plantActivity"},
		{http.MethodPost, "/plantActivity/edit"},
		{http.MethodDelete, "/plantActivity/delete/1"},
		{http.MethodPost, "/record-multi-activity"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer paDrain(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// EditActivity / DeleteActivity / RecordMultiPlantActivity — error paths
// ---------------------------------------------------------------------------

func TestPlantActivityHTTP_Edit_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-edit-json-key"
	paAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(paReq(t, http.MethodPost, c.BaseURL+"/plantActivity/edit", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer paDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantActivityHTTP_Delete_NoOpOnMissing(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-del-key"
	paAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(paReq(t, http.MethodDelete, c.BaseURL+"/plantActivity/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer paDrain(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"deleting a non-existent activity is a no-op success")
}

func TestPlantActivityHTTP_Multi_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-multi-json-key"
	paAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(paReq(t, http.MethodPost, c.BaseURL+"/record-multi-activity", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer paDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantActivityHTTP_Create_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-create-json-key"
	paAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(paReq(t, http.MethodPost, c.BaseURL+"/plantActivity", apiKey,
		bytes.NewBufferString(`bogus`), "application/json"))
	require.NoError(t, err)
	defer paDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
