package apiv1_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type sensorBody struct {
	ID   int `json:"id"`
	Zone *struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"zone"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Unit       string `json:"unit"`
	Visibility string `json:"visibility"`
	Latest     *struct {
		Value float64 `json:"value"`
		At    string  `json:"at"`
	} `json:"latest"`
}

func TestSensorsAPI(t *testing.T) {
	t.Parallel()
	checkSensorsAPI(t, newFixture(t))
}

func checkSensorsAPI(t *testing.T, f fixture) {
	db := f.server.DB
	read, write, admin := f.key(t, "read"), f.key(t, "write"), f.key(t, "admin")
	tent := testutil.SeedZone(t, db, "Tent")
	temp := testutil.SeedSensor(t, db, "acinfinity", "dev1", "Temperature")
	loose := testutil.SeedSensor(t, db, "custom", "probe", "Temperature")
	mustExec(t, db, "UPDATE sensors SET zone_id = $1 WHERE id = $2", tent, temp)
	mustExec(t, db, "UPDATE sensors SET zone_id = NULL WHERE id = $1", loose)
	mustExec(t, db, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES
		($1, 70.5, '2026-11-11 01:00:00'), ($1, 71.25, '2026-11-11 02:30:00')`, temp)

	// Every sensor is listed, including one in no zone.
	resp := f.cc.get("/sensors", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	list := decode[struct {
		Items []sensorBody `json:"items"`
	}](t, resp.Body)
	require.Len(t, list.Items, 2)
	assert.Equal(t, temp, list.Items[0].ID)
	assert.Equal(t, loose, list.Items[1].ID)
	assert.Nil(t, list.Items[1].Zone)
	assert.Nil(t, list.Items[1].Latest)

	resp = f.cc.get("/sensors?zone_id="+itoa(tent), read)
	list = decode[struct {
		Items []sensorBody `json:"items"`
	}](t, resp.Body)
	require.Len(t, list.Items, 1)
	resp = f.cc.get("/sensors?source=custom", read)
	list = decode[struct {
		Items []sensorBody `json:"items"`
	}](t, resp.Body)
	require.Len(t, list.Items, 1)
	assert.Equal(t, loose, list.Items[0].ID)

	resp = f.cc.get("/sensors/"+itoa(temp), read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got := decode[sensorBody](t, resp.Body)
	require.NotNil(t, got.Latest)
	assert.Equal(t, 71.25, got.Latest.Value)
	// Stored UTC, shown in the instance zone.
	assert.Equal(t, "2026-11-10T21:30:00-05:00", got.Latest.At)
	assert.Equal(t, "Tent", got.Zone.Name)

	// Changing a sensor needs admin.
	assert.Equal(t, http.StatusForbidden,
		f.cc.do(http.MethodPatch, "/sensors/"+itoa(temp), bearer(write), []byte(`{"name":"x"}`)).StatusCode)
	resp = f.cc.do(http.MethodPatch, "/sensors/"+itoa(temp), bearer(admin), []byte(`{"name":"Tent Temp","zone_id":null,"visibility":"zone"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	got = decode[sensorBody](t, resp.Body)
	assert.Equal(t, "Tent Temp", got.Name)
	assert.Nil(t, got.Zone)
	assert.Equal(t, "zone", got.Visibility)
	resp = f.cc.do(http.MethodPatch, "/sensors/"+itoa(temp), bearer(admin), []byte(`{"visibility":"sometimes","zone_id":9999}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	fields := requireErrorEnvelope(t, resp.Body).Error.Details["fields"].(map[string]any)
	assert.Contains(t, fields, "visibility")
	assert.Contains(t, fields, "zone_id")
	assert.Equal(t, http.StatusBadRequest,
		f.cc.do(http.MethodPatch, "/sensors/"+itoa(temp), bearer(admin), []byte(`{"source":"x"}`)).StatusCode,
		"identity fields can't be changed")

	assert.Equal(t, http.StatusNoContent, f.cc.do(http.MethodDelete, "/sensors/"+itoa(temp), bearer(admin), nil).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.get("/sensors/"+itoa(temp), read).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.do(http.MethodDelete, "/sensors/"+itoa(temp), bearer(admin), nil).StatusCode)
}

type sensorDataBody struct {
	Sensor struct {
		ID   int    `json:"id"`
		Unit string `json:"unit"`
	} `json:"sensor"`
	From       string           `json:"from"`
	To         string           `json:"to"`
	Bucket     string           `json:"bucket"`
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

func TestSensorDataAPI(t *testing.T) {
	t.Parallel()
	checkSensorDataAPI(t, newFixture(t))
}

func checkSensorDataAPI(t *testing.T, f fixture) {
	db := f.server.DB
	read := f.key(t, "read")
	temp := testutil.SeedSensor(t, db, "acinfinity", "dev1", "ACI.tempF")
	port := testutil.SeedSensor(t, db, "acinfinity", "dev1", "ACIP.2")
	// The fixture's clock is 2026-11-10 22:00 New York (03:00 UTC on the 11th).
	// Readings every 10 minutes from 20:00 to 21:50 New York.
	for i := 0; i < 12; i++ {
		utc := time.Date(2026, 11, 11, 1, 0, 0, 0, time.UTC).Add(time.Duration(i) * 10 * time.Minute).Format("2006-01-02 15:04:05")
		mustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)", temp, float64(60+i), utc)
		mustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)", port, float64(i%2*50), utc)
	}

	// Defaults: the last 24 hours, about 300 buckets (5 minutes).
	resp := f.cc.get("/sensors/"+itoa(temp)+"/data", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	got := decode[sensorDataBody](t, resp.Body)
	assert.Equal(t, "5m", got.Bucket)
	assert.Equal(t, "2026-11-10T22:00:00-05:00", got.To)
	assert.Equal(t, "2026-11-09T22:00:00-05:00", got.From)
	assert.Len(t, got.Items, 12)
	assert.Nil(t, got.NextCursor)

	resp = f.cc.get("/sensors/"+itoa(temp)+"/data?from=2026-11-10T20:00&to=2026-11-10T22:00&bucket=1h", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	got = decode[sensorDataBody](t, resp.Body)
	require.Len(t, got.Items, 2)
	assert.Equal(t, "2026-11-10T20:00:00-05:00", got.Items[0]["start"])
	assert.Equal(t, 62.5, got.Items[0]["avg"])
	assert.EqualValues(t, 6, got.Items[0]["count"])
	assert.NotContains(t, got.Items[0], "on_fraction")

	// Ports carry on_fraction and starts.
	resp = f.cc.get("/sensors/"+itoa(port)+"/data?from=2026-11-10T20:00&to=2026-11-10T22:00&bucket=1h", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	got = decode[sensorDataBody](t, resp.Body)
	require.Len(t, got.Items, 2)
	assert.Equal(t, 0.5, got.Items[0]["on_fraction"])
	assert.EqualValues(t, 3, got.Items[0]["starts"])

	// Raw, a page at a time.
	path := "/sensors/" + itoa(temp) + "/data?bucket=raw&limit=5&from=2026-11-10T20:00:00-05:00"
	var values []float64
	for pages := 0; ; pages++ {
		require.Less(t, pages, 5)
		resp = f.cc.get(path, read)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
		got = decode[sensorDataBody](t, resp.Body)
		assert.Equal(t, "raw", got.Bucket)
		for _, it := range got.Items {
			values = append(values, it["value"].(float64))
		}
		if got.NextCursor == nil {
			break
		}
		path = "/sensors/" + itoa(temp) + "/data?bucket=raw&limit=5&from=2026-11-10T20:00:00-05:00&cursor=" + *got.NextCursor
	}
	require.Len(t, values, 12)
	assert.Equal(t, 60.0, values[0])
	assert.Equal(t, 71.0, values[11])

	for _, q := range []string{"bucket=soon", "bucket=10s", "limit=0", "cursor=zzz", "from=yesterday",
		"from=2026-11-10T22:00&to=2026-11-10T20:00", "bucket=1m&from=2025-01-01T00:00"} {
		resp = f.cc.get("/sensors/"+itoa(temp)+"/data?"+q, read)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", q, resp.Body)
	}
	assert.Equal(t, http.StatusNotFound, f.cc.get("/sensors/9999/data", read).StatusCode)
}
