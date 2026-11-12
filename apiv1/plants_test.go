package apiv1_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := db.Exec(q, args...)
	require.NoError(t, err, q)
}

// ny is the stored form of a New York wall time (the fixtures' zone).
func ny(wall string) string { return testutil.StoredWall("America/New_York", wall) }

func addStatus(t *testing.T, db *sql.DB, plant int, status, at string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date)
		VALUES ($1, (SELECT id FROM plant_status WHERE status = $2), $3)`, plant, status, ny(at))
}

type plantIDs struct {
	growing, harvested, zone, strain int
}

// seedPlantsV1 makes a flowering plant and a harvested one. fixedNow puts
// "today" at 10 Nov 2026 in New York.
func seedPlantsV1(t *testing.T, db *sql.DB) plantIDs {
	t.Helper()
	var ids plantIDs
	breeder := testutil.SeedBreeder(t, db, "Breeder")
	ids.strain = testutil.SeedStrain(t, db, breeder, "Photo")
	mustExec(t, db, "UPDATE strain SET cycle_time = 60 WHERE id = $1", ids.strain)
	ids.zone = testutil.SeedZone(t, db, "Tent")

	ids.growing = testutil.SeedPlant(t, db, "Growing", ids.strain, ids.zone)
	mustExec(t, db, "UPDATE plant SET start_dt = '2026-10-01' WHERE id = $1", ids.growing)
	addStatus(t, db, ids.growing, "Seedling", "2026-10-01T08:00")
	addStatus(t, db, ids.growing, "Flower", "2026-10-25T10:00:00")
	var water, height, activity int
	require.NoError(t, db.QueryRow("SELECT id FROM activity WHERE name = 'Water'").Scan(&water))
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&height))
	require.NoError(t, db.QueryRow(`INSERT INTO plant_activity (plant_id, activity_id, note, date)
		VALUES ($1, $2, 'runoff', $3) RETURNING id`, ids.growing, water, ny("2026-11-09T20:00")).Scan(&activity))
	mustExec(t, db, `INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id)
		VALUES ($1, $2, 31.5, $3, $4)`, ids.growing, height, ny("2026-11-09T20:00"), activity)

	sensor := testutil.SeedSensor(t, db, "derived", itoa(ids.zone), "VPD")
	mustExec(t, db, "UPDATE sensors SET zone_id = $1 WHERE id = $2", ids.zone, sensor)
	mustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 1.2, '2026-11-11 02:55:00')", sensor)
	mustExec(t, db, "UPDATE plant_status SET vpd_low = 1.0, vpd_high = 1.5 WHERE status = 'Flower'")

	ids.harvested = testutil.SeedPlant(t, db, "Harvested", ids.strain, ids.zone)
	addStatus(t, db, ids.harvested, "Flower", "2026-09-01T09:00")
	addStatus(t, db, ids.harvested, "Drying", "2026-10-31T18:00")
	return ids
}

func TestPlantsRead(t *testing.T) {
	t.Parallel()
	checkPlantsRead(t, newFixture(t))
}

func checkPlantsRead(t *testing.T, f fixture) {
	ids := seedPlantsV1(t, f.server.DB)
	key := f.key(t, "read")

	resp := f.cc.get("/plants?phase=growing", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	list := decode[listBody[map[string]any]](t, resp.Body)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "Growing", list.Items[0]["name"])

	resp = f.cc.get("/plants/"+itoa(ids.growing), key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	body := string(resp.Body)
	for _, want := range []string{
		`"start_date":"2026-10-01"`,
		`"age":{"day":41,"week":6}`,
		`"status":{"id":`,
		`"name":"Flower","phase":"growing","since":"2026-10-25T10:00:00-04:00"}`,
		`"last_watered_at":"2026-11-09T20:00:00-05:00","days_since_watered":1`,
		`"last_fed_at":null,"days_since_fed":null`,
		`"flower":{"started_at":"2026-10-25T10:00:00-04:00","ended_at":null,"days":17}`,
		`"harvested_at":null`,
		`"estimated_harvest_date":"2026-12-24"`,
		`"zone":{"id":` + itoa(ids.zone) + `,"name":"Tent"}`,
		`"breeder":{"id":`,
		`"inherited":true,"latest":{"value":1.2,"at":"2026-11-10T21:55:00-05:00"},"vpd_target":{"low_kpa":1,"high_kpa":1.5,"state":"ideal"}`,
		`"note":"runoff"`,
		`"measurements":[{"id":`,
		`"metric":{"id":`,
		`"unit":"in","value":31.5`,
		`"name":"Seedling","days":24},{"status_id":`,
		`"name":"Flower","days":17}],"grow_days":41`,
	} {
		assert.Contains(t, body, want)
	}

	resp = f.cc.get("/plants/"+itoa(ids.harvested), key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	body = string(resp.Body)
	assert.Contains(t, body, `"start_date":"2026-01-01"`)
	assert.Contains(t, body, `"phase":"harvested"`)
	assert.Contains(t, body, `"flower":{"started_at":"2026-09-01T09:00:00-04:00","ended_at":"2026-10-31T18:00:00-04:00","days":60}`)
	assert.Contains(t, body, `"harvested_at":"2026-10-31T18:00:00-04:00"`)
	assert.Contains(t, body, `"recent_activities":[]`)

	for _, tc := range []struct{ query, want string }{
		{"phase=harvested", "Harvested"},
		{"zone_id=" + itoa(ids.zone) + "&phase=growing", "Growing"},
		{"strain_id=" + itoa(ids.strain) + "&limit=1", "Growing"},
	} {
		resp = f.cc.get("/plants?"+tc.query, key)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", tc.query, resp.Body)
		list = decode[listBody[map[string]any]](t, resp.Body)
		require.Len(t, list.Items, 1, tc.query)
		assert.Equal(t, tc.want, list.Items[0]["name"], tc.query)
	}
	resp = f.cc.get("/plants?limit=1", key)
	page := decode[listBody[map[string]any]](t, resp.Body)
	require.NotNil(t, page.NextCursor)
	resp = f.cc.get("/plants?limit=1&cursor="+*page.NextCursor, key)
	page = decode[listBody[map[string]any]](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Harvested", page.Items[0]["name"])
	assert.Nil(t, page.NextCursor)

	for _, q := range []string{"phase=alive", "zone_id=x", "status_id=0"} {
		resp = f.cc.get("/plants?"+q, key)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", q, resp.Body)
	}
	resp = f.cc.get("/plants/99999", key)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "plant_not_found", requireErrorEnvelope(t, resp.Body).Error.Code)
}
