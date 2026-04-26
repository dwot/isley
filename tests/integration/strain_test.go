package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// seedBreederWithKey creates one breeder row and an API key. Returns
// the api key plaintext.
func seedBreederWithKey(t *testing.T, db *sql.DB) string {
	t.Helper()
	testutil.SeedBreeder(t, db, "Acme Genetics")
	return testutil.SeedAPIKey(t, db, "test-strain-api-key")
}

// apiPutJSON issues PUT path with a JSON body and an X-API-KEY header.
func apiPutJSON(t *testing.T, c *testutil.Client, path, apiKey string, body interface{}) *http.Response {
	t.Helper()
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+path, apiKey,
		testutil.JSONBody(t, body), "application/json"))
	require.NoError(t, err)
	return resp
}

// ---------------------------------------------------------------------------
// POST /strains
// ---------------------------------------------------------------------------

func TestStrain_AddHappyPath(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedBreederWithKey(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/strains", apiKey, map[string]interface{}{
		"name":        "OG Test",
		"breeder_id":  1,
		"indica":      70,
		"sativa":      30,
		"autoflower":  false,
		"seed_count":  5,
		"description": "a desc",
		"short_desc":  "short",
		"cycle_time":  56,
		"url":         "https://example",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID      int    `json:"id"`
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.ID)

	// DB-level: fields persisted as expected.
	var name string
	var indica, sativa, seedCount, autoflower int
	require.NoError(t, db.QueryRow(
		`SELECT name, indica, sativa, autoflower, seed_count FROM strain WHERE id = $1`, got.ID,
	).Scan(&name, &indica, &sativa, &autoflower, &seedCount))
	assert.Equal(t, "OG Test", name)
	assert.Equal(t, 70, indica)
	assert.Equal(t, 30, sativa)
	assert.Equal(t, 0, autoflower, "autoflower=false should persist as 0")
	assert.Equal(t, 5, seedCount)
}

func TestStrain_AddCreatesNewBreeder(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintext = "test-strain-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	// breeder_id omitted → handler reads new_breeder and inserts a row.
	resp := apiPostJSON(t, c, "/strains", plaintext, map[string]interface{}{
		"name":        "Hybrid",
		"new_breeder": "Brand New Breeder",
		"indica":      50,
		"sativa":      50,
		"autoflower":  true,
		"seed_count":  0,
		"description": "",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// New breeder row exists.
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM breeder WHERE name = $1`, "Brand New Breeder").Scan(&name))
	assert.Equal(t, "Brand New Breeder", name)
}

func TestStrain_AddRejectsIndicaSativaSum(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedBreederWithKey(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/strains", apiKey, map[string]interface{}{
		"name":       "Bad Sum",
		"breeder_id": 1,
		"indica":     50,
		"sativa":     49, // 99 — must equal 100
		"autoflower": false,
		"seed_count": 0,
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStrain_AddRejectsMissingBreeder(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintext = "test-strain-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	// Neither breeder_id nor new_breeder.
	resp := apiPostJSON(t, c, "/strains", plaintext, map[string]interface{}{
		"name":       "Orphan",
		"indica":     50,
		"sativa":     50,
		"autoflower": false,
		"seed_count": 0,
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// PUT /strains/:id
// ---------------------------------------------------------------------------

func TestStrain_UpdateHappyPath(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedBreederWithKey(t, db)

	// Seed a strain to update.
	res, err := db.Exec(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ('Old Name', 1, 50, 50, 0, '', 0)`,
	)
	require.NoError(t, err)
	strainID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/strains/"+strconv.FormatInt(strainID, 10), apiKey, map[string]interface{}{
		"name":        "New Name",
		"breeder_id":  1,
		"indica":      60,
		"sativa":      40,
		"autoflower":  true,
		"description": "updated",
		"seed_count":  10,
		"cycle_time":  70,
		"url":         "https://updated",
		"short_desc":  "short",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	var indica, sativa, seedCount, autoflower int
	require.NoError(t, db.QueryRow(
		`SELECT name, indica, sativa, autoflower, seed_count FROM strain WHERE id = $1`, strainID,
	).Scan(&name, &indica, &sativa, &autoflower, &seedCount))
	assert.Equal(t, "New Name", name)
	assert.Equal(t, 60, indica)
	assert.Equal(t, 40, sativa)
	assert.Equal(t, 1, autoflower, "autoflower=true should persist as 1")
	assert.Equal(t, 10, seedCount)
}

// ---------------------------------------------------------------------------
// DELETE /strains/:id
// ---------------------------------------------------------------------------

func TestStrain_DeleteHappyPath(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedBreederWithKey(t, db)

	res, err := db.Exec(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ('Doomed', 1, 50, 50, 0, '', 0)`,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/strains/"+strconv.FormatInt(id, 10), apiKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM strain WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

func TestStrain_DeleteMissing(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedBreederWithKey(t, db)

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/strains/9999", apiKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GET /strains/:id  (api-protected, returns JSON)
// ---------------------------------------------------------------------------
//
// Note: there are TWO routes that match /strains/:id semantics:
//   - GET /strain/:id   (basic / session-gated, renders an HTML view)
//   - GET /strains/:id  (basic, returns JSON via GetStrainHandler)
//
// We exercise the JSON endpoint via API key by going through the
// session-protected basic-route group: log in as admin first.

func TestStrain_GetByIDJSON(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	// Seed reference data.
	mustExecRow(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	res, err := db.Exec(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ('Visible', 1, 50, 50, 0, '', 3)`,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	// /strains/:id is in the basic route group → session gate is on.
	testutil.SeedAdmin(t, db, "strain-pw")
	c := server.LoginAsAdmin(t, "strain-pw")

	resp := c.Get("/strains/" + strconv.FormatInt(id, 10))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, float64(id), got["id"])
	assert.Equal(t, "Visible", got["name"])
	assert.Equal(t, "B", got["breeder"])
}

func TestStrain_GetByIDNotFound(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, "strain-pw")
	c := server.LoginAsAdmin(t, "strain-pw")
	resp := c.Get("/strains/9999")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GET /strains/in-stock  +  /strains/out-of-stock
// ---------------------------------------------------------------------------

func TestStrain_InStockOnlyReturnsPositiveSeedCount(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	mustExecRow(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	mustExecRow(t, db, `INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES ('InStockOne', 1, 50, 50, 0, '', 5)`)
	mustExecRow(t, db, `INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES ('OutOfStock', 1, 50, 50, 0, '', 0)`)

	testutil.SeedAdmin(t, db, "stock-pw")
	c := server.LoginAsAdmin(t, "stock-pw")

	resp := c.Get("/strains/in-stock")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "InStockOne", got[0]["name"])
}

func TestStrain_OutOfStockOnlyReturnsZeroSeedCount(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	mustExecRow(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	mustExecRow(t, db, `INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES ('InStockOne', 1, 50, 50, 0, '', 5)`)
	mustExecRow(t, db, `INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES ('OutOfStock', 1, 50, 50, 0, '', 0)`)

	testutil.SeedAdmin(t, db, "stock-pw")
	c := server.LoginAsAdmin(t, "stock-pw")

	resp := c.Get("/strains/out-of-stock")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "OutOfStock", got[0]["name"])
}

// ---------------------------------------------------------------------------
// POST /breeders + PUT /breeders/:id + DELETE /breeders/:id
// ---------------------------------------------------------------------------

func TestBreeder_AddHappyPath(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintext = "test-breeder-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/breeders", plaintext, map[string]interface{}{
		"breeder_name": "New Breeder",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.ID)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM breeder WHERE id = $1`, got.ID).Scan(&name))
	assert.Equal(t, "New Breeder", name)
}

func TestBreeder_AddRejectsEmptyName(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintext = "test-breeder-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/breeders", plaintext, map[string]interface{}{
		"breeder_name": "",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBreeder_UpdateRenames(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	mustExecRow(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'Old Name')`)
	const plaintext = "test-breeder-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/breeders/1", plaintext, map[string]interface{}{
		"breeder_name": "New Name",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM breeder WHERE id = 1`).Scan(&name))
	assert.Equal(t, "New Name", name)
}

func TestBreeder_DeleteCascadesStrainsAndPlants(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	// breeder → strain → plant chain. DeleteBreederHandler removes
	// associated plants, then strains, then the breeder itself.
	mustExecRow(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'Doomed')`)
	mustExecRow(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES (1, 'Doomed Strain', 1, 50, 50, 0, '', 0)`)
	mustExecRow(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	mustExecRow(t, db, `INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	                    VALUES ('Doomed Plant', 1, 1, '', 0, '2026-01-01', '[]')`)

	const plaintext = "test-breeder-key"
	testutil.SeedAPIKey(t, db, plaintext)

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/breeders/1", plaintext)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	for _, table := range []string{"breeder", "strain", "plant"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
		assert.Zerof(t, n, "%s should be empty after breeder delete cascade", table)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// mustExecRow keeps the integration test focused on the API surface;
// errors in seed setup are fatal and shouldn't pollute test bodies with
// repetitive `_, err := ... ; require.NoError(t, err)` boilerplate.
func mustExecRow(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	_, err := db.Exec(query, args...)
	require.NoErrorf(t, err, "seed: %s", query)
}
