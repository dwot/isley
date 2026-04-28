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

// settingsCRUDFixture sets up an API key and returns the plaintext.
func settingsCRUDFixture(t *testing.T, db *sql.DB) string {
	t.Helper()
	return testutil.SeedAPIKey(t, db, "test-settings-crud-key")
}

// readID parses the standard {"id": <int>} response shape.
func readID(t *testing.T, resp *http.Response) int {
	t.Helper()
	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return got.ID
}

// ---------------------------------------------------------------------------
// POST/PUT/DELETE /zones
// ---------------------------------------------------------------------------

func TestZoneCRUD_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)

	// POST → 201 + id
	resp := c.APIPostJSON(t, "/zones", apiKey, map[string]interface{}{"zone_name": "Tent A"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()

	// PUT → 200 + name updated
	resp = apiPutJSON(t, c, "/zones/"+strconv.Itoa(id), apiKey, map[string]interface{}{"zone_name": "Tent A renamed"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM zones WHERE id = $1`, id).Scan(&name))
	assert.Equal(t, "Tent A renamed", name)

	// DELETE → 200 + row gone
	resp = c.APIDelete(t, "/zones/"+strconv.Itoa(id), apiKey)
	resp.Body.Close()

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM zones WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

func TestZone_DeleteCascadesPlantsSensorsStreams(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	testutil.MustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Doomed Zone')`)
	testutil.MustExec(t, db, `INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	                    VALUES ('Plant in zone', 1, 1, '', 0, '2026-01-01', '[]')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (name, zone_id, source, device, type) VALUES ('Sensor in zone', 1, 'src', 'D', 'temp')`)
	testutil.MustExec(t, db, `INSERT INTO streams (name, url, zone_id, visible) VALUES ('Stream in zone', 'http://example/stream', 1, 1)`)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/zones/1", apiKey)
	resp.Body.Close()

	for _, table := range []string{"zones", "plant", "sensors", "streams"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
		assert.Zerof(t, n, "%s should be empty after zone delete cascade", table)
	}
}

func TestZone_AddRejectsEmptyName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/zones", apiKey, map[string]interface{}{"zone_name": ""})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST/PUT/DELETE /metrics
// ---------------------------------------------------------------------------

func TestMetric_CRUDHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)

	resp := c.APIPostJSON(t, "/metrics", apiKey, map[string]interface{}{
		"metric_name": "Width",
		"metric_unit": "in",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()

	// DELETE → 200 (Width is unlocked, so allowed).
	resp = c.APIDelete(t, "/metrics/"+strconv.Itoa(id), apiKey)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM metric WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

func TestMetric_DeleteLockedReturns400(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	// "Height" is seeded by the migration with lock=TRUE.
	var heightID int
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name = 'Height'`).Scan(&heightID))

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/metrics/"+strconv.Itoa(heightID), apiKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "locked built-in metric must NOT be deletable")
}

func TestMetric_AddRejectsEmptyName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/metrics", apiKey, map[string]interface{}{"metric_name": ""})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST/PUT/DELETE /activities
// ---------------------------------------------------------------------------

func TestActivity_CRUDHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{
		"activity_name": "Defoliate",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()

	// PUT updates name.
	resp = apiPutJSON(t, c, "/activities/"+strconv.Itoa(id), apiKey, map[string]interface{}{
		"activity_name": "Trim",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM activity WHERE id = $1`, id).Scan(&name))
	assert.Equal(t, "Trim", name)

	// DELETE.
	resp = c.APIDelete(t, "/activities/"+strconv.Itoa(id), apiKey)
	resp.Body.Close()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM activity WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

func TestActivity_RejectsReservedName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	c := server.NewClient(t)
	for _, reserved := range []string{"Water", "Feed", "Note"} {
		resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{
			"activity_name": reserved,
		})
		resp.Body.Close()
		assert.Equalf(t, http.StatusBadRequest, resp.StatusCode,
			"reserved name %q must be rejected", reserved)
	}
}

func TestActivity_DeleteCascadesPlantActivity(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := settingsCRUDFixture(t, db)

	// Set up a plant + an activity row that references a custom activity.
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	testutil.MustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                    VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO plant (id, name, zone_id, strain_id, description, clone, start_dt, sensors)
	                    VALUES (1, 'P', 1, 1, '', 0, '2026-01-01', '[]')`)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{"activity_name": "Defoliate"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	actID := readID(t, resp)
	resp.Body.Close()

	testutil.MustExec(t, db,
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES (1, $1, '', '2026-04-25')`,
		actID,
	)

	resp = c.APIDelete(t, "/activities/"+strconv.Itoa(actID), apiKey)
	resp.Body.Close()

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE activity_id = $1`, actID).Scan(&n))
	assert.Zero(t, n, "DeleteActivityHandler must cascade to plant_activity rows")
}
