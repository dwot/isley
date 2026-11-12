//go:build integration_postgres

package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/service"
	"isley/tests/testutil"
)

func TestPostgresStatusMeanings_MatchDatabase(t *testing.T) {
	t.Parallel()
	assertStatusesMatchDatabase(t, testutil.NewTestPostgresDB(t))
}

// The test server runs the stock postgres image, whose zone is UTC
// (Etc/UTC).
func TestPostgresSystemZone(t *testing.T) {
	t.Parallel()
	loc, err := chrono.SystemZone(context.Background(), testutil.NewTestPostgresDB(t), true)
	require.NoError(t, err)
	_, offset := time.Now().In(loc).Zone()
	assert.Equal(t, 0, offset, "zone %s", loc)
}

func TestPostgresPlants(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkPlants(t, plantServices(t, db, service.Postgres), seedPlants(t, db))
}

func TestPostgresStrains(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkStrains(t, testutil.NewServices(t, db), db)
}

func TestPostgresStrains_Feminized(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkStrainFeminized(t, testutil.NewServices(t, db), db)
}

func TestPostgresBreeders(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkBreeders(t, testutil.NewServices(t, db), db)
}

func TestPostgresSensors(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkSensors(t, db, testutil.NewServices(t, db))
}

// The stock Postgres image stamps times in UTC, as SQLite does.
func TestPostgresSensorHistory(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkSensorHistory(t, db, nyServices(t, db, service.Postgres))
}

func TestPostgresSensorIngest(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkSensorIngest(t, db, clockServices(t, db, service.Postgres, ingestNow))
}

func TestPostgresZoneVPD(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkZoneVPD(t, db, clockServices(t, db, service.Postgres, ingestNow))
}

func TestPostgresStreams(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkStreams(t, db, testutil.NewServices(t, db))
}

func TestPostgresSmokeReports(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	checkSmokeReports(t, db, clockServices(t, db, service.Postgres, ingestNow))
}

func TestPostgresUTCConversion(t *testing.T) {
	t.Parallel()
	checkUTCConversion(t, testutil.NewTestPostgresDB(t), service.Postgres)
}

// A Postgres server that stamped in another zone has its system stamps
// shifted too, its rollup rebuilt, and its update triggers left working.
func TestPostgresUTCConversion_SystemStamps(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	ctx := context.Background()
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	zone := testutil.SeedZone(t, db, "Tent")
	sensor := testutil.SeedSensor(t, db, "s", "d", "t")
	// Readings from 20:40 (the 20:00 hour was cut by pruning) to 21:00, in
	// Chicago time; the rollup also has a June hour whose readings are gone,
	// a 20:00 row and a short 21:00 row.
	testutil.MustExec(t, db, `INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES
		($1, 9, '2026-11-10 20:40:00'), ($1, 1, '2026-11-10 21:00:00'), ($1, 3, '2026-11-10 21:30:00')`, sensor)
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count) VALUES
		($1, '2026-06-01 10:00:00', 7, 7, 7, 60), ($1, '2026-11-10 20:00:00', 8, 9, 8.5, 60), ($1, '2026-11-10 21:00:00', 1, 1, 1, 1)`, sensor)
	// The update trigger re-stamps update_dt on any UPDATE, so set the
	// create stamp (which it leaves alone) and read update_dt as it is.
	testutil.MustExec(t, db, "UPDATE zones SET create_dt = '2026-01-01 12:00:00' WHERE id = $1", zone)
	var updatedBefore time.Time
	require.NoError(t, db.QueryRow("SELECT update_dt FROM zones WHERE id = $1", zone).Scan(&updatedBefore))

	converted, err := service.EnsureUTCStorage(ctx, db, service.Postgres, time.UTC, chicago)
	require.NoError(t, err)
	require.True(t, converted)

	var stamp time.Time
	require.NoError(t, db.QueryRow("SELECT create_dt FROM sensor_data WHERE value = 1").Scan(&stamp))
	assert.Equal(t, "2026-11-11 03:00:00", stamp.Format("2006-01-02 15:04:05"), "CST: +6h")
	require.NoError(t, db.QueryRow("SELECT create_dt FROM zones WHERE id = $1", zone).Scan(&stamp))
	assert.Equal(t, "2026-01-01 18:00:00", stamp.Format("2006-01-02 15:04:05"), "CST: +6h")
	require.NoError(t, db.QueryRow("SELECT update_dt FROM zones WHERE id = $1", zone).Scan(&stamp))
	assert.Equal(t, updatedBefore.Add(6*time.Hour), stamp, "shifted, not re-stamped by the trigger")
	// The rollup keeps its history: older hours shifted, hours with
	// readings recomputed from them.
	rows, err := db.Query("SELECT bucket, avg_val, sample_count FROM sensor_data_hourly ORDER BY bucket")
	require.NoError(t, err)
	var hours []string
	for rows.Next() {
		var b time.Time
		var avg float64
		var n int
		require.NoError(t, rows.Scan(&b, &avg, &n))
		hours = append(hours, fmt.Sprintf("%s avg=%g n=%d", b.Format("2006-01-02 15:04"), avg, n))
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{
		"2026-06-01 15:00 avg=7 n=60",   // CDT: +5h, kept
		"2026-11-11 02:00 avg=8.5 n=60", // the pruned hour: shifted, kept
		"2026-11-11 03:00 avg=2 n=2",    // recomputed whole
	}, hours)

	testutil.MustExec(t, db, "UPDATE zones SET name = 'Tent 2' WHERE id = $1", zone)
	var after time.Time
	require.NoError(t, db.QueryRow("SELECT update_dt FROM zones WHERE id = $1", zone).Scan(&after))
	assert.NotEqual(t, stamp, after, "the trigger runs again")
}

func TestPostgresImportCannadb(t *testing.T) {
	t.Parallel()
	checkImportCannadb(t, testutil.NewTestPostgresDB(t), service.Postgres)
}

func TestPostgresSensorRollup_WindowEdge(t *testing.T) {
	t.Parallel()
	checkRollupWindowEdge(t, testutil.NewTestPostgresDB(t), service.Postgres)
}
