package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type activityBody struct {
	ID    int `json:"id"`
	Plant struct {
		ID int `json:"id"`
	} `json:"plant"`
	ActivityType struct {
		Name string `json:"name"`
	} `json:"activity_type"`
	Note         string `json:"note"`
	At           string `json:"at"`
	Measurements []struct {
		ID    int     `json:"id"`
		Value float64 `json:"value"`
		At    string  `json:"at"`
	} `json:"measurements"`
}

func TestActivities(t *testing.T) {
	t.Parallel()
	checkActivities(t, newFixture(t))
}

func checkActivities(t *testing.T, f fixture) {
	db := f.server.DB
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	zone := testutil.SeedZone(t, db, "Tent")
	a := testutil.SeedPlant(t, db, "A", strain, zone)
	b := testutil.SeedPlant(t, db, "B", strain, zone)
	var water, height, ph int
	require.NoError(t, db.QueryRow("SELECT id FROM activity WHERE name = 'Water'").Scan(&water))
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&height))
	require.NoError(t, db.QueryRow("INSERT INTO metric (name, unit) VALUES ('pH', '') RETURNING id").Scan(&ph))
	var flush int
	require.NoError(t, db.QueryRow("INSERT INTO activity (name, is_watering) VALUES ('Flush', TRUE) RETURNING id").Scan(&flush))
	mustExec(t, db, "INSERT INTO activity_metric (activity_id, metric_id, required) VALUES ($1, $2, TRUE)", flush, ph)
	// An old activity stored as a bare date, the way some versions did.
	mustExec(t, db, "INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'old', $3)", a, water, ny("2026-11-01"))

	read, write := f.key(t, "read"), f.key(t, "write")
	w := bearer(write)

	resp := f.cc.do(http.MethodPost, "/plants/"+itoa(a)+"/activities", w,
		[]byte(`{"activity_type_id":`+itoa(water)+`,"note":"runoff","at":"2026-11-10T08:00","measurements":[{"metric_id":`+itoa(height)+`,"value":31.5}]}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	act := decode[activityBody](t, resp.Body)
	assert.Equal(t, "/api/v1/activities/"+itoa(act.ID), resp.Header.Get("Location"))
	assert.Equal(t, a, act.Plant.ID)
	assert.Equal(t, "Water", act.ActivityType.Name)
	assert.Equal(t, "2026-11-10T08:00:00-05:00", act.At)
	require.Len(t, act.Measurements, 1)
	assert.Equal(t, act.At, act.Measurements[0].At, "measurements take the activity's time")

	// Without at, it's now (fixedNow: 22:00 on the 10th in New York).
	resp = f.cc.do(http.MethodPost, "/plants/"+itoa(b)+"/activities", w, []byte(`{"activity_type_id":`+itoa(water)+`}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "2026-11-10T22:00:00-05:00", decode[activityBody](t, resp.Body).At)

	for _, tc := range []struct{ path, body, field string }{
		{"/plants/" + itoa(a) + "/activities", `{}`, "activity_type_id"},
		{"/plants/" + itoa(a) + "/activities", `{"activity_type_id":99999}`, "activity_type_id"},
		{"/plants/" + itoa(a) + "/activities", `{"activity_type_id":` + itoa(flush) + `}`, "measurements"},
		{"/plants/" + itoa(a) + "/activities", `{"activity_type_id":` + itoa(water) + `,"at":"yesterday"}`, "at"},
		{"/activities/bulk", `{"activity_type_id":` + itoa(water) + `,"plant_ids":[]}`, "plant_ids"},
		{"/activities/bulk", `{"activity_type_id":` + itoa(water) + `,"plant_ids":[` + itoa(a) + `,99999]}`, "plant_ids"},
	} {
		resp = f.cc.do(http.MethodPost, tc.path, w, []byte(tc.body))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", tc.body, resp.Body)
		assert.Contains(t, requireErrorEnvelope(t, resp.Body).Error.Details["fields"], tc.field, tc.body)
	}
	resp = f.cc.do(http.MethodPost, "/plants/99999/activities", w, []byte(`{"activity_type_id":`+itoa(water)+`}`))
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)

	resp = f.cc.do(http.MethodPost, "/activities/bulk", w,
		[]byte(`{"activity_type_id":`+itoa(flush)+`,"plant_ids":[`+itoa(a)+`,`+itoa(b)+`],"at":"2026-11-09T19:00","measurements":[{"metric_id":`+itoa(ph)+`,"value":6.2}]}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	bulk := decode[listBody[activityBody]](t, resp.Body)
	require.Len(t, bulk.Items, 2)
	assert.Equal(t, []int{a, b}, []int{bulk.Items[0].Plant.ID, bulk.Items[1].Plant.ID})

	// The log: 5 activities in all, newest first.
	resp = f.cc.get("/activities", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	all := decode[listBody[activityBody]](t, resp.Body)
	require.Len(t, all.Items, 5)
	assert.Equal(t, "2026-11-10T22:00:00-05:00", all.Items[0].At)
	assert.Equal(t, "old", all.Items[4].Note)

	count := func(query string) int {
		t.Helper()
		resp := f.cc.get("/activities?"+query, read)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", query, resp.Body)
		return len(decode[listBody[activityBody]](t, resp.Body).Items)
	}
	assert.Equal(t, 1, count("from=2026-11-01&to=2026-11-01"), "a midnight row counts on its day")
	assert.Equal(t, 2, count("from=2026-11-09&to=2026-11-09"))
	assert.Equal(t, 4, count("from=2026-11-09"))
	assert.Equal(t, 3, count("plant_id="+itoa(a)))
	assert.Equal(t, 2, count("activity_type_id="+itoa(flush)))
	assert.Equal(t, 5, count("activity_type_id="+itoa(flush)+","+itoa(water)))
	assert.Equal(t, 1, count("q=RUNOFF"))
	assert.Equal(t, 5, count("zone_id="+itoa(zone)))

	resp = f.cc.get("/activities?limit=2&order=oldest", read)
	page := decode[listBody[activityBody]](t, resp.Body)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "old", page.Items[0].Note)
	require.NotNil(t, page.NextCursor)
	seen := 2
	for page.NextCursor != nil {
		resp = f.cc.get("/activities?limit=2&order=oldest&cursor="+*page.NextCursor, read)
		page = decode[listBody[activityBody]](t, resp.Body)
		seen += len(page.Items)
	}
	assert.Equal(t, 5, seen)
	for _, q := range []string{"from=11/09/2026", "order=sideways", "activity_type_id=x", "plant_id=-1"} {
		resp = f.cc.get("/activities?"+q, read)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", q, resp.Body)
	}

	resp = f.cc.get("/plants/"+itoa(b)+"/activities", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Len(t, decode[listBody[activityBody]](t, resp.Body).Items, 2)

	// Change and delete.
	id := itoa(act.ID)
	resp = f.cc.do(http.MethodPatch, "/activities/"+id, w, []byte(`{"note":"heavy runoff","at":"2026-11-10T09:15:00-05:00"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	act = decode[activityBody](t, resp.Body)
	assert.Equal(t, "heavy runoff", act.Note)
	require.Len(t, act.Measurements, 1)
	assert.Equal(t, "2026-11-10T09:15:00-05:00", act.Measurements[0].At, "measurements move with the activity")
	resp = f.cc.do(http.MethodPatch, "/activities/"+id, w, []byte(`{"measurements":[]}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Empty(t, decode[activityBody](t, resp.Body).Measurements)
	resp = f.cc.get("/activities/"+id, read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/activities/"+id, w, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.get("/activities/"+id, read)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "activity_not_found", requireErrorEnvelope(t, resp.Body).Error.Code)
}

func TestMeasurements(t *testing.T) {
	t.Parallel()
	checkMeasurements(t, newFixture(t))
}

func checkMeasurements(t *testing.T, f fixture) {
	db := f.server.DB
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	plant := testutil.SeedPlant(t, db, "A", strain, testutil.SeedZone(t, db, "Tent"))
	var height int
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&height))
	read, w := f.key(t, "read"), bearer(f.key(t, "write"))
	path := "/plants/" + itoa(plant) + "/measurements"

	var ids []int
	for _, body := range []string{
		`{"metric_id":` + itoa(height) + `,"value":12,"at":"2026-11-01T12:00"}`,
		`{"metric_id":` + itoa(height) + `,"value":14.5}`,
	} {
		resp := f.cc.do(http.MethodPost, path, w, []byte(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
		m := decode[struct {
			ID   int    `json:"id"`
			Unit string `json:"unit"`
		}](t, resp.Body)
		assert.Equal(t, "in", m.Unit)
		ids = append(ids, m.ID)
	}
	for _, body := range []string{`{"metric_id":` + itoa(height) + `}`, `{"metric_id":99999,"value":1}`} {
		resp := f.cc.do(http.MethodPost, path, w, []byte(body))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", body, resp.Body)
	}

	resp := f.cc.get(path+"?limit=1", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	page := decode[listBody[struct {
		ID    int     `json:"id"`
		Value float64 `json:"value"`
	}]](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 14.5, page.Items[0].Value, "newest first")
	require.NotNil(t, page.NextCursor)
	resp = f.cc.get(path+"?limit=1&cursor="+*page.NextCursor+"&metric_id="+itoa(height), read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	page = decode[listBody[struct {
		ID    int     `json:"id"`
		Value float64 `json:"value"`
	}]](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 12.0, page.Items[0].Value)
	assert.Nil(t, page.NextCursor)

	resp = f.cc.do(http.MethodPatch, "/measurements/"+itoa(ids[0]), w, []byte(`{"value":12.25,"at":"2026-11-02T08:00"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Contains(t, string(resp.Body), `"value":12.25,"at":"2026-11-02T08:00:00-05:00"`)
	resp = f.cc.do(http.MethodPatch, "/measurements/"+itoa(ids[0]), w, []byte(`{"value":null}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/measurements/"+itoa(ids[0]), w, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/measurements/"+itoa(ids[0]), w, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "measurement_not_found", requireErrorEnvelope(t, resp.Body).Error.Code)
}

// A retried POST with the same Idempotency-Key logs one watering, not two.
func TestIdempotencyKey(t *testing.T) {
	t.Parallel()
	checkIdempotencyKey(t, newFixture(t))
}

func checkIdempotencyKey(t *testing.T, f fixture) {
	db := f.server.DB
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	plant := testutil.SeedPlant(t, db, "A", strain, testutil.SeedZone(t, db, "Tent"))
	var water int
	require.NoError(t, db.QueryRow("SELECT id FROM activity WHERE name = 'Water'").Scan(&water))
	path := "/plants/" + itoa(plant) + "/activities"
	body := []byte(`{"activity_type_id":` + itoa(water) + `,"note":"retry me"}`)
	write := f.key(t, "write")
	h := map[string]string{"Authorization": "Bearer " + write, "Idempotency-Key": "water-2026-11-10-a"}

	first := f.cc.do(http.MethodPost, path, h, body)
	require.Equal(t, http.StatusCreated, first.StatusCode, "%s", first.Body)
	assert.Empty(t, first.Header.Get("Idempotent-Replayed"))
	again := f.cc.do(http.MethodPost, path, h, body)
	require.Equal(t, http.StatusCreated, again.StatusCode, "%s", again.Body)
	assert.Equal(t, "true", again.Header.Get("Idempotent-Replayed"))
	assert.Equal(t, string(first.Body), string(again.Body))
	assert.Equal(t, first.Header.Get("Location"), again.Header.Get("Location"))
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM plant_activity WHERE plant_id = $1", plant).Scan(&n))
	assert.Equal(t, 1, n)

	resp := f.cc.do(http.MethodPost, path, h, []byte(`{"activity_type_id":`+itoa(water)+`,"note":"different"}`))
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "idempotency_key_reused", requireErrorEnvelope(t, resp.Body).Error.Code)

	// Keys are per API key: another key may use the same string.
	other := map[string]string{"Authorization": "Bearer " + f.key(t, "admin"), "Idempotency-Key": "water-2026-11-10-a"}
	resp = f.cc.do(http.MethodPost, path, other, body)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)

	// A failed request frees its key for a corrected retry.
	bad := map[string]string{"Authorization": "Bearer " + write, "Idempotency-Key": "k2"}
	resp = f.cc.do(http.MethodPost, path, bad, []byte(`{"activity_type_id":99999}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodPost, path, bad, []byte(`{"activity_type_id":99999}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "a failed request is not replayed: %s", resp.Body)
	assert.Empty(t, resp.Header.Get("Idempotent-Replayed"))
}
