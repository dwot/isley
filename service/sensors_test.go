package service_test

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/service"
	"isley/tests/testutil"
)

func TestSensors(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkSensors(t, db, testutil.NewServices(t, db))
}

func checkSensors(t *testing.T, db *sql.DB, base *service.Services) {
	ctx := context.Background()
	var changes atomic.Int32
	svc := base.WithSensorsChanged(func() { changes.Add(1) })

	tent := testutil.SeedZone(t, db, "Tent")
	closet := testutil.SeedZone(t, db, "Closet")
	temp, err := svc.Sensors.Register(ctx, service.SensorRegistration{
		Name: "Tent Temp", Source: "acinfinity", Device: "AAA", Type: "temp", ZoneID: &tent, Unit: "F"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), changes.Load(), "a new sensor drops the caches")
	again, err := svc.Sensors.Register(ctx, service.SensorRegistration{
		Name: "renamed?", Source: "acinfinity", Device: "AAA", Type: "temp"})
	require.NoError(t, err)
	assert.Equal(t, temp, again, "the same source/device/type is the same sensor")
	assert.Equal(t, int32(1), changes.Load())

	humid, err := svc.Sensors.Register(ctx, service.SensorRegistration{
		Name: "Tent RH", Source: "acinfinity", Device: "AAA", Type: "humi", ZoneID: &tent, Unit: "%"})
	require.NoError(t, err)
	loose, err := svc.Sensors.Register(ctx, service.SensorRegistration{
		Name: "Loose", Source: "custom", Device: "x", Type: "temp", Unit: "C"})
	require.NoError(t, err)
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 70.5, '2026-11-10 08:00:00')", temp)
	testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value, create_dt) VALUES ($1, 72.25, '2026-11-10 09:00:00')", temp)

	// A sensor without a zone is listed.
	all, total, err := svc.Sensors.List(ctx, service.SensorFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, all, 3)
	assert.Equal(t, loose, all[2].ID)
	assert.Nil(t, all[2].Zone)
	assert.Nil(t, all[2].Latest)

	inTent, total, err := svc.Sensors.List(ctx, service.SensorFilter{ZoneID: tent})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, humid, inTent[0].ID)

	got, err := svc.Sensors.Get(ctx, temp)
	require.NoError(t, err)
	require.NotNil(t, got.Latest)
	assert.Equal(t, 72.25, got.Latest.Value)
	assert.Equal(t, 9, got.Latest.At.UTC().Hour(), "readings are stored in the system zone (UTC here)")
	assert.Equal(t, &service.Ref{ID: tent, Name: "Tent"}, got.Zone)

	// Update: move, rename, and take out of any zone.
	name, vis := " Closet Temp ", service.VisibilityZone
	got, err = svc.Sensors.Update(ctx, temp, service.SensorUpdate{
		Name: service.Put(&name), ZoneID: service.Put(&closet), Visibility: service.Put(&vis)})
	require.NoError(t, err)
	assert.Equal(t, "Closet Temp", got.Name)
	assert.Equal(t, closet, got.Zone.ID)
	assert.Equal(t, "zone", got.Visibility)
	assert.Equal(t, "F", got.Unit, "unset fields stay")
	got, err = svc.Sensors.Update(ctx, temp, service.SensorUpdate{ZoneID: service.Put[int](nil)})
	require.NoError(t, err)
	assert.Nil(t, got.Zone)

	bad, missing := "everywhere", 9999
	_, err = svc.Sensors.Update(ctx, temp, service.SensorUpdate{Visibility: service.Put(&bad), ZoneID: service.Put(&missing)})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Contains(t, e.Fields, "visibility")
	assert.Contains(t, e.Fields, "zone_id")
	_, err = svc.Sensors.Update(ctx, 9999, service.SensorUpdate{})
	assert.True(t, service.IsKind(err, service.KindNotFound))

	// Delete takes readings, rollups, plant links and zone VPD inputs.
	testutil.MustExec(t, db, "UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2 WHERE id = $3", temp, humid, tent)
	testutil.MustExec(t, db, `INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		VALUES ($1, '2026-11-10 08:00:00', 1, 2, 1.5, 2)`, temp)
	plant := testutil.SeedPlant(t, db, "P", testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), tent)
	testutil.MustExec(t, db, "INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, 0)", plant, temp)
	before := changes.Load()
	require.NoError(t, svc.Sensors.Delete(ctx, temp))
	assert.Greater(t, changes.Load(), before)
	for _, q := range []string{
		"SELECT COUNT(*) FROM sensors WHERE id = $1",
		"SELECT COUNT(*) FROM sensor_data WHERE sensor_id = $1",
		"SELECT COUNT(*) FROM sensor_data_hourly WHERE sensor_id = $1",
		"SELECT COUNT(*) FROM rolling_averages WHERE sensor_id = $1",
		"SELECT COUNT(*) FROM plant_sensor WHERE sensor_id = $1",
		"SELECT COUNT(*) FROM zones WHERE vpd_temp_sensor_id = $1",
	} {
		var n int
		require.NoError(t, db.QueryRow(q, temp).Scan(&n))
		assert.Zero(t, n, q)
	}
	var humidRef sql.NullInt64
	require.NoError(t, db.QueryRow("SELECT vpd_humidity_sensor_id FROM zones WHERE id = $1", tent).Scan(&humidRef))
	assert.Equal(t, int64(humid), humidRef.Int64, "other zone inputs stay")
	assert.True(t, service.IsKind(svc.Sensors.Delete(ctx, temp), service.KindNotFound))
}

// The dashboard shows zone sensors with readings, and names the plant a
// sensor is linked to only while that plant is active.
func TestSensors_Dashboard(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	svc := testutil.NewServices(t, db)
	ctx := context.Background()

	tent := testutil.SeedZone(t, db, "Tent")
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	soil, err := svc.Sensors.Register(ctx, service.SensorRegistration{Name: "Soil", Source: "ecowitt", Device: "d", Type: "Soil.1", ZoneID: &tent})
	require.NoError(t, err)
	soil2, err := svc.Sensors.Register(ctx, service.SensorRegistration{Name: "Soil 2", Source: "ecowitt", Device: "d", Type: "Soil.2", ZoneID: &tent})
	require.NoError(t, err)
	_, err = svc.Sensors.Register(ctx, service.SensorRegistration{Name: "Silent", Source: "ecowitt", Device: "d", Type: "Soil.3", ZoneID: &tent})
	require.NoError(t, err)
	for _, id := range []int{soil, soil2} {
		testutil.MustExec(t, db, "INSERT INTO sensor_data (sensor_id, value) VALUES ($1, 40)", id)
	}

	var active, done int
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE active = 1 ORDER BY id LIMIT 1").Scan(&active))
	require.NoError(t, db.QueryRow("SELECT id FROM plant_status WHERE active = 0 ORDER BY id LIMIT 1").Scan(&done))
	growing := testutil.SeedPlant(t, db, "Growing", strain, tent)
	finished := testutil.SeedPlant(t, db, "Finished", strain, tent)
	testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-11-01')", growing, active)
	testutil.MustExec(t, db, "INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-11-01')", finished, done)
	testutil.MustExec(t, db, "INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, 0)", growing, soil)
	testutil.MustExec(t, db, "INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, 0)", finished, soil2)

	got, err := svc.Sensors.Dashboard(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "a sensor with no reading isn't shown")
	assert.Equal(t, "Growing", got[0].PlantName)
	assert.Equal(t, "", got[1].PlantName)
	assert.Equal(t, "Tent", got[0].Zone)
}
