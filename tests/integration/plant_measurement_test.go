package integration

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// POST /plantMeasurement
// ---------------------------------------------------------------------------

func TestMeasurement_CreateHappyPath(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/plantMeasurement", fix.APIKey, map[string]interface{}{
		"plant_id":  fix.PlantID,
		"metric_id": fix.HeightID,
		"value":     12.5,
		"date":      "2026-04-25",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var v float64
	require.NoError(t, db.QueryRow(
		`SELECT value FROM plant_measurements WHERE plant_id = $1 AND metric_id = $2`,
		fix.PlantID, fix.HeightID,
	).Scan(&v))
	assert.InDelta(t, 12.5, v, 0.0001)
}

func TestMeasurement_CreateRejectsBadJSON(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	// value should be a number; passing a string makes JSON decode fail.
	resp := apiPostJSON(t, c, "/plantMeasurement", fix.APIKey, map[string]interface{}{
		"plant_id":  fix.PlantID,
		"metric_id": fix.HeightID,
		"value":     "not-a-number",
		"date":      "2026-04-25",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /plantMeasurement/edit
// ---------------------------------------------------------------------------

func TestMeasurement_EditUpdatesRow(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 5.0, '2026-04-01')`,
		fix.PlantID, fix.HeightID,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/plantMeasurement/edit", fix.APIKey, map[string]interface{}{
		"id":    id,
		"date":  "2026-04-15",
		"value": 7.5,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var v float64
	require.NoError(t, db.QueryRow(`SELECT value FROM plant_measurements WHERE id = $1`, id).Scan(&v))
	assert.InDelta(t, 7.5, v, 0.0001)
}

// ---------------------------------------------------------------------------
// DELETE /plantMeasurement/delete/:id
// ---------------------------------------------------------------------------

func TestMeasurement_DeleteRemovesRow(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 99.0, '2026-04-01')`,
		fix.PlantID, fix.HeightID,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/plantMeasurement/delete/"+strconv.FormatInt(id, 10), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_measurements WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}
