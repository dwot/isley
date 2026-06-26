package integration

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

const apiKeyTestPassword = "api-key-test-pw"

// createdKey mirrors the JSON returned by the create/regenerate endpoints.
type createdKey struct {
	Message string `json:"message"`
	APIKey  string `json:"api_key"`
	Key     struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
	} `json:"key"`
}

func newAPIKeySession(t *testing.T) (*testutil.TestServer, *testutil.Client, string) {
	t.Helper()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, apiKeyTestPassword)
	c, csrf := server.LoginAndFetchCSRF(t, apiKeyTestPassword, "/settings")
	return server, c, csrf
}

func createAPIKey(t *testing.T, c *testutil.Client, csrf, name string) createdKey {
	t.Helper()
	resp := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{"name": name})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got createdKey
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got.APIKey, 32, "plaintext key should be a 32-char hex string")
	require.Equal(t, name, got.Key.Name)
	require.Equal(t, got.APIKey[:8], got.Key.Prefix)
	return got
}

// sessionDelete issues a DELETE on the session client with the CSRF header set.
func sessionDelete(t *testing.T, c *testutil.Client, csrf, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-CSRF-Token", csrf)
	resp, err := c.Do(req)
	require.NoError(t, err)
	return resp
}

func TestAPIKeys_CreateReturnsPlaintextAndAuthenticates(t *testing.T) {
	t.Parallel()

	server, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "Greenhouse controller")

	// The new key should authenticate an X-API-KEY request from a cookie-less
	// client (so the session can't mask a broken key check).
	nc := server.NewClient(t)
	resp := nc.APIGet(t, "/api/overlay", created.APIKey)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// It should appear in the list with a prefix but no secret.
	listResp := c.Get("/settings/api-keys")
	defer testutil.DrainAndClose(listResp)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var list struct {
		Keys []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Prefix string `json:"prefix"`
		} `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	require.Len(t, list.Keys, 1)
	assert.Equal(t, "Greenhouse controller", list.Keys[0].Name)
	assert.Equal(t, created.APIKey[:8], list.Keys[0].Prefix)
}

func TestAPIKeys_CreateRejectsBlankName(t *testing.T) {
	t.Parallel()

	_, c, csrf := newAPIKeySession(t)
	resp := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{"name": "   "})
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAPIKeys_CreateRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	_, c, csrf := newAPIKeySession(t)
	createAPIKey(t, c, csrf, "Device A")

	dup := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{"name": "Device A"})
	defer testutil.DrainAndClose(dup)
	assert.Equal(t, http.StatusConflict, dup.StatusCode)

	// Uniqueness is case-insensitive.
	dupCase := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{"name": "device a"})
	defer testutil.DrainAndClose(dupCase)
	assert.Equal(t, http.StatusConflict, dupCase.StatusCode)
}

func TestAPIKeys_StoresHashNotPlaintext(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, apiKeyTestPassword)
	c, csrf := server.LoginAndFetchCSRF(t, apiKeyTestPassword, "/settings")

	created := createAPIKey(t, c, csrf, "Logger")

	var hash, prefix string
	require.NoError(t, db.QueryRow(
		`SELECT key_hash, prefix FROM api_keys WHERE id = $1`, created.Key.ID,
	).Scan(&hash, &prefix))
	assert.NotEqual(t, created.APIKey, hash, "DB must store the hash, not the plaintext")
	assert.Contains(t, hash, "$2", "hash should be a bcrypt digest")
	assert.Equal(t, created.APIKey[:8], prefix)
}

func TestAPIKeys_RegenerateInvalidatesOldValue(t *testing.T) {
	t.Parallel()

	server, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "Rotating")

	resp := c.SessionPostJSON(t, "/settings/api-keys/"+strconv.Itoa(created.Key.ID)+"/regenerate", csrf, nil)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rotated createdKey
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rotated))
	require.Len(t, rotated.APIKey, 32)
	require.NotEqual(t, created.APIKey, rotated.APIKey, "regenerate must mint a different key")

	// Old value rejected, new value accepted.
	nc := server.NewClient(t)
	oldResp := nc.APIGet(t, "/api/overlay", created.APIKey)
	testutil.DrainAndClose(oldResp)
	assert.Equal(t, http.StatusUnauthorized, oldResp.StatusCode, "old key must stop working")

	newResp := nc.APIGet(t, "/api/overlay", rotated.APIKey)
	testutil.DrainAndClose(newResp)
	assert.Equal(t, http.StatusOK, newResp.StatusCode, "rotated key must work")
}

func TestAPIKeys_RevokeLocksOutKey(t *testing.T) {
	t.Parallel()

	server, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "Doomed")

	resp := sessionDelete(t, c, csrf, "/settings/api-keys/"+strconv.Itoa(created.Key.ID))
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	nc := server.NewClient(t)
	authResp := nc.APIGet(t, "/api/overlay", created.APIKey)
	testutil.DrainAndClose(authResp)
	assert.Equal(t, http.StatusUnauthorized, authResp.StatusCode)

	// And it's gone from the list.
	listResp := c.Get("/settings/api-keys")
	defer testutil.DrainAndClose(listResp)
	var list struct {
		Keys []json.RawMessage `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	assert.Empty(t, list.Keys)
}

func TestAPIKeys_MultipleKeysAuthenticateIndependently(t *testing.T) {
	t.Parallel()

	server, c, csrf := newAPIKeySession(t)
	first := createAPIKey(t, c, csrf, "Device A")
	second := createAPIKey(t, c, csrf, "Device B")

	nc := server.NewClient(t)

	// Both authenticate.
	for _, key := range []string{first.APIKey, second.APIKey} {
		resp := nc.APIGet(t, "/api/overlay", key)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Revoking the first leaves the second working.
	delResp := sessionDelete(t, c, csrf, "/settings/api-keys/"+strconv.Itoa(first.Key.ID))
	testutil.DrainAndClose(delResp)
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	gone := nc.APIGet(t, "/api/overlay", first.APIKey)
	testutil.DrainAndClose(gone)
	assert.Equal(t, http.StatusUnauthorized, gone.StatusCode)

	stillThere := nc.APIGet(t, "/api/overlay", second.APIKey)
	testutil.DrainAndClose(stillThere)
	assert.Equal(t, http.StatusOK, stillThere.StatusCode)
}

// TestAPIKeys_ManagementRequiresSession verifies that key management is gated on
// a browser session and cannot be driven with only an X-API-KEY header — a
// leaked key must not be able to mint or list other keys.
func TestAPIKeys_ManagementRequiresSession(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	plaintext := testutil.SeedAPIKey(t, db, "seeded-management-key")

	nc := server.NewClient(t)
	req, err := http.NewRequest(http.MethodGet, nc.BaseURL+"/settings/api-keys", nil)
	require.NoError(t, err)
	req.Header.Set("X-API-KEY", plaintext)
	resp, err := nc.Do(req)
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	// AuthMiddleware redirects unauthenticated browser requests to /login
	// rather than serving the key list.
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// TestAPIKeys_SessionCannotBypassCSRFWithKeyHeader verifies that the API-key
// CSRF exemption does not apply to a logged-in session: a session POST carrying
// an X-API-KEY header but no CSRF token must still be rejected.
func TestAPIKeys_SessionCannotBypassCSRFWithKeyHeader(t *testing.T) {
	t.Parallel()

	_, c, _ := newAPIKeySession(t)

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/settings/api-keys",
		testutil.JSONBody(t, map[string]interface{}{"name": "Should fail"}))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "irrelevant-value")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestAPIKeys_LegacyKeyBackfillsPrefix verifies a key carried over from the
// single-key era (empty prefix) authenticates and has its prefix backfilled on
// first use.
func TestAPIKeys_LegacyKeyBackfillsPrefix(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const legacyPlaintext = "legacy-single-key-value"
	seedHashedAPIKey(t, db, handlers.HashAPIKey(legacyPlaintext))

	// Confirm it starts with an empty prefix.
	var before string
	require.NoError(t, db.QueryRow(`SELECT prefix FROM api_keys WHERE key_hash IS NOT NULL ORDER BY id LIMIT 1`).Scan(&before))
	require.Equal(t, "", before)

	nc := server.NewClient(t)
	resp := nc.APIGet(t, "/api/overlay", legacyPlaintext)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var after string
	require.NoError(t, db.QueryRow(`SELECT prefix FROM api_keys WHERE key_hash IS NOT NULL ORDER BY id LIMIT 1`).Scan(&after))
	assert.Equal(t, legacyPlaintext[:8], after, "prefix should be backfilled on first use")
}
