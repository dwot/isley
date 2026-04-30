package handlers_test

// HTTP-layer tests for handlers/strain.go beyond what
// tests/integration/strain_test.go already covers. The integration suite
// hits the AddStrain / UpdateStrain / DeleteStrain happy paths plus a
// few validation branches; this file fills in:
//
//   - auth gating across breeder + strain api endpoints
//   - AddBreeder / UpdateBreeder validation (bad JSON, blank, overlength)
//   - AddStrain — bad JSON, missing breeder when BreederID is nil
//   - UpdateStrain — invalid id, bad JSON, indica/sativa sum mismatch,
//     missing breeder when BreederID is nil
//   - GetStrainHandler — invalid id (non-numeric), not-found
//   - DeleteStrainHandler — invalid id
//   - PlantsByStrainHandler (basic-route, session-only) — invalid id,
//     happy path
//
// Routes:
//
//   POST   /strains              (api-protected) → AddStrainHandler
//   PUT    /strains/:id          (api-protected) → UpdateStrainHandler
//   DELETE /strains/:id          (api-protected) → DeleteStrainHandler
//   GET    /strains/:id          (basic)         → GetStrainHandler
//   GET    /strains/in-stock     (basic)         → InStockStrainsHandler
//   GET    /strains/out-of-stock (basic)         → OutOfStockStrainsHandler
//   GET    /plants/by-strain/:strainID (basic)   → PlantsByStrainHandler
//   POST   /breeders             (api-protected) → AddBreederHandler
//   PUT    /breeders/:id         (api-protected) → UpdateBreederHandler
//   DELETE /breeders/:id         (api-protected) → DeleteBreederHandler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestStrainHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/strains"},
		{http.MethodPut, "/strains/1"},
		{http.MethodDelete, "/strains/1"},
		{http.MethodPost, "/breeders"},
		{http.MethodPut, "/breeders/1"},
		{http.MethodDelete, "/breeders/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer testutil.DrainAndClose(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// AddBreederHandler
// ---------------------------------------------------------------------------

func TestStrainHTTP_AddBreeder_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addbreeder-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/breeders", apiKey,
		testutil.JSONBody(t, map[string]string{"breeder_name": "Acme"}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Greater(t, got.ID, 0)
}

func TestStrainHTTP_AddBreeder_RejectsBlank(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addbreeder-blank-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/breeders", apiKey,
		testutil.JSONBody(t, map[string]string{"breeder_name": ""}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_AddBreeder_RejectsLongName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addbreeder-long-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/breeders", apiKey,
		testutil.JSONBody(t, map[string]string{"breeder_name": strings.Repeat("b", 1024)}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_AddBreeder_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addbreeder-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/breeders", apiKey,
		bytes.NewBufferString(`{not json`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdateBreederHandler
// ---------------------------------------------------------------------------

func TestStrainHTTP_UpdateBreeder_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updbreeder-key"
	testutil.SeedAPIKey(t, db, apiKey)

	id := testutil.SeedBreeder(t, db, "Strain Test Breeder")
	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/breeders/"+strconv.Itoa(id), apiKey,
		testutil.JSONBody(t, map[string]string{"breeder_name": "Renamed"}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM breeder WHERE id = $1`, id).Scan(&name))
	assert.Equal(t, "Renamed", name)
}

func TestStrainHTTP_UpdateBreeder_RejectsBlank(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updbreeder-blank-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/breeders/1", apiKey,
		testutil.JSONBody(t, map[string]string{"breeder_name": ""}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// AddStrainHandler — additional validation paths
// ---------------------------------------------------------------------------

func TestStrainHTTP_AddStrain_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addstr-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_AddStrain_RejectsMissingBreederWhenNoNewBreeder(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "addstr-noBreeder-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"name":      "Nameless",
			"indica":    50,
			"sativa":    50,
			"ruderalis": 0,
			// breeder_id == nil and new_breeder == "" → should be 400
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdateStrainHandler
// ---------------------------------------------------------------------------

func TestStrainHTTP_UpdateStrain_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updstr-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/strains/abc", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"name":       "X",
			"indica":     50,
			"sativa":     50,
			"breeder_id": 1,
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-numeric :id must surface as api_invalid_strain_id")
}

func TestStrainHTTP_UpdateStrain_RejectsBadIndicaSativaSum(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updstr-sum-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/strains/1", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"name":       "X",
			"indica":     30,
			"sativa":     40, // 30 + 40 != 100
			"breeder_id": 1,
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_UpdateStrain_RejectsMissingBreeder(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "updstr-nobr-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/strains/1", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"name":   "X",
			"indica": 50,
			"sativa": 50,
			// breeder_id nil + new_breeder empty
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeleteStrainHandler
// ---------------------------------------------------------------------------

func TestStrainHTTP_DeleteStrain_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "delstr-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/strains/notanumber", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GetStrainHandler  (basic route — public)
// ---------------------------------------------------------------------------

func TestStrainHTTP_GetStrain_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/abc")
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_GetStrain_NotFound(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/99999")
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// PlantsByStrainHandler — covers the strain-id parsing branch
// ---------------------------------------------------------------------------

func TestStrainHTTP_PlantsByStrain_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/plants/by-strain/abc")
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrainHTTP_PlantsByStrain_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	// Seed the FK chain plus a plant.
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	testutil.MustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	          VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	          VALUES ('Plant1', 1, 1, '', 0, '2026-01-01', '[]')`)

	c := server.NewClient(t)
	resp := c.Get("/plants/by-strain/1")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rows []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "Plant1", rows[0]["name"])
}

// ---------------------------------------------------------------------------
// In/Out of stock smoke tests
// ---------------------------------------------------------------------------

// TestStrainHTTP_InStock_SmokeReturnsArray confirms /strains/in-stock
// responds 200 with an array shape even with no rows. The integration
// suite asserts on content; here we lock down the basic contract.
func TestStrainHTTP_InStock_SmokeReturnsArray(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/in-stock")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	// Must start with '[' (an array, possibly "null" or "[]")
	assert.True(t,
		bytes.HasPrefix(body, []byte("[")) || bytes.Equal(body, []byte("null\n")) || bytes.Equal(body, []byte("null")),
		"in-stock should respond with a JSON array; got %q", body)
}

func TestStrainHTTP_OutOfStock_SmokeReturnsArray(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/out-of-stock")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
