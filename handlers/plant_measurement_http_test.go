package handlers_test

// HTTP-layer tests for handlers/plant_measurement.go that complement
// tests/integration/plant_measurement_test.go.

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

func pmAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func pmReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func pmDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlantMeasurementHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plantMeasurement"},
		{http.MethodPost, "/plantMeasurement/edit"},
		{http.MethodDelete, "/plantMeasurement/delete/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer pmDrain(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// Validation branches
// ---------------------------------------------------------------------------

func TestPlantMeasurementHTTP_Create_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-create-json-key"
	pmAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(pmReq(t, http.MethodPost, c.BaseURL+"/plantMeasurement", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer pmDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantMeasurementHTTP_Edit_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-edit-json-key"
	pmAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(pmReq(t, http.MethodPost, c.BaseURL+"/plantMeasurement/edit", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer pmDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantMeasurementHTTP_Delete_NoOpOnMissing(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-del-key"
	pmAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(pmReq(t, http.MethodDelete, c.BaseURL+"/plantMeasurement/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer pmDrain(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
