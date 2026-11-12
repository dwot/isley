package service_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/service"
	"isley/tests/testutil"
)

func TestSmokeReports(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkSmokeReports(t, db, clockServices(t, db, service.SQLite, ingestNow))
}

func checkSmokeReports(t *testing.T, db *sql.DB, svc *service.Services) {
	ctx := context.Background()
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "Blue")
	zone := testutil.SeedZone(t, db, "Z")
	plant := testutil.SeedPlant(t, db, "Blue #1", strain, zone)
	other := testutil.SeedPlant(t, db, "Blue #2", strain, zone)
	testutil.SeedPlant(t, db, "Never tasted", strain, zone)

	// Criteria: seeded, ordered; archiving hides them from the entry form.
	all, err := svc.SmokeReports.Criteria(ctx, false)
	require.NoError(t, err)
	require.Len(t, all, 5)
	assert.Equal(t, "Aroma", all[0].Name)
	aroma, burn := all[0].ID, all[4].ID

	// Only a harvested plant takes a new report.
	_, err = svc.SmokeReports.Create(ctx, plant, service.SmokeReportInput{Rating: 4})
	assert.True(t, service.IsKind(err, service.KindConflict), "%v", err)
	harvested, has, err := svc.SmokeReports.Eligibility(ctx, plant)
	require.NoError(t, err)
	assert.False(t, harvested)
	assert.False(t, has)

	// Harvested (Curing) on 1 June, in the instance zone.
	testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)",
		plant, statusID(t, db, "Curing"), testutil.StoredWall("America/New_York", "2026-06-01"))
	testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)",
		other, statusID(t, db, "Drying"), testutil.StoredWall("America/New_York", "2026-05-20"))

	_, err = svc.SmokeReports.Create(ctx, plant, service.SmokeReportInput{Rating: 9, Scores: []service.SmokeScoreInput{
		{CriterionID: aroma, Value: 5}, {CriterionID: aroma, Value: 4}, {CriterionID: 9999, Value: 3}}})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	for _, f := range []string{"rating", "scores[1].criterion_id", "scores[2].criterion_id"} {
		assert.Contains(t, e.Fields, f)
	}

	early, err := svc.SmokeReports.Create(ctx, plant, service.SmokeReportInput{
		ReportDate: chrono.Date{Year: 2026, Month: 7, Day: 13}, Rating: 2, ConsumptionMethod: " joint ",
		Scores: []service.SmokeScoreInput{{CriterionID: aroma, Value: 5}, {CriterionID: burn, Value: 3}}})
	require.NoError(t, err)
	assert.Equal(t, "joint", early.ConsumptionMethod)
	require.NotNil(t, early.CureDays)
	assert.Equal(t, 42, *early.CureDays, "1 June to 13 July")
	require.Len(t, early.Scores, 2)
	assert.Equal(t, "Aroma", early.Scores[0].Criterion.Name)

	// A report dated before the harvest has no cure time.
	backdated, err := svc.SmokeReports.Create(ctx, plant, service.SmokeReportInput{
		ReportDate: chrono.Date{Year: 2026, Month: 5, Day: 1}, Rating: 3})
	require.NoError(t, err)
	assert.Nil(t, backdated.CureDays)
	require.NoError(t, svc.SmokeReports.Delete(ctx, backdated.ID))

	// No date means today (the fixed clock's day in New York).
	latest, err := svc.SmokeReports.Create(ctx, plant, service.SmokeReportInput{Rating: 4})
	require.NoError(t, err)
	assert.Equal(t, chrono.Date{Year: 2026, Month: 11, Day: 10}, latest.ReportDate)
	assert.Empty(t, latest.Scores, "a report without scores is fine")

	reports, err := svc.SmokeReports.ForPlant(ctx, plant)
	require.NoError(t, err)
	require.Len(t, reports, 2)
	assert.Equal(t, latest.ID, reports[0].ID, "latest first")

	// Archiving a scored criterion keeps its score through later edits.
	archived, err := svc.SmokeReports.DeleteCriterion(ctx, burn)
	require.NoError(t, err)
	assert.True(t, archived)
	edited, err := svc.SmokeReports.Update(ctx, early.ID, service.SmokeReportInput{
		ReportDate: early.ReportDate, Rating: 3, Scores: []service.SmokeScoreInput{{CriterionID: aroma, Value: 2}}})
	require.NoError(t, err)
	require.Len(t, edited.Scores, 2)
	assert.Equal(t, 2, edited.Scores[0].Value, "active criterion rewritten")
	assert.Equal(t, 3, edited.Scores[1].Value, "archived criterion kept")
	_, err = svc.SmokeReports.Update(ctx, early.ID, service.SmokeReportInput{Rating: 3,
		Scores: []service.SmokeScoreInput{{CriterionID: burn, Value: 1}}})
	assert.True(t, service.IsKind(err, service.KindInvalid), "an archived criterion can't be scored")
	unused, err := svc.SmokeReports.CreateCriterion(ctx, " Bag appeal ", 6)
	require.NoError(t, err)
	assert.Equal(t, "Bag appeal", unused.Name)
	archived, err = svc.SmokeReports.DeleteCriterion(ctx, unused.ID)
	require.NoError(t, err)
	assert.False(t, archived, "never scored: deleted outright")
	_, err = svc.SmokeReports.DeleteCriterion(ctx, unused.ID)
	assert.True(t, service.IsKind(err, service.KindNotFound))

	// Strain summary: each plant's latest report, averaged.
	_, err = svc.SmokeReports.Create(ctx, other, service.SmokeReportInput{ReportDate: chrono.Date{Year: 2026, Month: 8, Day: 1}, Rating: 5})
	require.NoError(t, err)
	sum, err := svc.SmokeReports.StrainSummary(ctx, strain)
	require.NoError(t, err)
	require.Len(t, sum.Entries, 2)
	assert.Equal(t, "Blue #1", sum.Entries[0].Plant.Name, "latest report first")
	assert.InDelta(t, 4.5, sum.Average, 0.001)

	_, has, err = svc.SmokeReports.Eligibility(ctx, plant)
	require.NoError(t, err)
	assert.True(t, has)

	// Deleting the plant takes its reports and scores; criteria stay.
	require.NoError(t, svc.Plants.Delete(ctx, plant))
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM smoke_report_score WHERE report_id = $1", edited.ID).Scan(&n))
	assert.Zero(t, n)
	all, err = svc.SmokeReports.Criteria(ctx, false)
	require.NoError(t, err)
	assert.Len(t, all, 5)
	_, err = svc.SmokeReports.Get(ctx, edited.ID)
	assert.True(t, service.IsKind(err, service.KindNotFound))
}
