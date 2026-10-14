//go:build integration_postgres

package handlers_test

// Dialect coverage for the smoke-report SQL. The CI postgres job selects tests
// by name with -run Postgres, so every function here must carry that word or it
// is silently skipped — which is exactly how CureDays shipped broken on SQLite
// while passing every test.

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

const smokeReportPostgresPassword = "smoke-report-postgres-pw"

// smokeStatusID looks up a plant_status id by name against a Postgres test
// DB. tests/integration's statusIDByNameInt is package-local to
// tests/integration, so this package needs its own copy.
func smokeStatusID(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = $1`, name).Scan(&id))
	return id
}

// TestSmokeReport_CureDaysPostgres exercises harvestDateFor's
// ORDER BY ... LIMIT 1 over a TIMESTAMP column scanned into time.Time — the
// exact construct the SQLite/Postgres decltype divergence broke once
// already (see handlers/smoke_report.go's harvestDateFor doc comment).
func TestSmokeReport_CureDaysPostgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	testutil.MustExec(t, db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)`,
		plantID, smokeStatusID(t, db, "Curing"), "2026-06-01 00:00:00")

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))

	testutil.SeedSmokeReportAt(t, db, plantID, 4,
		time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), map[int]int{aromaID: 5})

	reports := handlers.GetSmokeReports(db, plantID)
	require.Len(t, reports, 1)
	require.NotNil(t, reports[0].CureDays, "harvest date must scan as time.Time on Postgres, not a bare string")
	assert.Equal(t, 42, *reports[0].CureDays, "1 June to 13 July is 42 days")
}

// TestSmokeReport_StrainSummaryPostgres exercises GetStrainSmokeSummary's
// correlated scalar subquery inside a JOIN ON clause against Postgres.
func TestSmokeReport_StrainSummaryPostgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "Blue")
	zoneID := testutil.SeedZone(t, db, "Z")

	p1 := testutil.SeedPlant(t, db, "Blue #1", strainID, zoneID)
	p2 := testutil.SeedPlant(t, db, "Blue #2", strainID, zoneID)

	// p1 tasted twice — only the later report counts.
	testutil.SeedSmokeReportAt(t, db, p1, 2, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), nil)
	testutil.SeedSmokeReportAt(t, db, p1, 4, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), nil)
	testutil.SeedSmokeReportAt(t, db, p2, 5, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), nil)

	got := handlers.GetStrainSmokeSummary(db, strainID)

	assert.Equal(t, 2, got.Count, "count is harvests, not reports")
	assert.InDelta(t, 4.5, got.Average, 0.001, "average must equal the mean of the entries below it")
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "Blue #2", got.Entries[0].PlantName, "most recent harvest first")
}

// TestSmokeReport_CriterionArchiveOnDeletePostgres exercises
// DeleteSmokeCriterionHandler's DELETE ... WHERE NOT EXISTS (...) with $1
// referenced twice — repeated-placeholder binding is driver-specific, and
// lib/pq's positional binding needs the same ordinal reused correctly.
func TestSmokeReport_CriterionArchiveOnDeletePostgres(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)
	testutil.SeedAdmin(t, db, smokeReportPostgresPassword)

	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var criterionID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&criterionID))

	reportID := testutil.SeedSmokeReport(t, db, plantID, 4, map[int]int{criterionID: 5})

	server := testutil.NewTestServer(t, db)
	client, csrf := server.LoginAndFetchCSRF(t, smokeReportPostgresPassword, "/settings")

	resp := client.SessionDelete(t, "/smoke-criteria/"+strconv.Itoa(criterionID), csrf)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var active int
	require.NoError(t, db.QueryRow(
		`SELECT active FROM smoke_report_criterion WHERE id = $1`, criterionID).Scan(&active))
	assert.Equal(t, 0, active, "a scored criterion must be archived, not deleted")

	var scoreCount int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, reportID).Scan(&scoreCount))
	assert.Equal(t, 1, scoreCount, "historical scores must survive archiving")
}
