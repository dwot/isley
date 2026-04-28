package integration

// +parallel:serial — handlers/sensors.go chart cache package-global
//
// TestSensors_GroupedReturnsLatestReading calls
// handlers.ResetGroupedSensorCache to scrub the process-global grouped
// sensor cache; the rest of the file shares the singleton even when it
// does not reset it. The chart-cache annotation is cleared by Phase 4.2
// of TEST_PLAN_2.md (SensorCacheService).

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

// sensorFixture seeds a single sensor + an API key.
type sensorFixture struct {
	APIKey   string
	SensorID int64
}

func seedSensorHTTP(t *testing.T, db *sql.DB) sensorFixture {
	t.Helper()
	testutil.SeedZone(t, db, "Z")
	res, err := db.Exec(
		`INSERT INTO sensors (name, zone_id, source, device, type, unit, visibility) VALUES ('Tent Temp', 1, 'src', 'D', 'temp', 'C', 'zone_plant')`,
	)
	require.NoError(t, err)
	sid, _ := res.LastInsertId()

	return sensorFixture{
		APIKey:   testutil.SeedAPIKey(t, db, "test-sensors-key"),
		SensorID: sid,
	}
}

// ---------------------------------------------------------------------------
// POST /sensors/edit
// ---------------------------------------------------------------------------

func TestSensors_EditUpdatesFields(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedSensorHTTP(t, db)

	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (2, 'Tent B')`)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/sensors/edit", fix.APIKey, map[string]interface{}{
		"id":         fix.SensorID,
		"name":       "Renamed Probe",
		"visibility": "zone",
		"zone_id":    2,
		"unit":       "F",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name, vis, unit string
	var zid int
	require.NoError(t, db.QueryRow(
		`SELECT name, visibility, zone_id, unit FROM sensors WHERE id = $1`, fix.SensorID,
	).Scan(&name, &vis, &zid, &unit))
	assert.Equal(t, "Renamed Probe", name)
	assert.Equal(t, "zone", vis)
	assert.Equal(t, 2, zid)
	assert.Equal(t, "F", unit)
}

func TestSensors_EditRejectsInvalidVisibility(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedSensorHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/sensors/edit", fix.APIKey, map[string]interface{}{
		"id":         fix.SensorID,
		"name":       "Anything",
		"visibility": "not-a-real-visibility",
		"zone_id":    1,
		"unit":       "C",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DELETE /sensors/delete/:id
// ---------------------------------------------------------------------------

func TestSensors_DeleteCascadesData(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedSensorHTTP(t, db)

	// Pre-seed two sensor_data rows so we can verify cascade.
	_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 1.0)`, fix.SensorID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 2.0)`, fix.SensorID)
	require.NoError(t, err)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/sensors/delete/"+strconv.FormatInt(fix.SensorID, 10), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var sensorCount, dataCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensors WHERE id = $1`, fix.SensorID).Scan(&sensorCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data WHERE sensor_id = $1`, fix.SensorID).Scan(&dataCount))
	assert.Zero(t, sensorCount)
	assert.Zero(t, dataCount, "delete must purge associated sensor_data")
}

// ---------------------------------------------------------------------------
// GET /sensors/grouped
// ---------------------------------------------------------------------------

func TestSensors_GroupedReturnsLatestReading(t *testing.T) {
	// The grouped-sensor cache is a process-global; clear it so an
	// earlier test's cache state doesn't bleed in.
	handlers.ResetGroupedSensorCache()

	db := testutil.NewTestDB(t)
	// Disable cache TTL pressure with a long polling interval.
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(storeWithPollingInterval(600)))
	fix := seedSensorHTTP(t, db)

	// Two readings — the latest should win.
	_, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 10.0)`, fix.SensorID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 20.0)`, fix.SensorID)
	require.NoError(t, err)

	testutil.SeedAdmin(t, db, "grouped-pw")
	c := server.LoginAsAdmin(t, "grouped-pw")

	resp := c.Get("/sensors/grouped")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]map[string][]map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))

	zoneEntries, ok := got["Z"]
	require.True(t, ok, "zone Z should appear in grouped response")
	devices, ok := zoneEntries["D"]
	require.True(t, ok)
	require.Len(t, devices, 1)
	assert.InDelta(t, 20.0, devices[0]["value"], 0.0001, "latest reading should be returned")
}
