package handlers_test

// DB-layer tests for handlers/smoke_report.go and
// handlers/smoke_report_criterion.go — the functions that take a *sql.DB
// directly rather than a *gin.Context.

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func TestGetSmokeCriteria_SeededAndOrdered(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	all := handlers.GetSmokeCriteria(db, false)
	require.Len(t, all, 5)
	assert.Equal(t, "Aroma", all[0].Name)
	assert.Equal(t, "Burn", all[4].Name)
	assert.True(t, all[0].Active)
}

func TestGetSmokeCriteria_ActiveOnlyFiltersArchived(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db,
		`UPDATE smoke_report_criterion SET active = 0 WHERE name = $1`, "Burn")

	active := handlers.GetSmokeCriteria(db, true)
	require.Len(t, active, 4)
	for _, c := range active {
		assert.NotEqual(t, "Burn", c.Name)
	}

	assert.Len(t, handlers.GetSmokeCriteria(db, false), 5)
}

func TestGetSmokeReports_OrdersMostRecentFirst(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	testutil.SeedSmokeReportAt(t, db, plantID, 3,
		time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC), nil)
	testutil.SeedSmokeReportAt(t, db, plantID, 5,
		time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), nil)

	reports := handlers.GetSmokeReports(db, plantID)
	require.Len(t, reports, 2)
	assert.Equal(t, 5, reports[0].Rating, "most recent report first")
	assert.Equal(t, 3, reports[1].Rating)
}

func TestGetSmokeReports_IncludesScoresWithCriterionNames(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))

	testutil.SeedSmokeReport(t, db, plantID, 4, map[int]int{aromaID: 5})

	reports := handlers.GetSmokeReports(db, plantID)
	require.Len(t, reports, 1)
	require.Len(t, reports[0].Scores, 1)
	assert.Equal(t, "Aroma", reports[0].Scores[0].Name)
	assert.Equal(t, 5, reports[0].Scores[0].Value)
}

// A report with no scores at all is valid: an overall rating plus a
// description is enough. It must not be dropped from the list.
func TestGetSmokeReports_ReportWithoutScoresIsReturned(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	testutil.SeedSmokeReport(t, db, plantID, 4, nil)

	reports := handlers.GetSmokeReports(db, plantID)
	require.Len(t, reports, 1)
	assert.Empty(t, reports[0].Scores)
	assert.Nil(t, reports[0].CureDays, "no harvest status logged → no cure duration")
}

// TestWriteScores_FailureRollsBackTransaction is the genuine transactional
// rollback test the handler layer's coverage was missing: it reproduces the
// same primitives AddSmokeReportHandler/UpdateSmokeReportHandler use
// (db.Begin, insert smoke_report, insert smoke_report_score, roll back on
// error) directly against the schema, forcing the second score insert to
// fail on the UNIQUE(report_id, criterion_id) constraint, and asserts the
// report row does not survive the rollback either — a half-written report
// is never acceptable. (TestSmokeReportHTTP_InvalidCriterionWritesNothing
// covers the request-validation path, which rejects before a transaction is
// ever opened and so cannot exercise the rollback itself.)
func TestWriteScores_FailureRollsBackTransaction(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	var aromaID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM smoke_report_criterion WHERE name = $1`, "Aroma").Scan(&aromaID))

	tx, err := db.Begin()
	require.NoError(t, err)

	var reportID int
	require.NoError(t, tx.QueryRow(
		`INSERT INTO smoke_report (plant_id, report_date, consumption_method, rating, description)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		plantID, "2026-08-12 00:00:00", "", 4, "").Scan(&reportID))

	_, err = tx.Exec(
		`INSERT INTO smoke_report_score (report_id, criterion_id, value) VALUES ($1, $2, $3)`,
		reportID, aromaID, 5)
	require.NoError(t, err, "first score insert must succeed")

	// Same (report_id, criterion_id) pair again — violates the UNIQUE
	// constraint, exactly the kind of mid-transaction failure
	// defer tx.Rollback() exists to undo.
	_, err = tx.Exec(
		`INSERT INTO smoke_report_score (report_id, criterion_id, value) VALUES ($1, $2, $3)`,
		reportID, aromaID, 3)
	require.Error(t, err, "duplicate (report_id, criterion_id) must violate the UNIQUE constraint")

	require.NoError(t, tx.Rollback())

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report WHERE plant_id = $1`, plantID).Scan(&n))
	assert.Equal(t, 0, n, "a mid-transaction failure must roll back the report row too, not just the scores")
}

// The headline average must equal the mean of the rows displayed beneath it:
// one row per harvested plant, carrying that plant's most recent rating.
func TestGetStrainSmokeSummary_AveragesLatestReportPerPlant(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
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
	assert.InDelta(t, 4.5, got.Average, 0.001)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "Blue #2", got.Entries[0].PlantName, "most recent harvest first")
	assert.Equal(t, 5, got.Entries[0].Rating)
	assert.Equal(t, 4, got.Entries[1].Rating)
}

func TestGetStrainSmokeSummary_ExcludesPlantsWithoutReports(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "Blue")
	zoneID := testutil.SeedZone(t, db, "Z")

	rated := testutil.SeedPlant(t, db, "Rated", strainID, zoneID)
	testutil.SeedPlant(t, db, "Never tasted", strainID, zoneID)
	testutil.SeedSmokeReport(t, db, rated, 4, nil)

	got := handlers.GetStrainSmokeSummary(db, strainID)
	assert.Equal(t, 1, got.Count)
	require.Len(t, got.Entries, 1)
	assert.Equal(t, "Rated", got.Entries[0].PlantName)
}

func TestDeletePlantById_RemovesSmokeReports(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "Doomed", strainID, zoneID)

	criterionID := testutil.SeedSmokeCriterion(t, db, "Axis", 1)
	reportID := testutil.SeedSmokeReport(t, db, plantID, 4, map[int]int{criterionID: 3})

	require.NoError(t, handlers.DeletePlantById(db, strconv.Itoa(plantID)))

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report WHERE plant_id = $1`, plantID).Scan(&n))
	assert.Zero(t, n, "reports must go with the plant")

	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1`, reportID).Scan(&n))
	assert.Zero(t, n, "scores must go with the report")

	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM smoke_report_criterion WHERE id = $1`, criterionID).Scan(&n))
	assert.Equal(t, 1, n, "the criterion itself is shared config and must survive")
}

// TestPlantHasSmokeReportsForTemplate pins IMPORTANT 3 from the final
// whole-branch review: a plant can lose its harvested status (e.g. a
// mis-logged Curing entry gets deleted) while still owning smoke reports
// that must stay reachable. PlantHasSmokeReportsForTemplate is the other
// half of the OR that decides plant.html's card visibility alongside
// PlantIsHarvestedForTemplate — see routes.go and smoke_report.go's doc
// comment on this function.
func TestPlantHasSmokeReportsForTemplate(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := testutil.SeedPlant(t, db, "P", strainID, zoneID)

	assert.False(t, handlers.PlantHasSmokeReportsForTemplate(db, plantID),
		"a plant with no reports must report false")

	testutil.SeedSmokeReport(t, db, plantID, 4, nil)

	assert.True(t, handlers.PlantHasSmokeReportsForTemplate(db, plantID),
		"a plant with at least one report must report true")
}

func TestGetStrainSmokeSummary_EmptyStrainIsZeroNotPanic(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "Untasted")

	got := handlers.GetStrainSmokeSummary(db, strainID)
	assert.Equal(t, 0, got.Count)
	assert.Zero(t, got.Average)
	assert.Empty(t, got.Entries)
}

func TestSmokeRatingScale_CoversTheDocumentedBounds(t *testing.T) {
	t.Parallel()

	scale := handlers.SmokeRatingScale()

	require.Len(t, scale, 5, "the 1-5 scale is what both modals render")
	assert.Equal(t, 1, scale[0])
	assert.Equal(t, 5, scale[len(scale)-1])

	// Ascending and gapless: the templates emit one star per entry in order, so
	// a reordered or sparse slice would render a nonsensical widget.
	for i, v := range scale {
		assert.Equal(t, i+1, v, "scale[%d] should be %d", i, i+1)
	}
}
