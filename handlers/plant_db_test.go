package handlers_test

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// External package — handlers_test — to avoid the
// handlers→app→handlers cycle that arises when handlers' internal
// _test.go files import tests/testutil.

// ---------------------------------------------------------------------------
// Reference-table getters (rely on default seed data baked into migrations)
// ---------------------------------------------------------------------------

func TestGetActivities_DefaultSeed(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	// Migration 001 inserts Water/Feed/Note as built-ins.
	got := handlers.GetActivities(db)
	assert.GreaterOrEqual(t, len(got), 3, "default seed should produce ≥3 activities")

	names := map[string]bool{}
	for _, a := range got {
		names[a.Name] = true
	}
	for _, want := range []string{"Water", "Feed", "Note"} {
		assert.Truef(t, names[want], "missing default activity %q", want)
	}
}

func TestGetMetrics_DefaultSeed(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	got := handlers.GetMetrics(db)
	assert.GreaterOrEqual(t, len(got), 1)

	hasHeight := false
	for _, m := range got {
		if m.Name == "Height" {
			hasHeight = true
			assert.Equal(t, "in", m.Unit)
		}
	}
	assert.True(t, hasHeight, "default Height metric should be present")
}

func TestGetStatuses_OrderedByStatusOrder(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	got := handlers.GetStatuses(db)
	require.NotEmpty(t, got)

	// Migration 004 sets explicit status_order values; the query orders by
	// that column, so Germinating should come before Planted, Planted
	// before Seedling, etc.
	want := []string{"Germinating", "Planted", "Seedling", "Veg", "Flower"}
	if len(got) < len(want) {
		t.Fatalf("expected at least %d statuses, got %d", len(want), len(got))
	}
	for i, w := range want {
		assert.Equalf(t, w, got[i].Status, "status_order index %d", i)
	}
}

// ---------------------------------------------------------------------------
// DeletePlantById — must remove plant + every child row
// ---------------------------------------------------------------------------

func TestDeletePlantById_RemovesChildren(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	plantID := seedPlantTree(t, db)

	// Add a row in each child table so we can confirm cascade-style cleanup.
	exec := func(query string, args ...interface{}) {
		_, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
	}
	exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, 1, '2026-01-15')`, plantID)
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, 1, '', '2026-01-16')`, plantID)
	exec(`INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, 1, 12.5, '2026-01-16')`, plantID)
	exec(`INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date) VALUES ($1, '/x.jpg', 'desc', 0, '2026-01-16')`, plantID)

	// Sanity: each child table has at least one row pointing at the plant.
	for _, table := range []string{"plant_status_log", "plant_activity", "plant_measurements", "plant_images"} {
		require.Equal(t, 1, childCount(t, db, table, plantID),
			"%s should have one row before delete", table)
	}

	require.NoError(t, handlers.DeletePlantById(db, idStr(plantID)))

	// Plant gone.
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant WHERE id = $1`, plantID).Scan(&n))
	assert.Zero(t, n, "plant row must be deleted")

	// Each child table now empty for this plant.
	for _, table := range []string{"plant_status_log", "plant_activity", "plant_measurements", "plant_images"} {
		assert.Zerof(t, childCount(t, db, table, plantID),
			"%s should have no rows for the deleted plant", table)
	}
}

// ---------------------------------------------------------------------------
// GetPlant — returns expected fields for a seeded plant tree
// ---------------------------------------------------------------------------

func TestGetPlant_PopulatesFields(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	plantID := seedPlantTree(t, db)

	// Add a status_log row so GetPlant can resolve current status.
	_, err := db.Exec(
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		plantID, statusIDByName(t, db, "Veg"),
	)
	require.NoError(t, err)

	got := handlers.GetPlant(db, idStr(plantID))

	assert.Equal(t, uint(plantID), got.ID)
	assert.Equal(t, "Test Plant", got.Name)
	assert.Equal(t, "Test Strain", got.StrainName)
	assert.Equal(t, "Test Breeder", got.BreederName)
	assert.Equal(t, "Test Zone", got.ZoneName)
	assert.Equal(t, "Veg", got.Status, "current status should resolve via status_log join")
	assert.Equal(t, 1, got.StrainID)
	assert.False(t, got.IsClone, "default clone=0 should map to false")
}

func TestGetPlant_UnknownIDReturnsZeroValue(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	// No rows inserted; GetPlant on a missing ID logs and returns the
	// zero value of types.Plant (current code path).
	got := handlers.GetPlant(db, "9999")
	assert.Zero(t, got.ID, "unknown id should yield zero-value plant")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// seedPlantTree creates the minimal hierarchy needed to satisfy the FKs
// on the plant table: breeder → strain → zone → plant. Returns the plant
// id. The caller adds child rows (status logs, activities, etc.) on top.
func seedPlantTree(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'Test Breeder')`)
	testutil.MustExec(t, db, `INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	          VALUES (1, 'Test Strain', 1, 50, 50, 0, '', 0)`)
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Test Zone')`)
	res, err := db.Exec(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	                 VALUES ('Test Plant', 1, 1, '', 0, '2026-01-01', '[]')`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func childCount(t *testing.T, db *sql.DB, table string, plantID int64) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE plant_id = $1`, plantID).Scan(&n))
	return n
}

func idStr(id int64) string {
	return strconv.FormatInt(id, 10)
}

func statusIDByName(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var id int
	require.NoErrorf(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = $1`, name).Scan(&id),
		"plant_status row %q must exist", name)
	return id
}
