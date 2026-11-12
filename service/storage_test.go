package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

// storageNow is 10 Nov 2026, 22:00 in New York.
var storageNow = time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)

func storageServices(t *testing.T, db *sql.DB, dialect service.Dialect, zones *chrono.Zones) *service.Services {
	t.Helper()
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	svc, err := service.New(service.Deps{
		Version: "test", DB: db, Dialect: dialect, Clock: chrono.Fixed(storageNow),
		Zones: zones, Store: store, UploadDir: t.TempDir(),
	})
	require.NoError(t, err)
	return svc
}

func nyTZ() string { return "America/New_York" }

// seedLegacyTimes writes a plant the way older versions stored it: wall
// times in New York, in the layouts different versions used.
func seedLegacyTimes(t *testing.T, db *sql.DB) int {
	t.Helper()
	plant := testutil.SeedPlant(t, db, "P", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"),
		testutil.SeedZone(t, db, "Tent"))
	testutil.MustExec(t, db, "UPDATE plant SET start_dt = '2026-10-01' WHERE id = $1", plant)
	for status, at := range map[string]string{
		"Seedling": "2026-10-01T08:00",    // datetime-local
		"Veg":      "2026-10-10 09:00:00", // LayoutDB
		"Flower":   "2026-11-01",          // bare date, the day DST ended
	} {
		testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)",
			plant, statusID(t, db, status), at)
	}
	water := activityID(t, db, "Water")
	for _, at := range []string{"2026-11-09T20:00", "2026-11-10T23:30:00"} { // the second is 11 Nov in UTC
		testutil.MustExec(t, db, "INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', $3)",
			plant, water, at)
	}
	var height int
	require.NoError(t, db.QueryRow("SELECT id FROM metric WHERE name = 'Height'").Scan(&height))
	testutil.MustExec(t, db, "INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, 30, '2026-07-04T12:00')",
		plant, height)
	return plant
}

// view is what a client sees of the plant: the API's facts and times.
func view(t *testing.T, svc *service.Services, plant int) string {
	t.Helper()
	ctx := context.Background()
	d, err := svc.Plants.Detail(ctx, plant, 10)
	require.NoError(t, err)
	day := chrono.Date{Year: 2026, Month: 11, Day: 10}
	log, _, err := svc.Activities.Log(ctx, service.ActivityFilter{From: &day, To: &day})
	require.NoError(t, err)
	b, err := json.Marshal(map[string]any{
		"start": d.StartedAt, "day": d.Day, "statuses": d.StatusHistory, "stages": d.Stages,
		"activities": d.Activities, "measurements": d.Measurements, "watered": d.LastWatered,
		"days_since_watered": d.DaysSinceWatered, "flower": d.Flower, "on_10_nov": len(log),
	})
	require.NoError(t, err)
	return string(b)
}

func TestUTCConversion(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkUTCConversion(t, db, service.SQLite)
}

// checkUTCConversion: converting changes how times are stored, never what
// anyone sees.
func checkUTCConversion(t *testing.T, db *sql.DB, dialect service.Dialect) {
	ctx := context.Background()
	plant := seedLegacyTimes(t, db)
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	before := view(t, storageServices(t, db, dialect, chrono.NewZones(nyTZ, time.UTC)), plant)

	converted, err := service.EnsureUTCStorage(ctx, db, dialect, ny, time.UTC)
	require.NoError(t, err)
	assert.True(t, converted)
	done, err := service.StoresUTC(ctx, db)
	require.NoError(t, err)
	assert.True(t, done)
	converted, err = service.EnsureUTCStorage(ctx, db, dialect, ny, time.UTC)
	require.NoError(t, err)
	assert.False(t, converted, "once only")

	after := view(t, storageServices(t, db, dialect, chrono.NewUTCZones(nyTZ)), plant)
	assert.JSONEq(t, before, after)
	assert.Contains(t, after, `"on_10_nov":1`, "the 23:30 watering is on 10 Nov, though it is 11 Nov in UTC")

	// Stored in UTC, one layout; the start day isn't shifted.
	var stored chrono.Stored
	require.NoError(t, db.QueryRow(
		"SELECT date FROM plant_activity WHERE plant_id = $1 ORDER BY date DESC LIMIT 1", plant).Scan(&stored))
	assert.Equal(t, "2026-11-11 04:30:00", stored.Time.Format("2006-01-02 15:04:05"))
	require.NoError(t, db.QueryRow(
		"SELECT date FROM plant_measurements WHERE plant_id = $1", plant).Scan(&stored))
	assert.Equal(t, "2026-07-04 16:00:00", stored.Time.Format("2006-01-02 15:04:05"), "EDT: +4h")
	var start chrono.Date
	require.NoError(t, db.QueryRow("SELECT start_dt FROM plant WHERE id = $1", plant).Scan(&start))
	assert.Equal(t, chrono.Date{Year: 2026, Month: 10, Day: 1}, start)
}

// A database copied from SQLite carries a note that its system stamps are
// UTC, which beats the Postgres server's zone.
func TestUTCConversion_CopiedSystemZone(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	sensor := testutil.SeedSensor(t, db, "s", "d", "t")
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 1, '2026-11-11 03:00:00')", sensor)
	testutil.MustExec(t, db, "INSERT INTO settings (name, value) VALUES ('time_storage_system_zone', 'UTC')")
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	// SQLite never shifts system stamps; the note must also be consumed.
	_, err = service.EnsureUTCStorage(context.Background(), db, service.SQLite, time.UTC, chicago)
	require.NoError(t, err)
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM settings WHERE name = 'time_storage_system_zone'").Scan(&n))
	assert.Zero(t, n)
	var stamp chrono.Stored
	require.NoError(t, db.QueryRow("SELECT create_dt FROM sensor_data").Scan(&stamp))
	assert.Equal(t, "2026-11-11 03:00:00", stamp.Time.Format("2006-01-02 15:04:05"))
}
