package integration

// End-to-end coverage of the smoke report feature through a real
// app.NewEngine: create a report on a harvested plant, read it back on the
// public endpoint, and see it surface in the strain summary.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

const smokeIntegrationPassword = "smoke-integration-pw"

func TestSmokeReport_CreateReadAndSummarise(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeIntegrationPassword)

	breederID := testutil.SeedBreeder(t, db, "Mephisto")
	strainID := testutil.SeedStrain(t, db, breederID, "Blue")
	zoneID := testutil.SeedZone(t, db, "Tent")
	plantID := testutil.SeedPlant(t, db, "Blue #1", strainID, zoneID)

	// statusIDByNameInt is already defined in this package. Midnight (not
	// noon) so the cure-day count below lands on a whole number: report_date
	// is date-only ("2026-07-13" → midnight UTC), and cureDays truncates
	// toward zero, so a noon harvest would read back as 41 days, not 42.
	testutil.MustExec(t, db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`,
		plantID, statusIDByNameInt(t, db, "Curing"), "2026-06-01 00:00:00")

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))

	// Guest mode so the anonymous reads below (list + strain summary) hit
	// AddBasicRoutes' public path rather than 302-ing to /login — see
	// routes.go: AddBasicRoutes is only public when guest mode is on.
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	client, csrf := server.LoginAndFetchCSRF(t, smokeIntegrationPassword,
		"/plant/"+strconv.Itoa(plantID)+"/edit")

	createResp := client.SessionPostJSON(t,
		"/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date":        "2026-07-13",
			"consumption_method": "Joint",
			"rating":             4,
			"description":        "Citrus nose, clean white ash.",
			"scores":             []map[string]int{{"criterion_id": aromaID, "value": 5}},
		})
	defer testutil.DrainAndClose(createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// Public read of the plant's reports.
	anon := server.NewClient(t)
	listResp := anon.Get("/plant/" + strconv.Itoa(plantID) + "/smoke-reports")
	defer testutil.DrainAndClose(listResp)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var reports []struct {
		Rating      int    `json:"rating"`
		Description string `json:"description"`
		CureDays    *int   `json:"cure_days"`
		Scores      []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		} `json:"scores"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&reports))
	require.Len(t, reports, 1)
	assert.Equal(t, 4, reports[0].Rating)
	require.Len(t, reports[0].Scores, 1)
	assert.Equal(t, "Aroma", reports[0].Scores[0].Name)
	require.NotNil(t, reports[0].CureDays)
	assert.Equal(t, 42, *reports[0].CureDays, "1 June to 13 July is 42 days")

	// The strain summary picks it up.
	summaryResp := anon.Get("/strains/" + strconv.Itoa(strainID) + "/smoke-summary")
	defer testutil.DrainAndClose(summaryResp)
	require.Equal(t, http.StatusOK, summaryResp.StatusCode)

	var summary struct {
		Average float64 `json:"average"`
		Count   int     `json:"count"`
		Entries []struct {
			PlantName string `json:"plant_name"`
			Rating    int    `json:"rating"`
		} `json:"entries"`
	}
	require.NoError(t, json.NewDecoder(summaryResp.Body).Decode(&summary))
	assert.Equal(t, 1, summary.Count)
	assert.InDelta(t, 4.0, summary.Average, 0.001)
	require.Len(t, summary.Entries, 1)
	assert.Equal(t, "Blue #1", summary.Entries[0].PlantName)
}
