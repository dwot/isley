package handlers_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func plantSensorLinks(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT p.name, ps.sensor_id FROM plant_sensor ps JOIN plant p ON p.id = ps.plant_id ORDER BY p.name, ps.position`)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		var id int
		require.NoError(t, rows.Scan(&name, &id))
		out = append(out, name+":"+itoaT(id))
	}
	require.NoError(t, rows.Err())
	return out
}

func itoaT(n int) string { return strconv.Itoa(n) }

// A backup made before plant_sensor existed carries each plant's linked
// sensors as JSON in plant.sensors. Restoring it builds the join rows,
// in order, without duplicates or links to sensors it doesn't have.
func TestApplyBackupToDB_LegacyPlantSensorJSON(t *testing.T) {
	t.Parallel()
	assertLegacyPlantSensorJSON(t, testutil.NewTestDB(t))
}

func assertLegacyPlantSensorJSON(t *testing.T, db *sql.DB) {
	t.Helper()
	payload := handlers.BackupPayload{
		Breeders: []map[string]interface{}{{"id": 1, "name": "B"}},
		Strains: []map[string]interface{}{{"id": 1, "name": "S", "breeder_id": 1, "sativa": 50, "indica": 50,
			"autoflower": 0, "description": "", "seed_count": 0}},
		Sensors: []map[string]interface{}{
			{"id": 1, "name": "s1", "source": "x", "device": "d", "type": "t1"},
			{"id": 2, "name": "s2", "source": "x", "device": "d", "type": "t2"},
		},
		Plants: []map[string]interface{}{
			{"id": 1, "name": "A", "description": "", "clone": 0, "strain_id": 1, "sensors": "[2, 1, 2, 99]"},
			{"id": 2, "name": "B", "description": "", "clone": 0, "strain_id": 1, "sensors": "[]"},
		},
	}
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), db, payload))
	assert.Equal(t, []string{"A:2", "A:1"}, plantSensorLinks(t, db))
}

// A current backup round-trips its plant_sensor rows.
func TestBackupArchive_PlantSensorRoundTrip(t *testing.T) {
	t.Parallel()
	src := testutil.NewTestDB(t)
	strain := testutil.SeedStrain(t, src, testutil.SeedBreeder(t, src, "B"), "S")
	zone := testutil.SeedZone(t, src, "Z")
	plant := testutil.SeedPlant(t, src, "A", strain, zone)
	s1 := testutil.SeedSensor(t, src, "x", "d", "t1")
	s2 := testutil.SeedSensor(t, src, "x", "d", "t2")
	testutil.MustExec(t, src, `INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, 0), ($1, $3, 1)`, plant, s2, s1)

	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	require.Len(t, payload.PlantSensors, 2)

	dst := testutil.NewTestDB(t)
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, payload))
	assert.Equal(t, []string{"A:" + itoaT(s2), "A:" + itoaT(s1)}, plantSensorLinks(t, dst))
}
