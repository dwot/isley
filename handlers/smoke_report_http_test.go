package handlers_test

// HTTP-layer tests for handlers/smoke_report.go.
//
// Routes:
//   GET    /plant/:id/smoke-reports       (basic)         → GetSmokeReportsHandler
//   POST   /plant/:plantID/smoke-reports  (api-protected) → AddSmokeReportHandler
//   PUT    /smoke-reports/:id             (api-protected) → UpdateSmokeReportHandler
//   DELETE /smoke-reports/:id             (api-protected) → DeleteSmokeReportHandler

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

// smokeReportScoreJSON and smokeReportJSON mirror the wire shape of
// types.SmokeReportScore / types.SmokeReport, used to decode responses and
// assert on the actual round-tripped content rather than just status codes.
type smokeReportScoreJSON struct {
	CriterionID int    `json:"criterion_id"`
	Name        string `json:"name"`
	Value       int    `json:"value"`
}

type smokeReportJSON struct {
	ID                int                    `json:"id"`
	PlantID           int                    `json:"plant_id"`
	ConsumptionMethod string                 `json:"consumption_method"`
	Rating            int                    `json:"rating"`
	Description       string                 `json:"description"`
	CureDays          *int                   `json:"cure_days"`
	Scores            []smokeReportScoreJSON `json:"scores"`
}

const smokeReportPassword = "smoke-report-pw"

// seedHarvestedPlant returns a plant whose status log puts it in Curing, which
// is what makes it eligible for a smoke report.
func seedHarvestedPlant(t *testing.T, db *sql.DB) int {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var curingID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status WHERE status = $1`, "Curing").Scan(&curingID))
	testutil.MustExec(t, db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`,
		plantID, curingID, "2026-06-01 12:00:00")

	return plantID
}

func TestSmokeReportHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct{ method, path string }{
		{http.MethodPost, "/plant/1/smoke-reports"},
		{http.MethodPut, "/smoke-reports/1"},
		{http.MethodDelete, "/smoke-reports/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer testutil.DrainAndClose(resp)
			assert.Contains(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode)
		})
	}
}

func TestSmokeReportHTTP_RejectsUnharvestedPlant(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)

	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "Still vegging", strainID, zoneID)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{"report_date": "2026-08-12", "rating": 4})
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSmokeReportHTTP_RejectsRatingOutOfRange(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	for _, rating := range []int{0, 6, -1} {
		resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
			map[string]any{"report_date": "2026-08-12", "rating": rating})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "rating %d must be rejected", rating)
		testutil.DrainAndClose(resp)
	}
}

func TestSmokeReportHTTP_RejectsArchivedCriterion(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))
	testutil.MustExec(t, db,
		`UPDATE smoke_report_criterion SET active = 0 WHERE id = $1`, aromaID)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      4,
			"scores":      []map[string]int{{"criterion_id": aromaID, "value": 5}},
		})
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestSmokeReportHTTP_InvalidCriterionWritesNothing confirms an out-of-range
// criterion_id is rejected and leaves no row behind. NOTE: validation runs
// before db.Begin(), so this never reaches the transaction — it is a
// request-validation test, not a rollback test. The genuine transactional
// rollback (a write failing mid-transaction and being undone) is covered by
// TestWriteScores_FailureRollsBackTransaction in smoke_report_db_test.go,
// which opens a real tx, inserts a report row, fails a score insert on the
// UNIQUE(report_id, criterion_id) constraint, and asserts the rollback
// leaves nothing behind.
func TestSmokeReportHTTP_InvalidCriterionWritesNothing(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      4,
			"scores":      []map[string]int{{"criterion_id": 99999, "value": 5}},
		})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report WHERE plant_id = $1`, plantID).Scan(&n))
	assert.Equal(t, 0, n, "no report row should survive a rejected submission")
}

// TestSmokeReportHTTP_CreateThenRead pins the read side's real visibility
// rule: GET /plant/:id/smoke-reports is mounted via AddBasicRoutes, which
// routes.go only exposes without a session when guest mode is on (see
// app/routes.go:51 vs :78). It is not unconditionally public — it follows
// the same visibility as the plant page it feeds, in both directions.
func TestSmokeReportHTTP_CreateThenRead(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date":        "2026-08-12",
			"consumption_method": "Joint",
			"rating":             4,
			"description":        "Citrus nose, clean ash.",
			"scores":             []map[string]int{{"criterion_id": aromaID, "value": 5}},
		})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	t.Run("guest mode on: anonymous read succeeds", func(t *testing.T) {
		guestServer := testutil.NewTestServer(t, db, testutil.WithGuestMode())
		anon := guestServer.NewClient(t)
		readResp := anon.Get("/plant/" + strconv.Itoa(plantID) + "/smoke-reports")
		defer testutil.DrainAndClose(readResp)
		require.Equal(t, http.StatusOK, readResp.StatusCode)

		var reports []smokeReportJSON
		require.NoError(t, json.NewDecoder(readResp.Body).Decode(&reports))
		require.Len(t, reports, 1)

		got := reports[0]
		assert.Equal(t, plantID, got.PlantID)
		assert.Equal(t, "Joint", got.ConsumptionMethod)
		assert.Equal(t, 4, got.Rating)
		assert.Equal(t, "Citrus nose, clean ash.", got.Description)
		require.NotNil(t, got.Scores, `"scores" must round-trip as [] not null`)
		require.Len(t, got.Scores, 1)
		assert.Equal(t, aromaID, got.Scores[0].CriterionID)
		assert.Equal(t, "Aroma", got.Scores[0].Name)
		assert.Equal(t, 5, got.Scores[0].Value)
	})

	t.Run("guest mode off: anonymous read is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		readResp := anon.Get("/plant/" + strconv.Itoa(plantID) + "/smoke-reports")
		defer testutil.DrainAndClose(readResp)
		assert.Contains(t,
			[]int{http.StatusFound, http.StatusUnauthorized, http.StatusForbidden},
			readResp.StatusCode)
	})
}

// TestSmokeReportHTTP_CureDaysReflectsHarvestDate is the regression test for
// the CureDays bug: harvestDateFor used to aggregate with MIN(psl.date),
// which strips SQLite's column decltype and makes modernc.org/sqlite hand
// back a bare string that sql.NullTime.Scan rejects, silently producing the
// zero time (and therefore a nil cure_days) for every report on the default
// backend. The harvest date here is pinned at midnight and the report a
// known number of whole days later so the expected cure_days is exact.
func TestSmokeReportHTTP_CureDaysReflectsHarvestDate(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)

	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var curingID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM plant_status WHERE status = $1`, "Curing").Scan(&curingID))
	testutil.MustExec(t, db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`,
		plantID, curingID, "2026-06-01 00:00:00")

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{"report_date": "2026-06-15", "rating": 4})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	readResp := client.Get("/plant/" + strconv.Itoa(plantID) + "/smoke-reports")
	defer testutil.DrainAndClose(readResp)
	require.Equal(t, http.StatusOK, readResp.StatusCode)

	var reports []smokeReportJSON
	require.NoError(t, json.NewDecoder(readResp.Body).Decode(&reports))
	require.Len(t, reports, 1)
	require.NotNil(t, reports[0].CureDays,
		"cure_days must be populated on SQLite — nil here means the MIN() decltype bug is back")
	assert.Equal(t, 14, *reports[0].CureDays)
}

// TestSmokeReportHTTP_UpdateRewritesReportAndScores confirms PUT actually
// changes the persisted row and replaces its scores, rather than just
// returning 200.
func TestSmokeReportHTTP_UpdateRewritesReportAndScores(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	var aromaID, tasteID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Taste").Scan(&tasteID))

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	createResp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      3,
			"description": "Original notes.",
			"scores":      []map[string]int{{"criterion_id": aromaID, "value": 2}},
		})
	var created struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	testutil.DrainAndClose(createResp)
	require.NotZero(t, created.ID)

	updateResp := client.SessionPutJSON(t, "/smoke-reports/"+strconv.Itoa(created.ID), csrf,
		map[string]any{
			"report_date": "2026-08-13",
			"rating":      5,
			"description": "Revised after a longer cure.",
			"scores":      []map[string]int{{"criterion_id": tasteID, "value": 4}},
		})
	defer testutil.DrainAndClose(updateResp)
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	var rating int
	var description string
	require.NoError(t, db.QueryRow(
		`SELECT rating, description FROM smoke_report WHERE id = $1`, created.ID).
		Scan(&rating, &description))
	assert.Equal(t, 5, rating)
	assert.Equal(t, "Revised after a longer cure.", description)

	var scoreCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, created.ID).
		Scan(&scoreCount))
	assert.Equal(t, 1, scoreCount, "the old Aroma score must have been replaced, not appended to")

	var scoredCriterion, value int
	require.NoError(t, db.QueryRow(
		`SELECT criterion_id, value FROM smoke_report_score WHERE report_id = $1`, created.ID).
		Scan(&scoredCriterion, &value))
	assert.Equal(t, tasteID, scoredCriterion)
	assert.Equal(t, 4, value)
}

// TestSmokeReportHTTP_EditPreservesArchivedCriterionScore is the end-to-end
// regression test for the final-review CRITICAL 1 finding: the edit modal
// only renders <select> inputs for active criteria, validateSmokeReportInput
// only accepts active criterion ids, and writeScores used to delete every
// score on a report before reinserting — so archiving a criterion and then
// editing any unrelated field on an existing report silently deleted that
// report's score on the now-archived criterion. The fix scopes writeScores'
// delete to active criteria only; this test reproduces the original repro
// through the real HTTP endpoints: create a report scoring two criteria,
// archive one via the real DELETE /smoke-criteria/:id handler, confirm both
// scores survive the archive, then PUT an edit carrying only the
// still-active criterion's score, and assert the archived criterion's score
// row is untouched.
func TestSmokeReportHTTP_EditPreservesArchivedCriterionScore(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	var aromaID, burnID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Burn").Scan(&burnID))

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	createResp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      4,
			"description": "Before archive.",
			"scores": []map[string]int{
				{"criterion_id": aromaID, "value": 4},
				{"criterion_id": burnID, "value": 2},
			},
		})
	var created struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	testutil.DrainAndClose(createResp)
	require.NotZero(t, created.ID)

	var scoreCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, created.ID).Scan(&scoreCount))
	require.Equal(t, 2, scoreCount, "both scores must be recorded before archiving")

	// Archive Burn through the real handler, exactly like a user would from
	// Settings.
	archiveResp := client.SessionDelete(t, "/smoke-criteria/"+strconv.Itoa(burnID), csrf)
	defer testutil.DrainAndClose(archiveResp)
	require.Equal(t, http.StatusOK, archiveResp.StatusCode)

	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, created.ID).Scan(&scoreCount))
	require.Equal(t, 2, scoreCount, "archiving a criterion must not touch existing scores")

	// Edit the report — a plausible real edit, e.g. fixing the description —
	// carrying only the still-active criterion's score. The archived
	// criterion never appears in the edit form and so cannot be resubmitted.
	updateResp := client.SessionPutJSON(t, "/smoke-reports/"+strconv.Itoa(created.ID), csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      4,
			"description": "Fixed a typo.",
			"scores": []map[string]int{
				{"criterion_id": aromaID, "value": 4},
			},
		})
	defer testutil.DrainAndClose(updateResp)
	require.Equal(t, http.StatusOK, updateResp.StatusCode)

	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, created.ID).Scan(&scoreCount))
	assert.Equal(t, 2, scoreCount,
		"the archived criterion's score must survive an edit that never mentions it")

	var burnValue int
	require.NoError(t, db.QueryRow(
		`SELECT value FROM smoke_report_score WHERE report_id = $1 AND criterion_id = $2`,
		created.ID, burnID).Scan(&burnValue))
	assert.Equal(t, 2, burnValue, "the archived criterion's score value must be unchanged")

	var aromaValue int
	require.NoError(t, db.QueryRow(
		`SELECT value FROM smoke_report_score WHERE report_id = $1 AND criterion_id = $2`,
		created.ID, aromaID).Scan(&aromaValue))
	assert.Equal(t, 4, aromaValue, "the active criterion's score must still reflect the edit's payload")
}

// TestSmokeReportHTTP_CriteriaLookupFailureIsInternalError pins the
// distinction between "no active criteria" and "the active-criteria query
// itself failed": GetSmokeCriteria returns nil on a DB error, which used to
// be indistinguishable from an empty active set, so every submitted score
// failed validation as 400 api_invalid_smoke_criterion_id during an outage
// instead of surfacing the real 5xx. Dropping the criterion table out from
// under a live plant reproduces the query failure without faking *sql.DB.
func TestSmokeReportHTTP_CriteriaLookupFailureIsInternalError(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeReportPassword)
	plantID := seedHarvestedPlant(t, db)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPassword, "/plant/"+strconv.Itoa(plantID)+"/edit")

	testutil.MustExec(t, db, `DROP TABLE smoke_report_criterion`)

	resp := client.SessionPostJSON(t, "/plant/"+strconv.Itoa(plantID)+"/smoke-reports", csrf,
		map[string]any{
			"report_date": "2026-08-12",
			"rating":      4,
			"scores":      []map[string]int{{"criterion_id": 1, "value": 5}},
		})
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"a failed criteria lookup must not be reported as the client's bad input")
}
