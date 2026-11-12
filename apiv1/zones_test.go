package apiv1_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type listBody[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(body, &v), "body: %s", body)
	return v
}

type zoneBody struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	LeafTempOffsetF     *float64 `json:"leaf_temp_offset_f"`
	VPDTempSensorID     *int     `json:"vpd_temp_sensor_id"`
	VPDHumiditySensorID *int     `json:"vpd_humidity_sensor_id"`
	Sensors             []struct {
		ID     int    `json:"id"`
		Source string `json:"source"`
		Type   string `json:"type"`
		Latest *struct {
			Value float64 `json:"value"`
			At    string  `json:"at"`
		} `json:"latest"`
	} `json:"sensors"`
}

func TestLookups(t *testing.T) {
	t.Parallel()
	checkLookups(t, newFixture(t))
}

func checkLookups(t *testing.T, f fixture) {
	key := f.key(t, "read")

	resp := f.cc.get("/statuses", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	statuses := decode[listBody[struct {
		Name  string `json:"name"`
		Phase string `json:"phase"`
	}]](t, resp.Body)
	require.Nil(t, statuses.NextCursor)
	var names, phases []string
	for _, s := range statuses.Items {
		names = append(names, s.Name)
		phases = append(phases, s.Phase)
	}
	assert.Equal(t, []string{"Germinating", "Planted", "Seedling", "Veg", "Flower", "Drying", "Curing", "Success", "Dead"}, names)
	assert.Equal(t, []string{"growing", "growing", "growing", "growing", "growing", "harvested", "harvested", "harvested", "dead"}, phases)

	resp = f.cc.get("/metrics", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	metrics := decode[listBody[struct {
		Name    string `json:"name"`
		Unit    string `json:"unit"`
		BuiltIn bool   `json:"built_in"`
	}]](t, resp.Body)
	require.NotEmpty(t, metrics.Items)
	assert.Equal(t, "Height", metrics.Items[0].Name)
	assert.True(t, metrics.Items[0].BuiltIn)

	resp = f.cc.get("/activity-types", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	activities := decode[listBody[struct {
		Name       string `json:"name"`
		IsWatering bool   `json:"is_watering"`
		BuiltIn    bool   `json:"built_in"`
	}]](t, resp.Body)
	builtIn := map[string]bool{}
	for _, a := range activities.Items {
		if a.BuiltIn {
			builtIn[a.Name] = true
		}
	}
	assert.Equal(t, map[string]bool{"Water": true, "Feed": true, "Note": true}, builtIn)
}

// An activity type's metrics come back with their names and units.
func TestActivityTypeMetrics(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	db := f.server.DB
	var activityID, metricID int
	require.NoError(t, db.QueryRow("INSERT INTO activity (name) VALUES ('Defoliate') RETURNING id").Scan(&activityID))
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&metricID))
	_, err := db.Exec("INSERT INTO activity_metric (activity_id, metric_id, required) VALUES ($1, $2, TRUE)", activityID, metricID)
	require.NoError(t, err)

	resp := f.cc.get("/activity-types", f.key(t, "read"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Contains(t, string(resp.Body),
		`"name":"Defoliate","is_watering":false,"is_feeding":false,"built_in":false,"metrics":[{"metric_id":`)
	assert.Contains(t, string(resp.Body), `"name":"Height","unit":"in","required":true}]`)
}

func TestZonesRead(t *testing.T) {
	t.Parallel()
	checkZonesRead(t, newFixture(t))
}

func checkZonesRead(t *testing.T, f fixture) {
	db := f.server.DB
	key := f.key(t, "read")

	resp := f.cc.get("/zones", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.JSONEq(t, `{"items":[],"next_cursor":null}`, string(resp.Body))

	tent := testutil.SeedZone(t, db, "Tent")
	testutil.SeedZone(t, db, "Room")
	testutil.SeedZone(t, db, "Bed")

	// Page through one at a time.
	var seen []string
	path := "/zones?limit=1"
	for i := 0; i < 5; i++ {
		resp = f.cc.get(path, key)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
		page := decode[listBody[zoneBody]](t, resp.Body)
		for _, z := range page.Items {
			seen = append(seen, z.Name)
		}
		if page.NextCursor == nil {
			break
		}
		path = "/zones?limit=1&cursor=" + *page.NextCursor
	}
	assert.Equal(t, []string{"Tent", "Room", "Bed"}, seen)

	// Detail: a sensor with readings and one without.
	temp := testutil.SeedSensor(t, db, "acinfinity", "dev1", "Temperature")
	quiet := testutil.SeedSensor(t, db, "acinfinity", "dev1", "Humidity")
	_, err := db.Exec("UPDATE sensors SET zone_id = $1 WHERE id IN ($2, $3)", tent, temp, quiet)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES
		($1, 70.5, '2026-11-11 01:00:00'), ($1, 71.25, '2026-11-11 02:30:00')`, temp)
	require.NoError(t, err)

	resp = f.cc.get("/zones/"+itoa(tent), key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	z := decode[zoneBody](t, resp.Body)
	require.Len(t, z.Sensors, 2)
	byType := map[string]int{}
	for i, s := range z.Sensors {
		byType[s.Type] = i
	}
	latest := z.Sensors[byType["Temperature"]].Latest
	require.NotNil(t, latest)
	assert.Equal(t, 71.25, latest.Value)
	// Stored UTC (CURRENT_TIMESTAMP's zone), shown in the instance zone.
	assert.Equal(t, "2026-11-10T21:30:00-05:00", latest.At)
	assert.Nil(t, z.Sensors[byType["Humidity"]].Latest)

	for _, path := range []string{"/zones/9999", "/zones/abc"} {
		resp = f.cc.get(path, key)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", path, resp.Body)
		assert.Equal(t, "zone_not_found", requireErrorEnvelope(t, resp.Body).Error.Code)
	}

	resp = f.cc.get("/zones?limit=0", key)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.get("/zones?cursor=bm9wZQ", key) // "nope"
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
}

func TestZonesWrite(t *testing.T) {
	t.Parallel()
	checkZonesWrite(t, newFixture(t))
}

func checkZonesWrite(t *testing.T, f fixture) {
	db := f.server.DB
	admin := f.key(t, "admin")
	write := f.key(t, "write")
	h := bearer(admin)

	// Zone writes need admin.
	resp := f.cc.do(http.MethodPost, "/zones", bearer(write), []byte(`{"name":"Tent"}`))
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", resp.Body)

	resp = f.cc.do(http.MethodPost, "/zones", h, []byte(`{"name":"  Tent 1 "}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	created := decode[zoneBody](t, resp.Body)
	assert.Equal(t, "Tent 1", created.Name)
	assert.Equal(t, "/api/v1/zones/"+itoa(created.ID), resp.Header.Get("Location"))
	assert.Empty(t, created.Sensors)
	assert.True(t, storeHasZone(f, "Tent 1"), "the config store sees the new zone")

	for _, tc := range []struct {
		body  string
		field string
	}{
		{`{"name":""}`, "name"},
		{`{}`, "name"},
		{`{"name":"x","colour":"green"}`, "body"},
		{`{"name":"x","vpd_temp_sensor_id":9999}`, "vpd_temp_sensor_id"},
		{`{"name":12}`, "body"},
		{`not json`, "body"},
	} {
		resp = f.cc.do(http.MethodPost, "/zones", h, []byte(tc.body))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", tc.body, resp.Body)
		env := requireErrorEnvelope(t, resp.Body)
		assert.Equal(t, "invalid_request", env.Error.Code)
		assert.Contains(t, env.Error.Details["fields"], tc.field, "%s", tc.body)
	}

	// Turning VPD on adds the derived VPD sensor.
	id := itoa(created.ID)
	resp = f.cc.do(http.MethodPatch, "/zones/"+id, h, []byte(`{"leaf_temp_offset_f":3.6}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	z := decode[zoneBody](t, resp.Body)
	assert.Equal(t, "Tent 1", z.Name, "fields not sent are left alone")
	require.NotNil(t, z.LeafTempOffsetF)
	assert.Equal(t, 3.6, *z.LeafTempOffsetF)
	require.Len(t, z.Sensors, 1)
	assert.Equal(t, "derived", z.Sensors[0].Source)
	assert.Equal(t, "VPD", z.Sensors[0].Type)

	// Renaming leaves the offset; null clears it.
	resp = f.cc.do(http.MethodPatch, "/zones/"+id, h, []byte(`{"name":"Tent A"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	z = decode[zoneBody](t, resp.Body)
	assert.Equal(t, "Tent A", z.Name)
	assert.NotNil(t, z.LeafTempOffsetF)
	resp = f.cc.do(http.MethodPatch, "/zones/"+id, h, []byte(`{"leaf_temp_offset_f":null}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Nil(t, decode[zoneBody](t, resp.Body).LeafTempOffsetF)
	assert.True(t, storeHasZone(f, "Tent A"))

	resp = f.cc.do(http.MethodPatch, "/zones/"+id, h, []byte(`{"name":null}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodPatch, "/zones/9999", h, []byte(`{"name":"x"}`))
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)

	// A zone with something in it can't be deleted.
	resp = f.cc.do(http.MethodDelete, "/zones/"+id, h, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "zone_not_empty", requireErrorEnvelope(t, resp.Body).Error.Code)

	_, err := db.Exec("DELETE FROM sensors WHERE zone_id = $1", created.ID)
	require.NoError(t, err)
	resp = f.cc.do(http.MethodDelete, "/zones/"+id, h, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	assert.Empty(t, resp.Body)
	assert.False(t, storeHasZone(f, "Tent A"))

	resp = f.cc.get("/zones/"+id, admin)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/zones/"+id, h, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)
}

func storeHasZone(f fixture, name string) bool {
	for _, z := range f.store.Zones() {
		if z.Name == name {
			return true
		}
	}
	return false
}

func itoa(n int) string { return strconv.Itoa(n) }
