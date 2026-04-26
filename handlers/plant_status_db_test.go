package handlers_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// updatePlantStatusLog is unexported in the handlers package and
// therefore not directly callable from package handlers_test. The
// equivalent business logic is exercised end-to-end through
// /plant/status in tests/integration/plant_status_test.go. This file
// covers the DB-shape contracts we *can* reach from outside the
// package (status_log row layout, the migration's seed statuses).

// TestPlantStatusLog_SchemaShape pins the columns plant_status_log
// uses, since several handlers (UpdatePlantStatus, EditStatus,
// DeleteStatus, GetPlant) all read or write into it.
func TestPlantStatusLog_SchemaShape(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedStatusFixture(t, db)

	rows, err := db.Query(`SELECT id, plant_id, status_id, date FROM plant_status_log`)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())

	var id, plantID, statusID int
	var date string
	require.NoError(t, rows.Scan(&id, &plantID, &statusID, &date))
	assert.NotZero(t, id)
	assert.NotZero(t, plantID)
	assert.NotZero(t, statusID)
	assert.NotEmpty(t, date)
}

// TestGetPlantResolvesLatestStatus is a regression check: GetPlant
// reads the most recent plant_status_log entry to populate Status. We
// seed two entries — the older "Veg" and the newer "Flower" — and
// verify GetPlant returns the latter.
func TestGetPlantResolvesLatestStatus(t *testing.T) {
	db := testutil.NewTestDB(t)
	plantID := seedStatusFixture(t, db)

	got := handlers.GetPlant(db, idStr(plantID))
	assert.Equal(t, "Flower", got.Status, "GetPlant should pick the latest status_log entry")
}

// seedStatusFixture builds the FK chain plus a plant with two status
// log rows (Veg → Flower). Returns the plant id.
func seedStatusFixture(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	mustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	mustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	                 VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	mustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)

	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Plant 1', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	plantID, err := res.LastInsertId()
	require.NoError(t, err)

	veg := statusIDByName(t, db, "Veg")
	flower := statusIDByName(t, db, "Flower")

	mustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`, plantID, veg)
	mustExec(t, db, `INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-02-01')`, plantID, flower)

	return plantID
}
