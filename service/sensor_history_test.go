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

// nyServices runs the instance in New York over a database whose stored
// times are UTC (SQLite, and the stock Postgres image).
func nyServices(t *testing.T, db *sql.DB, dialect service.Dialect) *service.Services {
	t.Helper()
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	svc, err := service.New(service.Deps{
		Version: "test", DB: db, Dialect: dialect, Clock: chrono.System(),
		Zones: chrono.NewUTCZones(store.Timezone), Store: store, UploadDir: t.TempDir(),
	})
	require.NoError(t, err)
	return svc
}

func reading(t *testing.T, db *sql.DB, sensor int, v float64, utc string) {
	t.Helper()
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, $2, $3)", sensor, v, utc)
}

func TestSensorHistory(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkSensorHistory(t, db, nyServices(t, db, service.SQLite))
}

func checkSensorHistory(t *testing.T, db *sql.DB, svc *service.Services) {
	ctx := context.Background()
	ny := svc.TimeZones().Instance()
	temp := testutil.SeedSensor(t, db, "acinfinity", "dev", "ACI.tempF")
	port := testutil.SeedSensor(t, db, "acinfinity", "dev", "ACIP.1")

	// Readings 10:00-11:50 New York (15:00-16:50 UTC) every 10 minutes.
	for i := 0; i < 12; i++ {
		at := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC).Add(time.Duration(i) * 10 * time.Minute)
		reading(t, db, temp, float64(70+i), at.Format("2006-01-02 15:04:05"))
	}
	from := time.Date(2026, 11, 10, 10, 0, 0, 0, ny)
	to := from.Add(2 * time.Hour)

	// Raw: in New York time, paged by reading.
	pts, more, err := svc.Sensors.Raw(ctx, temp, from, to, 0, 5)
	require.NoError(t, err)
	require.Len(t, pts, 5)
	assert.True(t, more)
	assert.True(t, from.Equal(pts[0].At))
	assert.Equal(t, "-05:00", pts[0].At.Format("-07:00"))
	pts, more, err = svc.Sensors.Raw(ctx, temp, from, to, pts[4].ID, 100)
	require.NoError(t, err)
	assert.Len(t, pts, 7)
	assert.False(t, more)

	// 30-minute buckets line up with the clock.
	bs, err := svc.Sensors.Buckets(ctx, temp, from, to, 30*time.Minute)
	require.NoError(t, err)
	require.Len(t, bs, 4)
	assert.True(t, from.Equal(bs[0].Start))
	assert.Equal(t, service.Bucket{Start: bs[0].Start, Min: 70, Avg: 71, Max: 72, Count: 3}, bs[0])
	assert.Nil(t, bs[0].OnFraction)

	// Hourly buckets read the rollup where it's complete. This rollup hour
	// says something different from the raw rows, to show it was used;
	// the newest rollup hour gives way to the raw readings.
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES
		($1, '2026-11-10 15:00:00', 1, 9, 5, 4), ($1, '2026-11-10 16:00:00', 0, 0, 0, 1)`, temp)
	bs, err = svc.Sensors.Buckets(ctx, temp, from, to, time.Hour)
	require.NoError(t, err)
	require.Len(t, bs, 2)
	assert.Equal(t, service.Bucket{Start: bs[0].Start, Min: 1, Avg: 5, Max: 9, Count: 4}, bs[0])
	assert.Equal(t, service.Bucket{Start: bs[1].Start, Min: 76, Avg: 78.5, Max: 81, Count: 6}, bs[1])

	// An hour the rollup never saw (downtime, a restore) is read raw.
	reading(t, db, temp, 50, "2026-11-10 13:30:00")
	bs, err = svc.Sensors.Buckets(ctx, temp, from.Add(-2*time.Hour), to, time.Hour)
	require.NoError(t, err)
	require.Len(t, bs, 3)
	assert.True(t, from.Add(-2*time.Hour).Equal(bs[0].Start))
	assert.Equal(t, 50.0, bs[0].Avg)
	assert.Equal(t, 5.0, bs[1].Avg, "rollup hour")
	assert.Equal(t, 78.5, bs[2].Avg, "raw tail")
	testutil.MustExec(t, db, "DELETE FROM sensor_data WHERE value = 50 AND sensor_id = $1", temp)

	// A day bucket starts at midnight in New York.
	bs, err = svc.Sensors.Buckets(ctx, temp, from, to, 24*time.Hour)
	require.NoError(t, err)
	require.Len(t, bs, 1)
	assert.True(t, time.Date(2026, 11, 10, 0, 0, 0, 0, ny).Equal(bs[0].Start))
	assert.Equal(t, 10, bs[0].Count)

	// Ports: share of readings on, and off→on switches. The first
	// reading has nothing to switch from.
	for i, v := range []float64{50, 0, 0, 30, 30, 0, 60, 0} {
		at := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC).Add(time.Duration(i) * 15 * time.Minute)
		reading(t, db, port, v, at.Format("2006-01-02 15:04:05"))
	}
	bs, err = svc.Sensors.Buckets(ctx, port, from, to, time.Hour)
	require.NoError(t, err)
	require.Len(t, bs, 2)
	require.NotNil(t, bs[0].OnFraction)
	assert.Equal(t, 0.5, *bs[0].OnFraction)
	assert.Equal(t, 1, *bs[0].Starts)
	assert.Equal(t, 0.5, *bs[1].OnFraction)
	assert.Equal(t, 1, *bs[1].Starts, "30 → 0 → 60: one switch on; the hour opens already on")

	// Limits.
	_, err = svc.Sensors.Buckets(ctx, temp, to, from, time.Hour)
	assert.True(t, service.IsKind(err, service.KindInvalid))
	_, err = svc.Sensors.Buckets(ctx, temp, from, from.AddDate(1, 0, 0), time.Minute)
	assert.True(t, service.IsKind(err, service.KindInvalid))
	_, err = svc.Sensors.Buckets(ctx, 9999, from, to, time.Hour)
	assert.True(t, service.IsKind(err, service.KindNotFound))
}

// Whole-day buckets follow the calendar across a DST change.
func TestSensorHistory_DayBucketsAcrossDST(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	svc := nyServices(t, db, service.SQLite)
	ny := svc.TimeZones().Instance()
	temp := testutil.SeedSensor(t, db, "s", "d", "t")
	// Noon New York on Nov 1, 2 and 3 2026; DST ends at 2am Nov 1.
	reading(t, db, temp, 1, "2026-11-01 17:00:00")
	reading(t, db, temp, 2, "2026-11-02 17:00:00")
	reading(t, db, temp, 3, "2026-11-03 17:00:00")
	from := time.Date(2026, 10, 31, 0, 0, 0, 0, ny)
	bs, err := svc.Sensors.Buckets(context.Background(), temp, from, from.AddDate(0, 0, 5), 24*time.Hour)
	require.NoError(t, err)
	require.Len(t, bs, 3)
	for i, b := range bs {
		assert.True(t, time.Date(2026, 11, 1+i, 0, 0, 0, 0, ny).Equal(b.Start), "bucket %d starts %s", i, b.Start)
	}
}

func TestParseBucket(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{
		"5m": 5 * time.Minute, "10m": 10 * time.Minute, "30m": 30 * time.Minute, "1h": time.Hour,
		"1h30m": 90 * time.Minute, "20h": 20 * time.Hour, "1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour,
	} {
		got, err := service.ParseBucket(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
		assert.Equal(t, in, service.FormatBucket(got))
	}
	for _, bad := range []string{"", "0m", "-1h", "xd", "0d", "soon"} {
		_, err := service.ParseBucket(bad)
		assert.Error(t, err, bad)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, 5*time.Minute, service.DefaultBucket(from, from.Add(24*time.Hour)))
	assert.Equal(t, time.Hour, service.DefaultBucket(from, from.AddDate(0, 0, 7)))
}
