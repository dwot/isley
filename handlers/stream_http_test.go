package handlers_test

// HTTP-layer tests for handlers/stream.go beyond what
// tests/integration/stream_test.go already covers.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func streamAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func streamReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func streamJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(v))
	return &buf
}

func streamDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestStreamHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/streams"},
		{http.MethodPut, "/streams/1"},
		{http.MethodDelete, "/streams/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer streamDrain(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// AddStreamHandler — additional validation paths
// ---------------------------------------------------------------------------

func TestStreamHTTP_Add_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-add-json-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodPost, c.BaseURL+"/streams", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStreamHTTP_Add_RejectsLongName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-add-long-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodPost, c.BaseURL+"/streams", apiKey,
		streamJSON(t, map[string]interface{}{
			"stream_name": strings.Repeat("s", 1024),
			"url":         "https://example.com/stream",
		}), "application/json"))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdateStreamHandler — validation
// ---------------------------------------------------------------------------

func TestStreamHTTP_Update_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-upd-json-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodPut, c.BaseURL+"/streams/1", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStreamHTTP_Update_RejectsBlankName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-upd-blank-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodPut, c.BaseURL+"/streams/1", apiKey,
		streamJSON(t, map[string]interface{}{
			"stream_name": "",
			"url":         "https://example.com/stream",
		}), "application/json"))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStreamHTTP_Update_RejectsBadURL(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-upd-url-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodPut, c.BaseURL+"/streams/1", apiKey,
		streamJSON(t, map[string]interface{}{
			"stream_name": "Stream",
			"url":         "not a valid url",
		}), "application/json"))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeleteStreamHandler — no-op when row missing
// ---------------------------------------------------------------------------

func TestStreamHTTP_Delete_NoOpOnMissing(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "stream-del-key"
	streamAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(streamReq(t, http.MethodDelete, c.BaseURL+"/streams/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer streamDrain(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GetStreamsByZoneHandler — public read
// ---------------------------------------------------------------------------

func TestStreamHTTP_GetByZone_EmptyReturnsObject(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/streams")
	defer streamDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(body, []byte("{")), "empty streamsByZone should serialise as {}")
}
