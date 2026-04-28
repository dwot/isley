package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// seedPlantTreeWithKey seeds the FK chain (breeder → strain → zone) and
// an API key. Returns the api key plaintext for use as X-API-KEY in
// subsequent requests.
func seedPlantTreeWithKey(t *testing.T, db *sql.DB) string {
	t.Helper()
	testutil.SeedBreeder(t, db, "Test Breeder")
	testutil.SeedStrain(t, db, 1, "Test Strain")
	testutil.SeedZone(t, db, "Test Zone")
	return testutil.SeedAPIKey(t, db, "test-plant-api-key")
}

// ---------------------------------------------------------------------------
// POST /plants
// ---------------------------------------------------------------------------

func TestPlant_AddHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedPlantTreeWithKey(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plants", apiKey, map[string]interface{}{
		"name":      "Sapling 1",
		"zone_id":   1,
		"strain_id": 1,
		"status_id": statusIDByNameInt(t, db, "Veg"),
		"date":      "2026-04-25",
		"sensors":   "[]",
		"clone":     0,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "POST /plants happy path should be 200")

	var got struct {
		ID      int    `json:"id"`
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.ID, "response should carry the new plant id")

	// DB-level check: plant row exists.
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM plant WHERE id = $1`, got.ID).Scan(&name))
	assert.Equal(t, "Sapling 1", name)

	// And the initial status_log entry was inserted in the same tx.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_status_log WHERE plant_id = $1`, got.ID).Scan(&n))
	assert.Equal(t, 1, n, "AddPlant should write one plant_status_log entry")
}

func TestPlant_AddRejectsMissingName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedPlantTreeWithKey(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plants", apiKey, map[string]interface{}{
		// no "name"
		"zone_id":   1,
		"strain_id": 1,
		"status_id": statusIDByNameInt(t, db, "Veg"),
		"date":      "2026-04-25",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "missing name must surface as 400")
}

func TestPlant_AddDecrementsSeedCount(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedPlantTreeWithKey(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plants", apiKey, map[string]interface{}{
		"name":                 "Decrement Test",
		"zone_id":              1,
		"strain_id":            1,
		"status_id":            statusIDByNameInt(t, db, "Veg"),
		"date":                 "2026-04-25",
		"decrement_seed_count": true,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var seedCount int
	require.NoError(t, db.QueryRow(`SELECT seed_count FROM strain WHERE id = 1`).Scan(&seedCount))
	assert.Equal(t, 4, seedCount, "seed_count should drop from 5 to 4")
}

// ---------------------------------------------------------------------------
// DELETE /plant/delete/:id
// ---------------------------------------------------------------------------

func TestPlant_DeleteRemovesRow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedPlantTreeWithKey(t, db)

	// Insert a plant directly so we don't depend on the AddPlant flow.
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Doomed', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	plantID, err := res.LastInsertId()
	require.NoError(t, err)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plant/delete/"+strconv.FormatInt(plantID, 10), apiKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant WHERE id = $1`, plantID).Scan(&n))
	assert.Zero(t, n, "plant row must be gone after DELETE")
}

// ---------------------------------------------------------------------------
// GET /plants/living  (auth-gated session route)
// ---------------------------------------------------------------------------

func TestPlant_LivingPlantsLandsOnlyActiveStatuses(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	// Seed the FK chain plus two plants — one in an active status (Veg)
	// and one in an inactive status (Success).
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	testutil.MustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	          VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)

	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Alive', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	aliveID, _ := res.LastInsertId()
	res, err = db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Done', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	doneID, _ := res.LastInsertId()

	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		aliveID, statusIDByNameInt(t, db, "Veg"))
	testutil.MustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		doneID, statusIDByNameInt(t, db, "Success"))

	// Login as admin (basic routes are session-gated when guest mode is off).
	testutil.SeedAdmin(t, db, "living-pw")
	c := server.LoginAsAdmin(t, "living-pw")

	resp := c.Get("/plants/living")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Alive", "/plants/living should include the active plant")
	assert.NotContains(t, string(body), "\"name\":\"Done\"",
		"/plants/living should exclude inactive plants")
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlant_AddRequiresAuth(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	// No SeedAPIKey, no login — request should be rejected.

	c := server.NewClient(t)
	body := bytes.NewBufferString(`{"name":"foo","zone_id":1,"strain_id":1,"status_id":1,"date":"2026-04-25"}`)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/plants", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// CSRF rejects POSTs without csrf_token first; the request never
	// reaches AuthMiddlewareApi. Either 401 or 403 is "not allowed".
	assert.Containsf(t,
		[]int{http.StatusUnauthorized, http.StatusForbidden},
		resp.StatusCode,
		"unauthenticated POST /plants should be rejected (got %d)", resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GetAdjacentPlantIDs (prev/next plant navigation, PR #158)
// ---------------------------------------------------------------------------

// adjPlant inserts a plant with the given name and start_dt and writes a
// plant_status_log row linking it to statusName. Returns the plant id.
func adjPlant(t *testing.T, db *sql.DB, name, startDt, statusName string, strainID, zoneID int) int {
	t.Helper()
	pid := testutil.SeedPlant(t, db, name, strainID, zoneID)
	testutil.MustExec(t, db, `UPDATE plant SET start_dt = $1 WHERE id = $2`, startDt, pid)
	testutil.MustExec(t,
		db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`,
		pid, statusIDByNameInt(t, db, statusName), startDt,
	)
	return pid
}

func TestGetAdjacentPlantIDs(t *testing.T) {
	t.Parallel()

	t.Run("middle of partition", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		a := adjPlant(t, db, "A", "2026-01-01", "Veg", strainID, zoneID)
		b := adjPlant(t, db, "B", "2026-02-01", "Veg", strainID, zoneID)
		c := adjPlant(t, db, "C", "2026-03-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, b)
		assert.Equal(t, a, prev, "prev of B should be A")
		assert.Equal(t, c, next, "next of B should be C")
	})

	t.Run("first in partition", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		a := adjPlant(t, db, "A", "2026-01-01", "Veg", strainID, zoneID)
		b := adjPlant(t, db, "B", "2026-02-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, a)
		assert.Equal(t, 0, prev, "first plant has no prev")
		assert.Equal(t, b, next)
	})

	t.Run("last in partition", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		b := adjPlant(t, db, "B", "2026-01-01", "Veg", strainID, zoneID)
		c := adjPlant(t, db, "C", "2026-02-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, c)
		assert.Equal(t, b, prev)
		assert.Equal(t, 0, next, "last plant has no next")
	})

	t.Run("only plant in partition", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		a := adjPlant(t, db, "A", "2026-01-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, a)
		assert.Equal(t, 0, prev)
		assert.Equal(t, 0, next)
	})

	t.Run("plant does not exist", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)

		prev, next := handlers.GetAdjacentPlantIDs(db, 99999)
		assert.Equal(t, 0, prev)
		assert.Equal(t, 0, next)
	})

	t.Run("tie on start_dt orders by name", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		// Insert in reverse-name order so we can confirm the LAG/LEAD
		// ordering tiebreaker (name ASC) holds regardless of insert order.
		bb := adjPlant(t, db, "Bravo", "2026-01-01", "Veg", strainID, zoneID)
		aa := adjPlant(t, db, "Alpha", "2026-01-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, aa)
		assert.Equal(t, 0, prev, "Alpha (alphabetically first) should have no prev")
		assert.Equal(t, bb, next, "next of Alpha should be Bravo")

		prev, next = handlers.GetAdjacentPlantIDs(db, bb)
		assert.Equal(t, aa, prev)
		assert.Equal(t, 0, next)
	})

	t.Run("active and inactive partitions do not cross", func(t *testing.T) {
		t.Parallel()
		db := testutil.NewTestDB(t)
		breederID := testutil.SeedBreeder(t, db, "B")
		strainID := testutil.SeedStrain(t, db, breederID, "S")
		zoneID := testutil.SeedZone(t, db, "Z")

		// "Success" is an inactive status (active=0). The dead plant has
		// the earliest start_dt of any plant, so a naive query without
		// PARTITION BY would surface it as living's prev.
		_ = adjPlant(t, db, "Dead", "2025-12-01", "Success", strainID, zoneID)
		living := adjPlant(t, db, "Living", "2026-01-01", "Veg", strainID, zoneID)

		prev, next := handlers.GetAdjacentPlantIDs(db, living)
		assert.Equal(t, 0, prev, "living plant must not see the dead plant as prev")
		assert.Equal(t, 0, next)
	})
}

// ---------------------------------------------------------------------------
// helpers shared with this file
// ---------------------------------------------------------------------------

func statusIDByNameInt(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoErrorf(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = $1`, name).Scan(&id),
		"plant_status row %q must exist", name)
	return id
}
