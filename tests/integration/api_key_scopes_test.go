package integration

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// scopeError is the 403 body RequireKeyScope returns.
type scopeError struct {
	Error    string `json:"error"`
	Code     string `json:"code"`
	Required string `json:"required"`
}

func requireInsufficientScope(t *testing.T, resp *http.Response, required string) {
	t.Helper()
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	var body scopeError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "insufficient_scope", body.Code)
	assert.Equal(t, required, body.Required)
	assert.NotEmpty(t, body.Error)
}

func ingestReading(t *testing.T, c *testutil.Client, key string) *http.Response {
	t.Helper()
	return c.APIPostJSON(t, "/api/sensors/ingest", key, map[string]interface{}{
		"source": "scope-test", "device": "D1", "type": "temp", "value": 20.0, "unit": "C",
	})
}

// The web UI's own endpoints (settings, deletes, backups, ...) need admin.
// A read key can use the overlay but nothing else.
func TestAPIKeyScopes_ReadKey(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(storeWithAPIIngest(1)))
	key := testutil.SeedAPIKeyWithScopes(t, db, "readkey-0000000000000000000000000", "read")
	c := server.NewClient(t)

	ok := c.APIGet(t, "/api/overlay", key)
	testutil.DrainAndClose(ok)
	assert.Equal(t, http.StatusOK, ok.StatusCode)

	requireInsufficientScope(t, c.APIGet(t, "/metrics", key), "admin")
	requireInsufficientScope(t, c.APIDelete(t, "/plant/delete/1", key), "admin")
	requireInsufficientScope(t, ingestReading(t, c, key), "ingest")
}

// Write is below admin, so the legacy endpoints stay closed to it too.
func TestAPIKeyScopes_WriteKeyCannotUseLegacyEndpoints(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	key := testutil.SeedAPIKeyWithScopes(t, db, "writekey-000000000000000000000000", "write")
	c := server.NewClient(t)

	requireInsufficientScope(t, c.APIGet(t, "/metrics", key), "admin")
}

func TestAPIKeyScopes_IngestOnlyKey(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(storeWithAPIIngest(1)))
	key := testutil.SeedAPIKeyWithScopes(t, db, "ingestkey-00000000000000000000000", "ingest")
	c := server.NewClient(t)

	resp := ingestReading(t, c, key)
	testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	requireInsufficientScope(t, c.APIGet(t, "/api/overlay", key), "read")
	requireInsufficientScope(t, c.APIGet(t, "/metrics", key), "admin")
}

// Admin can do everything, including ingest, as every key could before
// scopes existed.
func TestAPIKeyScopes_AdminKey(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(storeWithAPIIngest(1)))
	key := testutil.SeedAPIKey(t, db, "adminkey-000000000000000000000000")
	c := server.NewClient(t)

	for _, resp := range []*http.Response{
		c.APIGet(t, "/metrics", key),
		c.APIGet(t, "/api/overlay", key),
		ingestReading(t, c, key),
	} {
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode, resp.Request.URL.Path)
	}
}

// A key with scopes that can't be parsed (hand-edited row) is denied
// everything rather than guessed at.
func TestAPIKeyScopes_UnreadableScopesFailClosed(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	key := testutil.SeedAPIKeyWithScopes(t, db, "badkey-0000000000000000000000000", "owner")
	c := server.NewClient(t)

	requireInsufficientScope(t, c.APIGet(t, "/api/overlay", key), "read")
}

func TestAPIKeyScopes_CreateDefaultsToRead(t *testing.T) {
	t.Parallel()
	_, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "No scopes given")

	listResp := c.Get("/settings/api-keys")
	defer testutil.DrainAndClose(listResp)
	var list struct {
		Keys []struct {
			ID     int      `json:"id"`
			Scopes []string `json:"scopes"`
		} `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	require.Len(t, list.Keys, 1)
	assert.Equal(t, created.Key.ID, list.Keys[0].ID)
	assert.Equal(t, []string{"read"}, list.Keys[0].Scopes)
}

func TestAPIKeyScopes_CreateWithScopes(t *testing.T) {
	t.Parallel()
	server, c, csrf := newAPIKeySession(t)

	resp := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{
		"name": "Probe", "scopes": []string{"ingest", "read"},
	})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got struct {
		APIKey string `json:"api_key"`
		Key    struct {
			Scopes []string `json:"scopes"`
		} `json:"key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, []string{"read", "ingest"}, got.Key.Scopes, "scopes come back canonical")

	nc := server.NewClient(t)
	requireInsufficientScope(t, nc.APIGet(t, "/metrics", got.APIKey), "admin")
}

func TestAPIKeyScopes_CreateRejectsBadScopes(t *testing.T) {
	t.Parallel()
	_, c, csrf := newAPIKeySession(t)

	for _, scopes := range [][]string{{}, {"owner"}, {"read", "delete"}} {
		resp := c.SessionPostJSON(t, "/settings/api-keys", csrf, map[string]interface{}{
			"name": "Bad", "scopes": scopes,
		})
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%v", scopes)
	}
}

// Changing a key's scopes takes effect immediately and keeps its secret.
func TestAPIKeyScopes_UpdateScopes(t *testing.T) {
	t.Parallel()
	server, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "Script")
	path := "/settings/api-keys/" + strconv.Itoa(created.Key.ID) + "/scopes"
	nc := server.NewClient(t)

	requireInsufficientScope(t, nc.APIGet(t, "/metrics", created.APIKey), "admin")

	resp := c.SessionPutJSON(t, path, csrf, map[string]interface{}{"scopes": []string{"admin"}})
	var body struct {
		Scopes []string `json:"scopes"`
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	testutil.DrainAndClose(resp)
	assert.Equal(t, []string{"admin"}, body.Scopes)

	ok := nc.APIGet(t, "/metrics", created.APIKey)
	testutil.DrainAndClose(ok)
	assert.Equal(t, http.StatusOK, ok.StatusCode, "same secret, wider access")

	// Narrow it again.
	resp = c.SessionPutJSON(t, path, csrf, map[string]interface{}{"scopes": []string{"read"}})
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireInsufficientScope(t, nc.APIGet(t, "/metrics", created.APIKey), "admin")
}

func TestAPIKeyScopes_UpdateRejectsBadInput(t *testing.T) {
	t.Parallel()
	_, c, csrf := newAPIKeySession(t)
	created := createAPIKey(t, c, csrf, "Script")
	path := "/settings/api-keys/" + strconv.Itoa(created.Key.ID) + "/scopes"

	for _, body := range []map[string]interface{}{
		{},
		{"scopes": []string{}},
		{"scopes": []string{"superuser"}},
	} {
		resp := c.SessionPutJSON(t, path, csrf, body)
		testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%v", body)
	}

	missing := c.SessionPutJSON(t, "/settings/api-keys/99999/scopes", csrf, map[string]interface{}{"scopes": []string{"read"}})
	testutil.DrainAndClose(missing)
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)
}

// Only a browser session can change a key's scopes; a key cannot widen
// itself.
func TestAPIKeyScopes_UpdateRequiresSession(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	key := testutil.SeedAPIKey(t, db, "adminkey-000000000000000000000000")

	nc := server.NewClient(t)
	req := testutil.APIReq(t, http.MethodPut, nc.BaseURL+"/settings/api-keys/1/scopes", key,
		testutil.JSONBody(t, map[string]interface{}{"scopes": []string{"admin"}}), "application/json")
	resp, err := nc.Do(req)
	require.NoError(t, err)
	testutil.DrainAndClose(resp)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// Key checks are cached for a minute; revoking or regenerating a key must
// still cut off the old secret at once.
func TestAPIKeyScopes_RevokeAndRegenerateTakeEffectAtOnce(t *testing.T) {
	t.Parallel()
	server, c, csrf := newAPIKeySession(t)
	nc := server.NewClient(t)

	regen := createAPIKey(t, c, csrf, "Regen")
	resp := nc.APIGet(t, "/api/v1/instance", regen.APIKey)
	require.Equal(t, http.StatusOK, resp.StatusCode, "first use caches the check")
	testutil.DrainAndClose(resp)
	resp = c.SessionPostJSON(t, "/settings/api-keys/"+strconv.Itoa(regen.Key.ID)+"/regenerate", csrf, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = nc.APIGet(t, "/api/v1/instance", regen.APIKey)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the old secret stops working")
	testutil.DrainAndClose(resp)

	revoked := createAPIKey(t, c, csrf, "Revoked")
	resp = nc.APIGet(t, "/api/v1/instance", revoked.APIKey)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = c.SessionDelete(t, "/settings/api-keys/"+strconv.Itoa(revoked.Key.ID), csrf)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.DrainAndClose(resp)
	resp = nc.APIGet(t, "/api/v1/instance", revoked.APIKey)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	testutil.DrainAndClose(resp)
}
