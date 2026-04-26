package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// statusFixture seeds the FK chain, a plant, an existing Veg status
// log entry, and an API key.
type statusFixture struct {
	APIKey   string
	PlantID  int64
	VegID    int
	FlowerID int
	DryingID int
}

func seedStatusHTTP(t *testing.T, db *sql.DB) statusFixture {
	t.Helper()
	exec := func(query string, args ...interface{}) sql.Result {
		res, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
		return res
	}
	exec(`INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	exec(`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	      VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	exec(`INSERT INTO zones (id, name) VALUES (1, 'Z')`)

	res := exec(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	             VALUES ('Plant 1', 1, 1, '', 0, '2026-01-01', '[]')`)
	plantID, _ := res.LastInsertId()

	var vegID, flowerID, dryingID int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Veg'`).Scan(&vegID))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Flower'`).Scan(&flowerID))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Drying'`).Scan(&dryingID))

	exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`, plantID, vegID)

	const plaintext = "test-status-key"
	seedAPIKey(t, db, handlers.HashAPIKey(plaintext))
	return statusFixture{
		APIKey: plaintext, PlantID: plantID,
		VegID: vegID, FlowerID: flowerID, DryingID: dryingID,
	}
}

func statusLogRowCount(t *testing.T, db *sql.DB, plantID int64) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_status_log WHERE plant_id = $1`, plantID).Scan(&n))
	return n
}

// ---------------------------------------------------------------------------
// POST /plant/status (UpdatePlantStatus)
// ---------------------------------------------------------------------------

func TestPlantStatus_UpdateInsertsNewLogWhenChanged(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/plant/status", fix.APIKey, map[string]interface{}{
		"plant_id":  fix.PlantID,
		"status_id": fix.FlowerID,
		"date":      "2026-02-01",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Updated bool `json:"updated"`
		ID      int  `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.True(t, got.Updated)
	assert.NotZero(t, got.ID)
	assert.Equal(t, 2, statusLogRowCount(t, db, fix.PlantID), "new log entry should be added")
}

func TestPlantStatus_UpdateNoOpWhenSameStatus(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	// Current status is already Veg — submitting Veg again should NOT
	// add a new row.
	resp := apiPostJSON(t, c, "/plant/status", fix.APIKey, map[string]interface{}{
		"plant_id":  fix.PlantID,
		"status_id": fix.VegID,
		"date":      "2026-01-20",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Updated bool `json:"updated"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.False(t, got.Updated, "same-status submission must be reported as no-op")
	assert.Equal(t, 1, statusLogRowCount(t, db, fix.PlantID), "row count unchanged")
}

func TestPlantStatus_UpdateRejectsMissingFields(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	// plant_id omitted (zero-value).
	resp := apiPostJSON(t, c, "/plant/status", fix.APIKey, map[string]interface{}{
		"status_id": fix.FlowerID,
		"date":      "2026-02-01",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /plantStatus/edit (EditStatus)
// ---------------------------------------------------------------------------

func TestPlantStatus_EditUpdatesDate(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	var logID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status_log WHERE plant_id = $1`, fix.PlantID,
	).Scan(&logID))

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/plantStatus/edit", fix.APIKey, map[string]interface{}{
		"id":   logID,
		"date": "2026-03-15",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var date string
	require.NoError(t, db.QueryRow(`SELECT date FROM plant_status_log WHERE id = $1`, logID).Scan(&date))
	assert.Contains(t, date, "2026-03-15")
}

// ---------------------------------------------------------------------------
// DELETE /plantStatus/delete/:id (DeleteStatus)
// ---------------------------------------------------------------------------

func TestPlantStatus_DeleteRemovesEntry(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	// Add a second status entry so we can delete one without hitting
	// the "last status" guard.
	_, err := db.Exec(
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-02-01')`,
		fix.PlantID, fix.FlowerID,
	)
	require.NoError(t, err)
	require.Equal(t, 2, statusLogRowCount(t, db, fix.PlantID))

	// Find the older Veg row (we'll delete it).
	var logID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status_log WHERE plant_id = $1 AND status_id = $2`,
		fix.PlantID, fix.VegID,
	).Scan(&logID))

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/plantStatus/delete/"+strconv.Itoa(logID), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, 1, statusLogRowCount(t, db, fix.PlantID))
}

func TestPlantStatus_DeleteRejectsLastStatus(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	// Only one status entry exists from the fixture — deletion must be
	// blocked.
	var logID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status_log WHERE plant_id = $1`, fix.PlantID,
	).Scan(&logID))

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/plantStatus/delete/"+strconv.Itoa(logID), fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"last-remaining status entry must NOT be deletable")
	assert.Equal(t, 1, statusLogRowCount(t, db, fix.PlantID), "row should still be present")
}

func TestPlantStatus_DeleteMissing(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/plantStatus/delete/9999", fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
