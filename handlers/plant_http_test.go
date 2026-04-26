package handlers_test

// HTTP-layer tests for handlers/plant.go beyond what
// tests/integration/plant_test.go already covers. The integration suite
// exercises happy paths for AddPlant and DeletePlant; this file fills in
// the validation/error branches plus UpdatePlant, LinkSensorsToPlant,
// LivingPlantsHandler, HarvestedPlantsHandler, and DeadPlantsHandler.
//
// Routes exercised:
//
//   POST   /plants                 → AddPlant            (api-protected)
//   POST   /plant                  → UpdatePlant         (api-protected)
//   DELETE /plant/delete/:id       → DeletePlant         (api-protected)
//   POST   /plant/link-sensors     → LinkSensorsToPlant  (api-protected)
//   GET    /plants/living          → LivingPlantsHandler (session-only)
//   GET    /plants/harvested       → HarvestedPlantsHandler (session-only)
//   GET    /plants/dead            → DeadPlantsHandler   (session-only)

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// File-local helpers
// ---------------------------------------------------------------------------

// plantTestSeed sets up the FK chain (breeder → strain → zone) required
// by every plant insert, plus an api_key for X-API-KEY auth. Returns the
// plaintext api_key.
func plantTestSeed(t *testing.T, db *sql.DB) string {
	t.Helper()
	mustExec := func(query string, args ...interface{}) {
		_, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
	}
	mustExec(`INSERT INTO breeder (id, name) VALUES (1, 'Plant Test Breeder')`)
	mustExec(`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	          VALUES (1, 'Plant Test Strain', 1, 50, 50, 0, '', 5)`)
	mustExec(`INSERT INTO zones (id, name) VALUES (1, 'Plant Test Zone')`)

	const plaintext = "plant-http-test-key"
	hashed := handlers.HashAPIKey(plaintext)
	var existingID int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = 'api_key'`).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(`INSERT INTO settings (name, value) VALUES ('api_key', $1)`, hashed)
	case err == nil:
		_, err = db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, hashed, existingID)
	}
	require.NoError(t, err)
	return plaintext
}

func plantStatusID(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = $1`, name).Scan(&id))
	return id
}

func plantReq(t *testing.T, method, url, apiKey string, body io.Reader, contentType string) *http.Request {
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

func plantJSONBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(v))
	return &buf
}

func plantDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// insertSeedPlant inserts a plant directly so DeletePlant /
// LinkSensorsToPlant tests don't depend on the AddPlant flow.
func insertSeedPlant(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ($1, 1, 1, '', 0, '2026-01-01', '[]')`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// ---------------------------------------------------------------------------
// AddPlant — validation paths beyond the integration suite
// ---------------------------------------------------------------------------

// TestPlantHTTP_Add_RejectsLongName confirms ValidateRequiredString
// surfaces overlength values as 400 (the failure message contains the
// validator's "too long" text).
func TestPlantHTTP_Add_RejectsLongName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plants", apiKey,
		plantJSONBody(t, map[string]interface{}{
			"name":      strings.Repeat("a", 1024), // way over MaxNameLength
			"zone_id":   1,
			"strain_id": 1,
			"status_id": plantStatusID(t, db, "Veg"),
			"date":      "2026-04-25",
		}), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"name longer than MaxNameLength must be rejected")
}

// TestPlantHTTP_Add_RejectsBadJSON confirms a malformed body trips the
// initial ShouldBindJSON branch with 400.
func TestPlantHTTP_Add_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plants", apiKey,
		bytes.NewBufferString(`{"name":"x","zone_id":}`), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdatePlant
// ---------------------------------------------------------------------------

func TestPlantHTTP_Update_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	plantID := insertSeedPlant(t, db, "Original")

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plant", apiKey,
		plantJSONBody(t, map[string]interface{}{
			"plant_id":          plantID,
			"plant_name":        "Renamed",
			"plant_description": "now with a description",
			"zone_id":           1,
			"strain_id":         1,
			"clone":             false,
			"start_date":        "2026-01-01",
			"harvest_weight":    0.0,
		}), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode,
		"UpdatePlant returns 201 on success")

	var name, desc string
	require.NoError(t, db.QueryRow(`SELECT name, description FROM plant WHERE id = $1`, plantID).Scan(&name, &desc))
	assert.Equal(t, "Renamed", name)
	assert.Equal(t, "now with a description", desc)
}

func TestPlantHTTP_Update_RejectsLongDescription(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	plantID := insertSeedPlant(t, db, "Has Long Desc")

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plant", apiKey,
		plantJSONBody(t, map[string]interface{}{
			"plant_id":          plantID,
			"plant_name":        "OK",
			"plant_description": strings.Repeat("d", 100_000), // far past MaxDescriptionLength
			"zone_id":           1,
			"strain_id":         1,
			"start_date":        "2026-01-01",
		}), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantHTTP_Update_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plant", apiKey,
		bytes.NewBufferString(`{"plant_id":` /* truncated */), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// LinkSensorsToPlant
// ---------------------------------------------------------------------------

func TestPlantHTTP_LinkSensors_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	plantID := insertSeedPlant(t, db, "Sensor Linkable")

	// Two sensor rows the plant can reference.
	_, err := db.Exec(`INSERT INTO sensors (id, name, zone_id, source, device, type)
	                   VALUES (1, 'A', 1, 'manual', 'M1', 't'), (2, 'B', 1, 'manual', 'M2', 't')`)
	require.NoError(t, err)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plant/link-sensors", apiKey,
		plantJSONBody(t, map[string]interface{}{
			"plant_id":   strconv.Itoa(plantID),
			"sensor_ids": []int{1, 2},
		}), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Plant.sensors should now hold the JSON-encoded array of IDs.
	var stored string
	require.NoError(t, db.QueryRow(`SELECT sensors FROM plant WHERE id = $1`, plantID).Scan(&stored))
	assert.Equal(t, "[1,2]", stored)
}

func TestPlantHTTP_LinkSensors_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodPost, c.BaseURL+"/plant/link-sensors", apiKey,
		bytes.NewBufferString(`not-json`), "application/json"))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeletePlant — additional coverage of the not-found / cascade path
// ---------------------------------------------------------------------------

// TestPlantHTTP_Delete_NoOpOnMissingPlant verifies DeletePlant returns
// 200 even when the row does not exist (the SQL DELETE is a no-op). The
// handler is forgiving by design.
func TestPlantHTTP_Delete_NoOpOnMissingPlant(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := plantTestSeed(t, db)

	c := server.NewClient(t)
	resp, err := c.Do(plantReq(t, http.MethodDelete, c.BaseURL+"/plant/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer plantDrain(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// LivingPlantsHandler / HarvestedPlantsHandler / DeadPlantsHandler
//
// These three live on the basic-routes group, gated by AuthMiddleware
// (session only — X-API-KEY is not accepted). Tests log in as admin so
// the cookie jar carries a session.
// ---------------------------------------------------------------------------

func TestPlantHTTP_LivingPlants_RequiresLogin(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	resp := c.Get("/plants/living")
	defer plantDrain(resp)
	// AuthMiddleware redirects unauthenticated GET requests to /login
	// (302). 401 would also be acceptable for an api endpoint, but this
	// route uses session middleware which redirects.
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

func TestPlantHTTP_LivingPlants_ReturnsActivePlants(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	plantTestSeed(t, db) // resets api_key but we don't use it; only need FK chain
	testutil.SeedAdmin(t, db, "living-pw")

	plantID := insertSeedPlant(t, db, "Alive Plant")
	_, err := db.Exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		plantID, plantStatusID(t, db, "Veg"))
	require.NoError(t, err)

	c := server.LoginAsAdmin(t, "living-pw")
	resp := c.Get("/plants/living")
	defer plantDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Alive Plant")
}

func TestPlantHTTP_HarvestedPlants_RequiresLogin(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	resp := c.Get("/plants/harvested")
	defer plantDrain(resp)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestPlantHTTP_HarvestedPlants_ReturnsHarvested(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	plantTestSeed(t, db)
	testutil.SeedAdmin(t, db, "harvest-pw")

	// Seed a plant in the "Success" status (active=0, status<>"Dead").
	plantID := insertSeedPlant(t, db, "Harvested Plant")
	_, err := db.Exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		plantID, plantStatusID(t, db, "Success"))
	require.NoError(t, err)

	c := server.LoginAsAdmin(t, "harvest-pw")
	resp := c.Get("/plants/harvested")
	defer plantDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Harvested Plant")
}

func TestPlantHTTP_DeadPlants_RequiresLogin(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	resp := c.Get("/plants/dead")
	defer plantDrain(resp)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestPlantHTTP_DeadPlants_ReturnsDead(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	plantTestSeed(t, db)
	testutil.SeedAdmin(t, db, "dead-pw")

	plantID := insertSeedPlant(t, db, "Dead Plant")
	_, err := db.Exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		plantID, plantStatusID(t, db, "Dead"))
	require.NoError(t, err)

	c := server.LoginAsAdmin(t, "dead-pw")
	resp := c.Get("/plants/dead")
	defer plantDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Dead Plant")
}

// ---------------------------------------------------------------------------
// Auth gating — the api-protected handlers
// ---------------------------------------------------------------------------

func TestPlantHTTP_AuthGating_APIRoutes(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plants"},
		{http.MethodPost, "/plant"},
		{http.MethodDelete, "/plant/delete/1"},
		{http.MethodPost, "/plant/link-sensors"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer plantDrain(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}
