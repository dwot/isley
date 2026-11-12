package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

func sqliteServices(t *testing.T) (*service.Services, *config.Store, *sql.DB) {
	t.Helper()
	db := testutil.NewTestDB(t)
	store := config.NewStore()
	return newServices(t, db, service.SQLite, store, chrono.Fixed(time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC))), store, db
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	e, ok := service.AsError(err)
	require.True(t, ok, "want a service error, got %v", err)
	require.Equal(t, service.KindInvalid, e.Kind, "%v", err)
	return e.Fields
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	e, ok := service.AsError(err)
	require.True(t, ok, "want a service error, got %v", err)
	return e.Code
}

func TestRefreshStore(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.SeedZone(t, db, "Tent")
	store := config.NewStore()
	require.NoError(t, service.RefreshStore(context.Background(), db, store))
	assert.Len(t, store.Statuses(), 9)
	assert.Equal(t, "Germinating", store.Statuses()[0].Status)
	assert.NotEmpty(t, store.Metrics())
	assert.Len(t, store.Activities(), 3)
	require.Len(t, store.Zones(), 1)
	assert.Equal(t, "Tent", store.Zones()[0].Name)
}

func TestLookups_Metrics(t *testing.T) {
	t.Parallel()
	svc, store, _ := sqliteServices(t)
	ctx := context.Background()

	_, err := svc.Lookups.CreateMetric(ctx, service.MetricInput{Name: " ", Unit: "x"})
	assert.Equal(t, map[string]string{"name": "is required"}, fieldsOf(t, err))

	m, err := svc.Lookups.CreateMetric(ctx, service.MetricInput{Name: " EC ", Unit: "mS/cm"})
	require.NoError(t, err)
	assert.Equal(t, service.Metric{ID: m.ID, Name: "EC", Unit: "mS/cm"}, m)
	assert.Equal(t, "EC", store.Metrics()[len(store.Metrics())-1].Name)

	m, err = svc.Lookups.UpdateMetric(ctx, m.ID, service.MetricInput{Name: "EC", Unit: "µS/cm"})
	require.NoError(t, err)
	assert.Equal(t, "µS/cm", m.Unit)

	_, err = svc.Lookups.UpdateMetric(ctx, 9999, service.MetricInput{Name: "x"})
	assert.True(t, service.IsKind(err, service.KindNotFound))

	all, err := svc.Lookups.Metrics(ctx)
	require.NoError(t, err)
	height := all[0]
	require.True(t, height.BuiltIn)
	assert.Equal(t, service.CodeMetricBuiltIn, codeOf(t, svc.Lookups.DeleteMetric(ctx, height.ID)))

	require.NoError(t, svc.Lookups.DeleteMetric(ctx, m.ID))
	assert.True(t, service.IsKind(svc.Lookups.DeleteMetric(ctx, m.ID), service.KindNotFound))
}

func TestLookups_ActivityTypes(t *testing.T) {
	t.Parallel()
	svc, store, db := sqliteServices(t)
	ctx := context.Background()
	metrics, err := svc.Lookups.Metrics(ctx)
	require.NoError(t, err)
	height := metrics[0].ID

	_, err = svc.Lookups.CreateActivityType(ctx, service.ActivityTypeInput{Name: "Water"})
	assert.Equal(t, service.CodeActivityTypeNameReserved, codeOf(t, err))

	_, err = svc.Lookups.CreateActivityType(ctx, service.ActivityTypeInput{
		Name: "Flush", Metrics: []service.ActivityMetricInput{{MetricID: 9999}},
	})
	assert.Contains(t, fieldsOf(t, err), "metrics")

	// Duplicates merge (required wins); ids <= 0 are dropped.
	flush, err := svc.Lookups.CreateActivityType(ctx, service.ActivityTypeInput{
		Name: "Flush", IsWatering: true,
		Metrics: []service.ActivityMetricInput{{MetricID: height}, {MetricID: 0}, {MetricID: height, Required: true}},
	})
	require.NoError(t, err)
	assert.True(t, flush.IsWatering)
	assert.Equal(t, []service.ActivityMetric{{MetricID: height, Name: "Height", Unit: "in", Required: true}}, flush.Metrics)
	assert.Len(t, store.Activities(), 4)

	// Nothing can be renamed to a built-in name; a built-in keeps its own.
	_, err = svc.Lookups.UpdateActivityType(ctx, flush.ID, service.ActivityTypeInput{Name: "Feed"})
	assert.Equal(t, service.CodeActivityTypeNameReserved, codeOf(t, err))
	all, err := svc.Lookups.ActivityTypes(ctx)
	require.NoError(t, err)
	var water service.ActivityType
	for _, a := range all {
		if a.Name == "Water" {
			water = a
		}
	}
	require.True(t, water.BuiltIn)
	water, err = svc.Lookups.UpdateActivityType(ctx, water.ID, service.ActivityTypeInput{Name: "Water", IsWatering: true})
	require.NoError(t, err)
	assert.True(t, water.IsWatering)

	flush, err = svc.Lookups.UpdateActivityType(ctx, flush.ID, service.ActivityTypeInput{Name: "Flush"})
	require.NoError(t, err)
	assert.Empty(t, flush.Metrics)

	// Deleting a type deletes what was logged with it.
	zone := testutil.SeedZone(t, db, "Tent")
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	plant := testutil.SeedPlant(t, db, "P", strain, zone)
	testutil.SeedActivity(t, db, plant, flush.ID, time.Now())
	require.NoError(t, svc.Lookups.DeleteActivityType(ctx, flush.ID))
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM plant_activity WHERE activity_id = $1", flush.ID).Scan(&n))
	assert.Zero(t, n)
	assert.Len(t, store.Activities(), 3)
}

func TestLookups_StatusVPD(t *testing.T) {
	t.Parallel()
	svc, store, _ := sqliteServices(t)
	ctx := context.Background()
	statuses, err := svc.Lookups.Statuses(ctx)
	require.NoError(t, err)
	veg := statuses[3]
	require.Equal(t, service.StatusVeg, veg.Name)

	low, high := 0.8, 1.2
	_, err = svc.Lookups.SetStatusVPD(ctx, veg.ID, &high, &low)
	assert.Contains(t, fieldsOf(t, err), "vpd_low_kpa")

	got, err := svc.Lookups.SetStatusVPD(ctx, veg.ID, &low, &high)
	require.NoError(t, err)
	assert.Equal(t, &low, got.VPDLowKPa)
	assert.Equal(t, &high, got.VPDHighKPa)
	assert.Equal(t, &low, store.Statuses()[3].VPDLow)

	got, err = svc.Lookups.SetStatusVPD(ctx, veg.ID, nil, &high)
	require.NoError(t, err)
	assert.Nil(t, got.VPDLowKPa)

	_, err = svc.Lookups.SetStatusVPD(ctx, 9999, nil, nil)
	assert.True(t, service.IsKind(err, service.KindNotFound))
}

func TestZones_FindOrCreate(t *testing.T) {
	t.Parallel()
	svc, store, _ := sqliteServices(t)
	ctx := context.Background()

	_, err := svc.Zones.ByName(ctx, "Tent")
	assert.True(t, service.IsKind(err, service.KindNotFound))

	a, err := svc.Zones.FindOrCreate(ctx, "Tent")
	require.NoError(t, err)
	b, err := svc.Zones.FindOrCreate(ctx, " Tent ")
	require.NoError(t, err)
	assert.Equal(t, a.ID, b.ID)
	assert.Len(t, store.Zones(), 1)
	// created_at is CURRENT_TIMESTAMP (UTC on SQLite), read back as an instant.
	assert.WithinDuration(t, time.Now(), a.CreatedAt, time.Minute)
}
