package service_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

// clockServices runs a New York instance at a fixed time over a database
// that stamps readings in UTC.
func clockServices(t *testing.T, db *sql.DB, dialect service.Dialect, now time.Time) *service.Services {
	t.Helper()
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	svc, err := service.New(service.Deps{
		Version: "test", DB: db, Dialect: dialect, Clock: chrono.Fixed(now),
		Zones: chrono.NewUTCZones(store.Timezone), Store: store, UploadDir: t.TempDir(),
	})
	require.NoError(t, err)
	return svc
}

var ingestNow = time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)

func utcStamp(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

func TestSensorIngest(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkSensorIngest(t, db, clockServices(t, db, service.SQLite, ingestNow))
}

func checkSensorIngest(t *testing.T, db *sql.DB, svc *service.Services) {
	ctx := context.Background()
	temp := testutil.SeedSensor(t, db, "acinfinity", "dev", "temp")
	count := func(q string, args ...any) int {
		var n int
		require.NoError(t, db.QueryRow(q, args...).Scan(&n))
		return n
	}

	// Record: a batch lands whole or not at all.
	require.NoError(t, svc.Sensors.Record(ctx, []service.NewReading{{SensorID: temp, Value: 1}, {SensorID: temp, Value: 2}}))
	assert.Equal(t, 2, count("SELECT COUNT(*) FROM sensor_data WHERE sensor_id = $1", temp))
	err := svc.Sensors.Record(ctx, []service.NewReading{{SensorID: temp, Value: 3}, {SensorID: 999999, Value: 4}})
	assert.Error(t, err)
	assert.Equal(t, 2, count("SELECT COUNT(*) FROM sensor_data WHERE sensor_id = $1", temp))

	tracked, err := svc.Sensors.RecordByIdentity(ctx, "acinfinity", "dev", "temp", 5)
	require.NoError(t, err)
	assert.True(t, tracked)
	tracked, err = svc.Sensors.RecordByIdentity(ctx, "acinfinity", "dev", "humidity", 5)
	require.NoError(t, err)
	assert.False(t, tracked, "only sensors the user scanned in are kept")
	testutil.MustExec(t, db, "DELETE FROM sensor_data")

	// Prune: the cutoff is exactly retention days before now, compared
	// with the stamps as stored (UTC), whatever the server's zone.
	cutoff := ingestNow.AddDate(0, 0, -30)
	for _, at := range []time.Time{cutoff.Add(-time.Minute), cutoff.Add(time.Minute)} {
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 1, $2)", temp, utcStamp(at))
	}
	n, err := svc.Sensors.Prune(ctx, 30)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Equal(t, 1, count("SELECT COUNT(*) FROM sensor_data"))
	n, err = svc.Sensors.Prune(ctx, 0)
	require.NoError(t, err)
	assert.Zero(t, n, "retention 0 keeps everything")
	require.NoError(t, svc.Sensors.Maintain(ctx))
	testutil.MustExec(t, db, "DELETE FROM sensor_data")

	// Rollup: everything on the first run, then only the last 25 hours.
	old := ingestNow.Add(-48 * time.Hour).Truncate(time.Hour)
	recent := ingestNow.Add(-2 * time.Hour).Truncate(time.Hour)
	for _, v := range []float64{10, 20} {
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)",
			temp, v, utcStamp(old.Add(10*time.Minute)))
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)",
			temp, v+1, utcStamp(recent.Add(10*time.Minute)))
	}
	require.NoError(t, svc.Sensors.RefreshRollup(ctx, false))
	assert.Equal(t, 2, count("SELECT COUNT(*) FROM sensor_data_hourly WHERE sensor_id = $1", temp), "first run backfills")
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 30, $2)", temp, utcStamp(old.Add(20*time.Minute)))
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 30, $2)", temp, utcStamp(recent.Add(20*time.Minute)))
	require.NoError(t, svc.Sensors.RefreshRollup(ctx, false))
	assert.Equal(t, 2, count("SELECT sample_count FROM sensor_data_hourly WHERE sensor_id = $1 AND bucket = $2", temp, utcStamp(old)),
		"an hour outside the window is left alone")
	assert.Equal(t, 3, count("SELECT sample_count FROM sensor_data_hourly WHERE sensor_id = $1 AND bucket = $2", temp, utcStamp(recent)),
		"an hour inside the window is re-summarised")
	require.NoError(t, svc.Sensors.RefreshRollup(ctx, true))
	assert.Equal(t, 3, count("SELECT sample_count FROM sensor_data_hourly WHERE sensor_id = $1 AND bucket = $2", temp, utcStamp(old)))
}

func TestSensorIngest_ZoneVPD(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkZoneVPD(t, db, clockServices(t, db, service.SQLite, ingestNow))
}

func checkZoneVPD(t *testing.T, db *sql.DB, svc *service.Services) {
	ctx := context.Background()
	zone := testutil.SeedZone(t, db, "Tent")
	temp := testutil.SeedSensor(t, db, "ecowitt", "gw", "temp")
	hum := testutil.SeedSensor(t, db, "ecowitt", "gw", "humidity")
	testutil.MustExec(t, db, "UPDATE sensors SET unit = '°F' WHERE id = $1", temp)
	testutil.MustExec(t, db, "UPDATE zones SET leaf_temp_offset = 0, vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2 WHERE id = $3", temp, hum, zone)

	// 77°F (25°C) at 50% with no leaf offset is about 1.58 kPa.
	fresh := utcStamp(ingestNow.Add(-time.Minute))
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 77, $2)", temp, fresh)
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 50, $2)", hum, fresh)
	res, err := svc.Sensors.ComputeZoneVPD(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.NotNil(t, res[0].VPD, res[0].Skipped)
	assert.InDelta(t, 1.58, *res[0].VPD, 0.01)

	// The derived sensor is made once.
	_, err = svc.Sensors.ComputeZoneVPD(ctx, 5*time.Minute)
	require.NoError(t, err)
	var sensors, readings int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sensors WHERE source = 'derived' AND device = $1", strconv.Itoa(zone)).Scan(&sensors))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data sd JOIN sensors s ON s.id = sd.sensor_id
		WHERE s.source = 'derived'`).Scan(&readings))
	assert.Equal(t, 1, sensors)
	assert.Equal(t, 2, readings)

	// A humidity sensor that went quiet stops VPD rather than repeating it.
	testutil.MustExec(t, db, "UPDATE sensor_data SET create_dt = $1 WHERE sensor_id = $2", utcStamp(ingestNow.Add(-time.Hour)), hum)
	res, err = svc.Sensors.ComputeZoneVPD(ctx, 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Nil(t, res[0].VPD)
	assert.NotEmpty(t, res[0].Skipped)
}

func TestSensorRollup_WindowEdge(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkRollupWindowEdge(t, db, service.SQLite)
}

// An incremental refresh whose window starts mid-hour still summarises
// that hour whole, rather than from its last few minutes.
func checkRollupWindowEdge(t *testing.T, db *sql.DB, dialect service.Dialect) {
	ctx := context.Background()
	now := time.Date(2026, 11, 11, 3, 40, 0, 0, time.UTC) // the window starts at 02:40 on the 10th
	svc := clockServices(t, db, dialect, now)
	sensor := testutil.SeedSensor(t, db, "acinfinity", "dev", "temp")
	edge := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	for i, m := range []int{10, 30, 50} {
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)",
			sensor, float64(10*(i+1)), utcStamp(edge.Add(time.Duration(m)*time.Minute)))
	}
	// Not empty, so the refresh is incremental.
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		VALUES ($1, '2026-01-01 00:00:00', 1, 1, 1, 1)`, sensor)

	require.NoError(t, svc.Sensors.RefreshRollup(ctx, false))
	var n int
	var avg float64
	require.NoError(t, db.QueryRow("SELECT sample_count, avg_val FROM sensor_data_hourly WHERE sensor_id = $1 AND bucket = $2",
		sensor, utcStamp(edge)).Scan(&n, &avg))
	assert.Equal(t, 3, n, "the whole hour, not just 02:50")
	assert.InDelta(t, 20, avg, 1e-9)
}
