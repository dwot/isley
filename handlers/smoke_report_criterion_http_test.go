package handlers_test

// HTTP-layer tests for handlers/smoke_report_criterion.go.
//
// Routes:
//   GET    /smoke-criteria      (api-protected) → GetSmokeCriteriaHandler
//   POST   /smoke-criteria      (api-protected) → AddSmokeCriterionHandler
//   PUT    /smoke-criteria/:id  (api-protected) → UpdateSmokeCriterionHandler
//   DELETE /smoke-criteria/:id  (api-protected) → DeleteSmokeCriterionHandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

const smokeAdminPassword = "smoke-criteria-pw"

func TestSmokeCriteriaHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct{ method, path string }{
		{http.MethodPost, "/smoke-criteria"},
		{http.MethodPut, "/smoke-criteria/1"},
		{http.MethodDelete, "/smoke-criteria/1"},
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

func TestSmokeCriteriaHTTP_AddRejectsEmptyName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)
	server := testutil.NewTestServer(t, db)

	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")
	resp := client.SessionPostJSON(t, "/smoke-criteria", csrf, map[string]any{"name": ""})
	defer testutil.DrainAndClose(resp)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSmokeCriteriaHTTP_AddThenList(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)
	server := testutil.NewTestServer(t, db)

	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")
	resp := client.SessionPostJSON(t, "/smoke-criteria", csrf,
		map[string]any{"name": "Mouthfeel", "sort_order": 9})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	listResp := client.Get("/smoke-criteria")
	defer testutil.DrainAndClose(listResp)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var got []map[string]any
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&got))
	assert.Len(t, got, 6)
}

// TestSmokeCriteriaHTTP_DeleteArchivesWhenScored is the load-bearing test of
// this task: removing a criterion that reports already reference must archive
// it, never destroy the recorded scores.
func TestSmokeCriteriaHTTP_DeleteArchivesWhenScored(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)

	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var criterionID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&criterionID))

	reportID := testutil.SeedSmokeReport(t, db, plantID, 4,
		map[int]int{criterionID: 5})

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")

	resp := client.SessionDelete(t, "/smoke-criteria/"+itoa(criterionID), csrf)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var active int
	require.NoError(t, db.QueryRow(
		`SELECT active FROM smoke_report_criterion WHERE id = $1`, criterionID).Scan(&active))
	assert.Equal(t, 0, active, "criterion should be archived, not deleted")

	var scoreCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, reportID).Scan(&scoreCount))
	assert.Equal(t, 1, scoreCount, "historical scores must survive archiving")
}

func TestSmokeCriteriaHTTP_DeleteRemovesWhenUnused(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)

	criterionID := testutil.SeedSmokeCriterion(t, db, "Throat hit", 9)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")

	resp := client.SessionDelete(t, "/smoke-criteria/"+itoa(criterionID), csrf)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_criterion WHERE id = $1`, criterionID).Scan(&n))
	assert.Equal(t, 0, n, "an unused criterion should be hard-deleted")
}

// TestSmokeCriteriaHTTP_UpdateSuccess exercises the success path of
// UpdateSmokeCriterionHandler: rename, reorder, and flip active in one call,
// then confirm the row actually changed in the DB (not just a 200).
func TestSmokeCriteriaHTTP_UpdateSuccess(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)

	criterionID := testutil.SeedSmokeCriterion(t, db, "Throat hit", 9)

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")

	resp := client.SessionPutJSON(t, "/smoke-criteria/"+itoa(criterionID), csrf,
		map[string]any{"name": "Retrohale", "sort_order": 3, "active": false})
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	var sortOrder, active int
	require.NoError(t, db.QueryRow(
		`SELECT name, sort_order, active FROM smoke_report_criterion WHERE id = $1`, criterionID,
	).Scan(&name, &sortOrder, &active))
	assert.Equal(t, "Retrohale", name)
	assert.Equal(t, 3, sortOrder)
	assert.Equal(t, 0, active, "active:false in the request should persist as 0")
}

// TestSmokeCriteriaHTTP_InvalidIDReturns400 checks that a non-numeric :id on
// PUT and DELETE is rejected by the strconv.Atoi guard before any query runs,
// rather than reaching the database and surfacing as a 500 or a no-op.
func TestSmokeCriteriaHTTP_InvalidIDReturns400(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAdmin(t, db, smokeAdminPassword)
	server := testutil.NewTestServer(t, db)

	client, csrf := server.LoginAndFetchCSRF(t, smokeAdminPassword, "/settings")

	countCriteria := func() int {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM smoke_report_criterion`).Scan(&n))
		return n
	}
	before := countCriteria()

	t.Run("PUT", func(t *testing.T) {
		resp := client.SessionPutJSON(t, "/smoke-criteria/not-a-number", csrf,
			map[string]any{"name": "X", "sort_order": 0, "active": true})
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("DELETE", func(t *testing.T) {
		resp := client.SessionDelete(t, "/smoke-criteria/not-a-number", csrf)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	assert.Equal(t, before, countCriteria(), "an invalid id must not touch the database")
}

func itoa(i int) string { return strconv.Itoa(i) }
