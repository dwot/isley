//go:build integration_postgres

package handlers_test

// Dialect coverage for the smoke-report web handlers (the service's own
// Postgres scenarios are in service/service_postgres_test.go). The CI postgres job selects tests
// by name with -run Postgres, so every function here must carry that word or it
// is silently skipped — which is exactly how CureDays shipped broken on SQLite
// while passing every test.

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
