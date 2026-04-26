package handlers_test

// HTTP-layer tests for handlers/ac_infinity.go (ACILoginHandler at
// POST /aci/login). The success path requires a live call to AC
// Infinity's auth API which Phase 4b explicitly forbids in tests; this
// file therefore only exercises auth and the bad-JSON validation branch.

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

func aciAPIKey(t *testing.T, db *sql.DB, plaintext string) {
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

func aciDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestACInfinityHTTP_RequiresAuth(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/aci/login", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer aciDrain(resp)
	assert.Containsf(t,
		[]int{http.StatusUnauthorized, http.StatusForbidden},
		resp.StatusCode,
		"unauthenticated POST /aci/login should be rejected (got %d)", resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestACInfinityHTTP_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "aci-login-json-key"
	aciAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/aci/login", bytes.NewBufferString(`{`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", apiKey)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer aciDrain(resp)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Response is shaped as {"success": false, "message": "..."}
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"success":false`,
		"bad-JSON path returns the AC-Infinity-style envelope, not the generic api_error one")
}
