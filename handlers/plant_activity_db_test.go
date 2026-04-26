package handlers_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// seedActivityFixture sets up a small but realistic dataset: two zones,
// two strains, two plants, and a handful of activity rows spanning two
// activities and a date range. Returned ids let the test cases narrow
// queries without re-querying.
type activityFixture struct {
	WaterID, FeedID, NoteID int
	Plant1, Plant2          int64
	Zone1                   int
}

func seedActivityFixture(t *testing.T, db *sql.DB) activityFixture {
	t.Helper()

	exec := func(query string, args ...interface{}) sql.Result {
		res, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
		return res
	}

	// Reference rows. The migration's default activities table already
	// has Water (1) / Feed (2) / Note (3); we read them back rather than
	// re-inserting.
	var waterID, feedID, noteID int
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Water'`).Scan(&waterID))
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Feed'`).Scan(&feedID))
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Note'`).Scan(&noteID))

	exec(`INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	exec(`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	      VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	exec(`INSERT INTO zones (id, name) VALUES (1, 'Zone A')`)
	exec(`INSERT INTO zones (id, name) VALUES (2, 'Zone B')`)

	res := exec(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	             VALUES ('Plant 1', 1, 1, '', 0, '2026-01-01', '[]')`)
	plant1, _ := res.LastInsertId()
	res = exec(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	             VALUES ('Plant 2', 2, 1, '', 0, '2026-01-01', '[]')`)
	plant2, _ := res.LastInsertId()

	// Activity rows: 4 for plant1, 1 for plant2.
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'first water', '2026-01-10')`, plant1, waterID)
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'second water', '2026-01-12')`, plant1, waterID)
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'first feed',   '2026-01-15')`, plant1, feedID)
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'note about leaves', '2026-01-20')`, plant1, noteID)
	exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'plant2 water', '2026-02-01')`, plant2, waterID)

	return activityFixture{
		WaterID: waterID, FeedID: feedID, NoteID: noteID,
		Plant1: plant1, Plant2: plant2, Zone1: 1,
	}
}

// ---------------------------------------------------------------------------
// QueryActivityLog
// ---------------------------------------------------------------------------

func TestQueryActivityLog_NoFiltersReturnsAll(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedActivityFixture(t, db)

	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{}, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 5, page.Total)
	assert.Len(t, page.Entries, 5)
	assert.Equal(t, 1, page.TotalPages)
}

func TestQueryActivityLog_FilterByPlantID(t *testing.T) {
	db := testutil.NewTestDB(t)
	fix := seedActivityFixture(t, db)

	pid := int(fix.Plant1)
	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{PlantID: &pid}, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 4, page.Total, "Plant 1 has 4 activity rows")
	for _, e := range page.Entries {
		assert.Equal(t, "Plant 1", e.PlantName)
	}
}

func TestQueryActivityLog_FilterByActivityIDs(t *testing.T) {
	db := testutil.NewTestDB(t)
	fix := seedActivityFixture(t, db)

	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{
		ActivityIDs: []int{fix.WaterID},
	}, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 3, page.Total, "three water activities across both plants")
	for _, e := range page.Entries {
		assert.Equal(t, "Water", e.ActivityName)
	}
}

func TestQueryActivityLog_FilterByZoneID(t *testing.T) {
	db := testutil.NewTestDB(t)
	fix := seedActivityFixture(t, db)

	zid := fix.Zone1
	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{ZoneID: &zid}, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 4, page.Total, "Plant 1 (zone 1) has 4 activities")
	for _, e := range page.Entries {
		assert.Equal(t, "Zone A", e.ZoneName)
	}
}

func TestQueryActivityLog_FreeTextSearch(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedActivityFixture(t, db)

	// LIKE is case-sensitive on SQLite by default — assert a substring
	// match that's lowercase. The Postgres ILIKE branch is exercised in
	// the build-tagged Phase 6 tests.
	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{Query: "leaves"}, 1, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	assert.Equal(t, "note about leaves", page.Entries[0].Note)
}

func TestQueryActivityLog_Pagination(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedActivityFixture(t, db)

	// 5 rows, page size 2 → 3 pages.
	page1, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{}, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, page1.Total)
	assert.Len(t, page1.Entries, 2)
	assert.Equal(t, 3, page1.TotalPages)

	page3, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{}, 3, 2)
	require.NoError(t, err)
	assert.Len(t, page3.Entries, 1, "last page has the remaining entry")

	// Out-of-range page numbers clamp to TotalPages (no error, no panic).
	pageHigh, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{}, 99, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, pageHigh.Page, "page > TotalPages should clamp to last page")
}

func TestQueryActivityLog_OrderingDateAsc(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedActivityFixture(t, db)

	page, err := handlers.QueryActivityLog(db, handlers.ActivityLogFilters{Order: "date_asc"}, 1, 100)
	require.NoError(t, err)
	require.Len(t, page.Entries, 5)

	for i := 1; i < len(page.Entries); i++ {
		prev := page.Entries[i-1].Date
		curr := page.Entries[i].Date
		assert.Falsef(t, curr.Before(prev),
			"date_asc ordering violated at index %d: %v before %v", i, curr, prev)
	}
}
