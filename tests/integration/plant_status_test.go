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
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := int64(testutil.SeedPlant(t, db, "Plant 1", strainID, zoneID))

	var vegID, flowerID, dryingID int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Veg'`).Scan(&vegID))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Flower'`).Scan(&flowerID))
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status='Drying'`).Scan(&dryingID))

	_, err := db.Exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`, plantID, vegID)
	require.NoError(t, err)

	return statusFixture{
		APIKey:  testutil.SeedAPIKey(t, db, "test-status-key"),
		PlantID: plantID,
		VegID:   vegID, FlowerID: flowerID, DryingID: dryingID,
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
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plant/status", fix.APIKey, map[string]interface{}{
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
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	// Current status is already Veg — submitting Veg again should NOT
	// add a new row.
	resp := c.APIPostJSON(t, "/plant/status", fix.APIKey, map[string]interface{}{
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
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	// plant_id omitted (zero-value).
	resp := c.APIPostJSON(t, "/plant/status", fix.APIKey, map[string]interface{}{
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
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	var logID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status_log WHERE plant_id = $1`, fix.PlantID,
	).Scan(&logID))

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plantStatus/edit", fix.APIKey, map[string]interface{}{
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
	t.Parallel()

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
	resp := c.APIDelete(t, "/plantStatus/delete/"+strconv.Itoa(logID), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, 1, statusLogRowCount(t, db, fix.PlantID))
}

func TestPlantStatus_DeleteRejectsLastStatus(t *testing.T) {
	t.Parallel()

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
	resp := c.APIDelete(t, "/plantStatus/delete/"+strconv.Itoa(logID), fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"last-remaining status entry must NOT be deletable")
	assert.Equal(t, 1, statusLogRowCount(t, db, fix.PlantID), "row should still be present")
}

func TestPlantStatus_DeleteMissing(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStatusHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plantStatus/delete/9999", fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
