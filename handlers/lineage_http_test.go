package handlers_test

// HTTP-layer tests for handlers/lineage.go that complement
// tests/integration/lineage_test.go. The integration suite already
// covers the handlers' main scenarios in depth; this file fills in:
//
//   - auth gating across the api-protected lineage write endpoints
//   - non-numeric :id / :lineageID branches that integration tests do
//     not consistently exercise
//   - bad-JSON branches on Add/Update/SetLineage
//   - public-read tests that need no auth (GetLineage, GetDescendants,
//     LookupStrainByName)
//
// Routes:
//
//   POST   /strains/:id/lineage   (api-protected) → AddLineageHandler
//   PUT    /strains/:id/lineage   (api-protected) → SetLineageHandler
//   PUT    /lineage/:lineageID    (api-protected) → UpdateLineageHandler
//   DELETE /lineage/:lineageID    (api-protected) → DeleteLineageHandler
//   GET    /strains/:id/lineage     (basic)       → GetLineageHandler
//   GET    /strains/:id/descendants (basic)       → GetDescendantsHandler
//   GET    /strains/lookup          (basic)       → LookupStrainByName

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func lineageAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func lineageReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func lineageJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(v))
	return &buf
}

func lineageDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func lineageSeedStrains(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec := func(q string, args ...interface{}) {
		_, err := db.Exec(q, args...)
		require.NoErrorf(t, err, "seed: %s", q)
	}
	mustExec(`INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	mustExec(`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	          VALUES (1, 'Child Strain', 1, 50, 50, 0, '', 0),
	                 (2, 'Parent Strain', 1, 50, 50, 0, '', 0)`)
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestLineageHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/strains/1/lineage"},
		{http.MethodPut, "/strains/1/lineage"},
		{http.MethodPut, "/lineage/1"},
		{http.MethodDelete, "/lineage/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer lineageDrain(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// AddLineageHandler — non-numeric :id and bad-JSON branches
// ---------------------------------------------------------------------------

func TestLineageHTTP_Add_RejectsNonNumericStrainID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-id-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPost, c.BaseURL+"/strains/abc/lineage", apiKey,
		lineageJSON(t, map[string]interface{}{"parent_name": "X"}), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Add_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-json-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPost, c.BaseURL+"/strains/1/lineage", apiKey,
		bytes.NewBufferString(`{not json`), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Add_RejectsLongParentName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-long-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPost, c.BaseURL+"/strains/1/lineage", apiKey,
		lineageJSON(t, map[string]interface{}{"parent_name": strings.Repeat("p", 1024)}), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdateLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Update_RejectsNonNumericLineageID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-upd-id-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPut, c.BaseURL+"/lineage/notanumber", apiKey,
		lineageJSON(t, map[string]interface{}{"parent_name": "X"}), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Update_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-upd-json-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPut, c.BaseURL+"/lineage/1", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeleteLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Delete_RejectsNonNumericLineageID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-del-id-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodDelete, c.BaseURL+"/lineage/abc", apiKey, nil, ""))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// SetLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Set_RejectsNonNumericStrainID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-set-id-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPut, c.BaseURL+"/strains/abc/lineage", apiKey,
		lineageJSON(t, map[string]interface{}{"parents": []interface{}{}}), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Set_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-set-json-key"
	lineageAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(lineageReq(t, http.MethodPut, c.BaseURL+"/strains/1/lineage", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GetLineageHandler / GetDescendantsHandler  (public reads)
// ---------------------------------------------------------------------------

func TestLineageHTTP_Get_RejectsNonNumericID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/abc/lineage")
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Get_EmptyReturnsArray(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	lineageSeedStrains(t, db)

	c := server.NewClient(t)
	resp := c.Get("/strains/1/lineage")
	defer lineageDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t,
		bytes.HasPrefix(body, []byte("[")),
		"empty lineage must serialise as [], not null; got %q", body)
}

func TestLineageHTTP_Descendants_RejectsNonNumericID(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/abc/descendants")
	defer lineageDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Descendants_EmptyReturnsArray(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	lineageSeedStrains(t, db)

	c := server.NewClient(t)
	resp := c.Get("/strains/2/descendants")
	defer lineageDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(body, []byte("[")),
		"empty descendants must serialise as []; got %q", body)
}

// ---------------------------------------------------------------------------
// LookupStrainByName  (public read)
// ---------------------------------------------------------------------------

func TestLineageHTTP_Lookup_TruncatesOverlongQuery(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	// Long query is truncated to MaxNameLength internally — handler must
	// still respond 200 with a JSON array (possibly empty).
	resp := c.Get("/strains/lookup?q=" + strings.Repeat("z", 4096))
	defer lineageDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(body, []byte("[")), "lookup must return []")
}
