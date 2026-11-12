package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type smokeBody struct {
	ID         int    `json:"id"`
	ReportDate string `json:"report_date"`
	Rating     int    `json:"rating"`
	CureDays   *int   `json:"cure_days"`
	Scores     []struct {
		Criterion struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"criterion"`
		Value int `json:"value"`
	} `json:"scores"`
}

func TestSmokeReportsAPI(t *testing.T) {
	t.Parallel()
	checkSmokeReportsAPI(t, newFixture(t))
}

func checkSmokeReportsAPI(t *testing.T, f fixture) {
	db := f.server.DB
	read, write := f.key(t, "read"), f.key(t, "write")
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "Blue")
	plant := testutil.SeedPlant(t, db, "Blue #1", strain, testutil.SeedZone(t, db, "Z"))

	resp := f.cc.get("/smoke-criteria", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	crit := decode[struct {
		Items []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}](t, resp.Body)
	require.Len(t, crit.Items, 5)
	aroma, taste := crit.Items[0].ID, crit.Items[1].ID

	// Not harvested yet.
	resp = f.cc.do(http.MethodPost, "/plants/"+itoa(plant)+"/smoke-reports", bearer(write), []byte(`{"rating":4}`))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "plant_not_harvested", requireErrorEnvelope(t, resp.Body).Error.Code)

	var curing int
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE status = 'Curing'").Scan(&curing))
	mustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)", plant, curing, ny("2026-10-01"))

	resp = f.cc.do(http.MethodPost, "/plants/"+itoa(plant)+"/smoke-reports", bearer(write),
		[]byte(`{"report_date":"2026-10-15","rating":4,"consumption_method":"joint","scores":[{"criterion_id":`+itoa(aroma)+`,"value":5},{"criterion_id":`+itoa(taste)+`,"value":3}]}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	rep := decode[smokeBody](t, resp.Body)
	assert.Equal(t, "/api/v1/smoke-reports/"+itoa(rep.ID), resp.Header.Get("Location"))
	assert.Equal(t, "2026-10-15", rep.ReportDate)
	require.NotNil(t, rep.CureDays)
	assert.Equal(t, 14, *rep.CureDays)
	require.Len(t, rep.Scores, 2)
	assert.Equal(t, "Aroma", rep.Scores[0].Criterion.Name)

	resp = f.cc.do(http.MethodPost, "/plants/"+itoa(plant)+"/smoke-reports", bearer(write), []byte(`{"rating":7}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, requireErrorEnvelope(t, resp.Body).Error.Details["fields"], "rating")

	// PATCH without scores keeps them.
	resp = f.cc.do(http.MethodPatch, "/smoke-reports/"+itoa(rep.ID), bearer(write), []byte(`{"rating":5}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	rep = decode[smokeBody](t, resp.Body)
	assert.Equal(t, 5, rep.Rating)
	assert.Len(t, rep.Scores, 2)
	resp = f.cc.do(http.MethodPatch, "/smoke-reports/"+itoa(rep.ID), bearer(write), []byte(`{"scores":[]}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, decode[smokeBody](t, resp.Body).Scores)

	resp = f.cc.get("/plants/"+itoa(plant)+"/smoke-reports", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = f.cc.get("/strains/"+itoa(strain)+"/smoke-summary", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	sum := decode[struct {
		Average  *float64 `json:"average_rating"`
		Harvests int      `json:"harvests"`
	}](t, resp.Body)
	require.NotNil(t, sum.Average)
	assert.Equal(t, 5.0, *sum.Average)
	assert.Equal(t, 1, sum.Harvests)

	assert.Equal(t, http.StatusNoContent, f.cc.do(http.MethodDelete, "/smoke-reports/"+itoa(rep.ID), bearer(write), nil).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.get("/smoke-reports/"+itoa(rep.ID), read).StatusCode)
	resp = f.cc.get("/strains/"+itoa(strain)+"/smoke-summary", read)
	assert.Contains(t, string(resp.Body), `"average_rating":null`)
}
