package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type plantBody struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	Age       struct {
		Day int `json:"day"`
	} `json:"age"`
	Zone *struct {
		ID int `json:"id"`
	} `json:"zone"`
	Status struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Since string `json:"since"`
	} `json:"status"`
	HarvestWeightG float64 `json:"harvest_weight_g"`
	Sensors        []struct {
		ID        int  `json:"id"`
		Inherited bool `json:"inherited"`
	} `json:"sensors"`
	StatusHistory []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		At   string `json:"at"`
	} `json:"status_history"`
}

func TestPlantsWrite(t *testing.T) {
	t.Parallel()
	checkPlantsWrite(t, newFixture(t))
}

// checkPlantsWrite follows the §8 use case: create a plant today in the
// right zone as a Seedling, link its soil probe, move it on, correct the
// history, and finally delete it.
func checkPlantsWrite(t *testing.T, f fixture) {
	db := f.server.DB
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	mustExec(t, db, "UPDATE strain SET seed_count = 3 WHERE id = $1", strain)
	zone := testutil.SeedZone(t, db, "Tent")
	probe := testutil.SeedSensor(t, db, "ecowitt", "probe", "Soil.Moisture")
	mustExec(t, db, "UPDATE sensors SET zone_id = $1 WHERE id = $2", zone, probe)
	var seedling, veg int
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE status = 'Seedling'").Scan(&seedling))
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE status = 'Veg'").Scan(&veg))

	read, write, admin := f.key(t, "read"), f.key(t, "write"), f.key(t, "admin")
	w := bearer(write)

	body := `{"name":"Mango 1","strain_id":` + itoa(strain) + `,"zone_id":` + itoa(zone) +
		`,"status_id":` + itoa(seedling) + `,"start_date":"2026-11-10","decrement_seed_count":true}`
	resp := f.cc.do(http.MethodPost, "/plants", bearer(read), []byte(body))
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", resp.Body)

	resp = f.cc.do(http.MethodPost, "/plants", w, []byte(body))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	p := decode[plantBody](t, resp.Body)
	assert.Equal(t, "/api/v1/plants/"+itoa(p.ID), resp.Header.Get("Location"))
	assert.Equal(t, "2026-11-10", p.StartDate)
	assert.Equal(t, 1, p.Age.Day, "created today: Day 1")
	assert.Equal(t, "Seedling", p.Status.Name)
	assert.Equal(t, "2026-11-10T00:00:00-05:00", p.Status.Since)
	require.NotNil(t, p.Zone)
	assert.Equal(t, zone, p.Zone.ID)
	assert.Empty(t, p.Sensors, "soil probes aren't inherited from the zone")
	var seeds int
	require.NoError(t, db.QueryRow("SELECT seed_count FROM strain WHERE id = $1", strain).Scan(&seeds))
	assert.Equal(t, 2, seeds)
	for _, s := range f.store.Strains() {
		if s.ID == strain {
			assert.Equal(t, 2, s.SeedCount, "the strain cache sees the new count")
		}
	}

	for _, tc := range []struct{ body, field string }{
		{`{"strain_id":` + itoa(strain) + `,"status_id":1,"start_date":"2026-11-10"}`, "name"},
		{`{"name":"x","strain_id":99999,"status_id":1,"start_date":"2026-11-10"}`, "strain_id"},
		{`{"name":"x","strain_id":` + itoa(strain) + `,"status_id":1}`, "start_date"},
		{`{"name":"x","strain_id":` + itoa(strain) + `,"status_id":1,"start_date":"2026-11-10","sensor_ids":[99999]}`, "sensor_ids"},
		{`{"name":"x","strain_id":` + itoa(strain) + `,"status_id":1,"start_date":"2026-11-10","status_at":"soon"}`, "status_at"},
		{`{"name":"x","strain_id":` + itoa(strain) + `,"status_id":1,"start_date":"10/11/2026"}`, "body"},
	} {
		resp = f.cc.do(http.MethodPost, "/plants", w, []byte(tc.body))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", tc.body, resp.Body)
		assert.Contains(t, requireErrorEnvelope(t, resp.Body).Error.Details["fields"], tc.field, tc.body)
	}

	id := itoa(p.ID)
	resp = f.cc.do(http.MethodPut, "/plants/"+id+"/sensors", w, []byte(`{"sensor_ids":[`+itoa(probe)+`]}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	p = decode[plantBody](t, resp.Body)
	require.Len(t, p.Sensors, 1)
	assert.Equal(t, probe, p.Sensors[0].ID)
	assert.False(t, p.Sensors[0].Inherited)
	resp = f.cc.do(http.MethodPut, "/plants/"+id+"/sensors", w, []byte(`{}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)

	resp = f.cc.do(http.MethodPatch, "/plants/"+id, w, []byte(`{"name":"Mango #1","harvest_weight_g":0,"zone_id":null}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	p = decode[plantBody](t, resp.Body)
	assert.Equal(t, "Mango #1", p.Name)
	assert.Nil(t, p.Zone)
	assert.Equal(t, "2026-11-10", p.StartDate, "fields not sent are left alone")
	resp = f.cc.do(http.MethodPatch, "/plants/"+id, w, []byte(`{"start_date":"2026-11-01","harvest_weight_g":-1}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodPatch, "/plants/"+id, w, []byte(`{"start_date":"2026-11-01"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, 10, decode[plantBody](t, resp.Body).Age.Day)

	// Status: a change is 201, the same status again is 200 and unchanged.
	resp = f.cc.do(http.MethodPost, "/plants/"+id+"/status", w, []byte(`{"status_id":`+itoa(veg)+`,"at":"2026-11-10T18:30"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	assert.Contains(t, string(resp.Body), `"changed":true,"entry":{"id":`)
	assert.Contains(t, string(resp.Body), `"name":"Veg","at":"2026-11-10T18:30:00-05:00"}`)
	resp = f.cc.do(http.MethodPost, "/plants/"+id+"/status", w, []byte(`{"status_id":`+itoa(veg)+`}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Contains(t, string(resp.Body), `"changed":false`)

	resp = f.cc.get("/plants/"+id+"/status-history", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	history := decode[listBody[struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}]](t, resp.Body)
	require.Len(t, history.Items, 2)
	first, second := itoa(history.Items[0].ID), itoa(history.Items[1].ID)

	resp = f.cc.do(http.MethodPatch, "/plants/"+id+"/status-history/"+second, w, []byte(`{"at":"2026-11-09T12:00:00Z"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Contains(t, string(resp.Body), `"at":"2026-11-09T07:00:00-05:00"`)
	resp = f.cc.do(http.MethodPatch, "/plants/99999/status-history/"+second, w, []byte(`{"status_id":`+itoa(seedling)+`}`))
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", resp.Body)

	resp = f.cc.do(http.MethodDelete, "/plants/"+id+"/status-history/"+first, w, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/plants/"+id+"/status-history/"+second, w, nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "last_status_entry", requireErrorEnvelope(t, resp.Body).Error.Code)

	// Deleting needs admin.
	resp = f.cc.do(http.MethodDelete, "/plants/"+id, w, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodDelete, "/plants/"+id, bearer(admin), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.get("/plants/"+id, read)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var links int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM plant_sensor").Scan(&links))
	assert.Zero(t, links)
}
