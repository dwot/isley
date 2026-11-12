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

// plantClock is 03:00 UTC on 11 Nov 2026: still 10 Nov in New York, the
// instance zone. "Today" is the 10th.
var plantClock = time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)

type plantScenario struct {
	growing, harvested, dead int
	tent                     int
	soil, tentTemp, vpd      int
}

func statusID(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE status = $1", name).Scan(&id))
	return id
}

func activityID(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow("SELECT id FROM activity WHERE name = $1", name).Scan(&id))
	return id
}

func exec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := db.Exec(q, args...)
	require.NoError(t, err, q)
}

// seedPlants builds three plants. Dates are stored as the browser sends
// them, in the mixed layouts real databases have.
func seedPlants(t *testing.T, db *sql.DB) plantScenario {
	t.Helper()
	var sc plantScenario
	breeder := testutil.SeedBreeder(t, db, "Breeder")
	strain := testutil.SeedStrain(t, db, breeder, "Photo")
	exec(t, db, "UPDATE strain SET cycle_time = 60, autoflower = 0 WHERE id = $1", strain)
	auto := testutil.SeedStrain(t, db, breeder, "Auto")
	exec(t, db, "UPDATE strain SET cycle_time = 75, autoflower = 1 WHERE id = $1", auto)
	sc.tent = testutil.SeedZone(t, db, "Tent")

	// Wall times below are New York times, stored in UTC as the app
	// stores them.
	ny := func(wall string) string { return testutil.StoredWall("America/New_York", wall) }
	status := func(plant int, name, at string) {
		exec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)", plant, statusID(t, db, name), ny(at))
	}
	activity := func(plant int, name, at string) int {
		var id int
		require.NoError(t, db.QueryRow("INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', $3) RETURNING id",
			plant, activityID(t, db, name), ny(at)).Scan(&id))
		return id
	}

	sc.growing = testutil.SeedPlant(t, db, "Growing", strain, sc.tent)
	exec(t, db, "UPDATE plant SET start_dt = '2026-10-01', harvest_weight = 0 WHERE id = $1", sc.growing)
	status(sc.growing, "Seedling", "2026-10-01T08:00")
	status(sc.growing, "Veg", "2026-10-10 09:00:00")
	status(sc.growing, "Flower", "2026-10-25T10:00:00")
	activity(sc.growing, "Water", "2026-11-08T20:00")
	activity(sc.growing, "Water", "2026-11-09T20:00")
	fed := activity(sc.growing, "Feed", "2026-11-10T23:30")
	var height int
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&height))
	exec(t, db, "INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id) VALUES ($1, $2, 31.5, $3, $4)", sc.growing, height, ny("2026-11-10T23:30"), fed)
	exec(t, db, "INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 30, $3)", sc.growing, height, ny("2026-11-01T12:00"))

	sc.harvested = testutil.SeedPlant(t, db, "Harvested", auto, sc.tent)
	exec(t, db, "UPDATE plant SET start_dt = '2026-08-01T09:00', harvest_weight = 112.5 WHERE id = $1", sc.harvested)
	status(sc.harvested, "Seedling", "2026-08-01T09:00")
	status(sc.harvested, "Flower", "2026-10-01T09:00")
	status(sc.harvested, "Drying", "2026-10-31T18:00")
	status(sc.harvested, "Curing", "2026-11-07T18:00")

	sc.dead = testutil.SeedPlant(t, db, "Dead", strain, sc.tent)
	status(sc.dead, "Seedling", "2026-10-05")
	status(sc.dead, "Dead", "2026-10-06")

	// Sensors: a soil probe linked to the growing plant, and the tent's
	// air sensor and VPD sensor, which it inherits.
	sc.soil = testutil.SeedSensor(t, db, "ecowitt", "probe", "Soil.Moisture")
	sc.tentTemp = testutil.SeedSensor(t, db, "acinfinity", "ctrl", "Temperature")
	sc.vpd = testutil.SeedSensor(t, db, "derived", "1", "VPD")
	exec(t, db, "UPDATE sensors SET zone_id = $1 WHERE id IN ($2, $3, $4)", sc.tent, sc.soil, sc.tentTemp, sc.vpd)
	exec(t, db, "INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, 0)", sc.growing, sc.soil)
	exec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 1.45, '2026-11-11 02:55:00')", sc.vpd)
	exec(t, db, "UPDATE plant_status SET vpd_low = 1.0, vpd_high = 1.4 WHERE status = 'Flower'")
	return sc
}

func plantServices(t *testing.T, db *sql.DB, dialect service.Dialect) *service.Services {
	t.Helper()
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	return newServices(t, db, dialect, store, chrono.Fixed(plantClock))
}

func TestPlants(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkPlants(t, plantServices(t, db, service.SQLite), seedPlants(t, db))
}

func day(s string) chrono.Date {
	d, err := chrono.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func checkPlants(t *testing.T, svc *service.Services, sc plantScenario) {
	ctx := context.Background()
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	p, err := svc.Plants.Get(ctx, sc.growing)
	require.NoError(t, err)
	assert.Equal(t, day("2026-10-01"), p.StartDay)
	assert.Equal(t, 41, p.Day, "1 Oct is Day 1, 10 Nov is Day 41")
	assert.Equal(t, 6, p.Week)
	require.NotNil(t, p.Status)
	assert.Equal(t, service.StatusFlower, p.Status.Status)
	assert.Equal(t, time.Date(2026, 10, 25, 10, 0, 0, 0, ny), p.Status.At)
	require.NotNil(t, p.DaysSinceWatered)
	assert.Equal(t, 1, *p.DaysSinceWatered)
	assert.Equal(t, time.Date(2026, 11, 9, 20, 0, 0, 0, ny), *p.LastWatered)
	require.NotNil(t, p.DaysSinceFed)
	assert.Equal(t, 0, *p.DaysSinceFed, "fed at 23:30 local today")
	require.NotNil(t, p.Flower)
	assert.Nil(t, p.Flower.End)
	assert.Equal(t, 17, p.Flower.Days, "25 Oct is day 1 of flower, 10 Nov is day 17")
	assert.Nil(t, p.HarvestedAt)
	require.NotNil(t, p.EstimatedHarvest)
	assert.Equal(t, day("2026-12-24"), *p.EstimatedHarvest, "first day of flower + 60")
	require.NotNil(t, p.Strain)
	assert.Equal(t, "Photo", p.Strain.Name)
	assert.Equal(t, "Breeder", p.Strain.Breeder.Name)
	require.NotNil(t, p.Zone)
	assert.Equal(t, "Tent", p.Zone.Name)

	h, err := svc.Plants.Get(ctx, sc.harvested)
	require.NoError(t, err)
	assert.Equal(t, service.StatusCuring, h.Status.Status)
	require.NotNil(t, h.Flower)
	require.NotNil(t, h.Flower.End)
	assert.Equal(t, 30, h.Flower.Days, "1 Oct to 31 Oct")
	require.NotNil(t, h.HarvestedAt)
	assert.Equal(t, time.Date(2026, 10, 31, 18, 0, 0, 0, ny), *h.HarvestedAt)
	assert.Equal(t, day("2026-10-15"), *h.EstimatedHarvest, "autoflower: start + 75")
	assert.Equal(t, 112.5, h.HarvestWeightG)
	assert.Nil(t, h.DaysSinceWatered)

	_, err = svc.Plants.Get(ctx, 99999)
	assert.True(t, service.IsKind(err, service.KindNotFound))

	names := func(f service.PlantFilter) []string {
		t.Helper()
		ps, _, err := svc.Plants.List(ctx, f)
		require.NoError(t, err)
		out := []string{}
		for _, p := range ps {
			out = append(out, p.Name)
		}
		return out
	}
	assert.Equal(t, []string{"Growing", "Harvested", "Dead"}, names(service.PlantFilter{}))
	assert.Equal(t, []string{"Growing"}, names(service.PlantFilter{Phase: service.PhaseGrowing}))
	assert.Equal(t, []string{"Harvested"}, names(service.PlantFilter{Phase: service.PhaseHarvested}))
	assert.Equal(t, []string{"Dead"}, names(service.PlantFilter{Phase: service.PhaseDead}))
	assert.Equal(t, []string{"Harvested"}, names(service.PlantFilter{StrainID: h.Strain.ID}))
	assert.Equal(t, []string{"Harvested"}, names(service.PlantFilter{StatusID: h.Status.StatusID}))
	assert.Equal(t, []string{}, names(service.PlantFilter{ZoneID: 99999}))

	page, more, err := svc.Plants.List(ctx, service.PlantFilter{Limit: 2})
	require.NoError(t, err)
	assert.True(t, more)
	require.Len(t, page, 2)
	page, more, err = svc.Plants.List(ctx, service.PlantFilter{Limit: 2, AfterID: page[1].ID})
	require.NoError(t, err)
	assert.False(t, more)
	require.Len(t, page, 1)
	assert.Equal(t, "Dead", page[0].Name)

	d, err := svc.Plants.Detail(ctx, sc.growing, 0)
	require.NoError(t, err)
	require.Len(t, d.StatusHistory, 3)
	assert.Equal(t, service.StatusSeedling, d.StatusHistory[0].Status)
	require.Len(t, d.Activities, 3)
	assert.Equal(t, "Feed", d.Activities[0].Type.Name)
	require.Len(t, d.Activities[0].Measurements, 1)
	assert.Equal(t, 31.5, d.Activities[0].Measurements[0].Value)
	require.Len(t, d.Measurements, 2)
	assert.Equal(t, "in", d.Measurements[0].Unit)

	// Direct links first, then inherited
	// zone sensors except soil probes.
	require.Len(t, d.Sensors, 3)
	assert.Equal(t, sc.soil, d.Sensors[0].ID)
	assert.False(t, d.Sensors[0].Inherited)
	assert.Equal(t, sc.tentTemp, d.Sensors[1].ID)
	assert.True(t, d.Sensors[1].Inherited)
	assert.Nil(t, d.Sensors[1].Latest)
	vpd := d.Sensors[2]
	require.NotNil(t, vpd.Latest)
	assert.Equal(t, time.Date(2026, 11, 10, 21, 55, 0, 0, ny), vpd.Latest.At)
	require.NotNil(t, vpd.VPD)
	assert.Equal(t, "high", vpd.VPD.State)

	stages := func(d service.PlantDetail) map[service.StatusName]int {
		out := map[service.StatusName]int{}
		for _, st := range d.Stages {
			out[st.Status] = st.Days
		}
		return out
	}
	assert.Equal(t, map[service.StatusName]int{"Seedling": 9, "Veg": 15, "Flower": 17}, stages(d))
	assert.Equal(t, 41, d.GrowDays)

	hd, err := svc.Plants.Detail(ctx, sc.harvested, 0)
	require.NoError(t, err)
	assert.Equal(t, map[service.StatusName]int{"Seedling": 61, "Flower": 30, "Drying": 7, "Curing": 4}, stages(hd))
	assert.Equal(t, 92, hd.GrowDays, "1 Aug is Day 1; harvested 31 Oct, Day 92")
	dd, err := svc.Plants.Detail(ctx, sc.dead, 0)
	require.NoError(t, err)
	assert.Equal(t, map[service.StatusName]int{"Seedling": 1, "Dead": 0}, stages(dd))

	d, err = svc.Plants.Detail(ctx, sc.growing, 1)
	require.NoError(t, err)
	assert.Len(t, d.Activities, 1)
	assert.Len(t, d.Measurements, 1)

	prev, next, err := svc.Plants.Adjacent(ctx, sc.harvested)
	require.NoError(t, err)
	assert.Equal(t, sc.dead, prev, "harvested and dead plants are both not growing; the dead one started first")
	assert.Equal(t, 0, next)
}
