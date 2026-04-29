package integration

import (
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/model/types"
	"isley/tests/testutil"
)

// Tests for PR #160 — Watering / Feeding activity flags. The migration
// adds is_watering / is_feeding columns to the activity table and the
// dashboard / plant page calculations now rely on the flags rather than
// hardcoded names or IDs, so the built-in "Water" and "Feed" entries can
// be renamed or translated without breaking tracking.

// TestActivityFlags_MigrationFlagsBuiltins asserts the 017 migration
// backfilled the built-in Water/Feed activities with the matching flag
// (and left other built-ins like Note alone).
func TestActivityFlags_MigrationFlagsBuiltins(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	cases := []struct {
		name        string
		wantWater   bool
		wantFeeding bool
	}{
		{"Water", true, false},
		{"Feed", false, true},
		{"Note", false, false},
	}
	for _, tc := range cases {
		var isWatering, isFeeding bool
		require.NoErrorf(t,
			db.QueryRow(`SELECT is_watering, is_feeding FROM activity WHERE name = $1`, tc.name).
				Scan(&isWatering, &isFeeding),
			"built-in activity %q must exist after migrations", tc.name)
		assert.Equalf(t, tc.wantWater, isWatering, "is_watering for %q", tc.name)
		assert.Equalf(t, tc.wantFeeding, isFeeding, "is_feeding for %q", tc.name)
	}
}

// TestActivityFlags_AddRoundTrip verifies POST /activities persists both
// flags as supplied in the JSON payload and that the returned ID can be
// looked up in the activity table with the same flag state.
func TestActivityFlags_AddRoundTrip(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := testutil.SeedAPIKey(t, db, "test-activity-flags-add-key")

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{
		"activity_name": "Soak",
		"is_watering":   true,
		"is_feeding":    true,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()

	var name string
	var isWatering, isFeeding bool
	require.NoError(t, db.QueryRow(
		`SELECT name, is_watering, is_feeding FROM activity WHERE id = $1`, id,
	).Scan(&name, &isWatering, &isFeeding))
	assert.Equal(t, "Soak", name)
	assert.True(t, isWatering, "is_watering must be true after POST with is_watering=true")
	assert.True(t, isFeeding, "is_feeding must be true after POST with is_feeding=true")
}

// TestActivityFlags_UpdateRoundTripAndCacheRefresh covers two contracts:
// (1) PUT /activities/:id flips the flag columns in the database, and
// (2) the per-engine ConfigStore cache reflects the new flag state
// without a process restart. Without (2) the dropdowns rendered from the
// cache would silently lag the database until the app was restarted.
func TestActivityFlags_UpdateRoundTripAndCacheRefresh(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := testutil.SeedAPIKey(t, db, "test-activity-flags-update-key")

	c := server.NewClient(t)

	// Create with both flags off.
	resp := c.APIPostJSON(t, "/activities", apiKey, map[string]interface{}{
		"activity_name": "Mist",
		"is_watering":   false,
		"is_feeding":    false,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	id := readID(t, resp)
	resp.Body.Close()

	// AppendActivity should have placed the new row in the cache as-is.
	require.True(t, cachedActivityHasFlags(server.ConfigStore.Activities(), id, false, false),
		"AppendActivity should put the new activity into the cache with the supplied flags")

	// Flip both flags via PUT.
	resp = apiPutJSON(t, c, "/activities/"+strconv.Itoa(id), apiKey, map[string]interface{}{
		"activity_name": "Mist",
		"is_watering":   true,
		"is_feeding":    true,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	var isWatering, isFeeding bool
	require.NoError(t, db.QueryRow(
		`SELECT is_watering, is_feeding FROM activity WHERE id = $1`, id,
	).Scan(&isWatering, &isFeeding))
	assert.True(t, isWatering, "PUT must update is_watering in the DB")
	assert.True(t, isFeeding, "PUT must update is_feeding in the DB")

	assert.True(t, cachedActivityHasFlags(server.ConfigStore.Activities(), id, true, true),
		"UpdateActivityHandler must refresh the in-memory ConfigStore so dropdowns see the new flags without a restart")
}

// TestActivityFlags_LoadPlantActivitiesPopulatesFlags asserts that the
// flags surface on the per-plant activity rows GetPlant exposes — the
// dashboard plant page reads them and reflects them in the rendered
// JSON for the activity log.
func TestActivityFlags_LoadPlantActivitiesPopulatesFlags(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	plantID, waterID, feedID := seedPlantWithBuiltinActivities(t, db)
	mustSetVegStatus(t, db, plantID)
	when := time.Date(2026, 4, 25, 8, 0, 0, 0, time.UTC)
	testutil.SeedActivity(t, db, plantID, waterID, when)
	testutil.SeedActivity(t, db, plantID, feedID, when.Add(24*time.Hour))

	plant := handlers.GetPlant(db, strconv.Itoa(plantID))
	require.GreaterOrEqual(t, len(plant.Activities), 2, "GetPlant should return both seeded activities")

	var sawWatering, sawFeeding bool
	for _, a := range plant.Activities {
		switch a.ActivityId {
		case waterID:
			assert.True(t, a.IsWatering, "Water activity row must surface IsWatering=true")
			assert.False(t, a.IsFeeding, "Water activity row must NOT surface IsFeeding=true")
			sawWatering = true
		case feedID:
			assert.True(t, a.IsFeeding, "Feed activity row must surface IsFeeding=true")
			assert.False(t, a.IsWatering, "Feed activity row must NOT surface IsWatering=true")
			sawFeeding = true
		}
	}
	assert.True(t, sawWatering, "did not find the seeded Water activity in the plant view")
	assert.True(t, sawFeeding, "did not find the seeded Feed activity in the plant view")
}

// TestActivityFlags_DeriveActivityDatesUsesFlagsNotIDs locks in the new
// behavior of deriveActivityDates: a custom activity flagged as watering
// (with no relation to the built-in Water row) drives LastWaterDate.
// Pre-PR #160 this relied on hardcoded ActivityWater/ActivityFeed IDs,
// which drift between installs.
func TestActivityFlags_DeriveActivityDatesUsesFlagsNotIDs(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	plantID, waterID, _ := seedPlantWithBuiltinActivities(t, db)
	mustSetVegStatus(t, db, plantID)

	// Custom activity with is_watering=TRUE and a guaranteed-non-1 id.
	soakID := insertActivityWithFlags(t, db, "Soak", true, false)
	require.NotEqualf(t, waterID, soakID, "Soak id must differ from the built-in Water id (%d)", waterID)

	// Built-in Water on day 1; custom Soak on day 5 (later, so it should win).
	day1 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	day5 := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	testutil.SeedActivity(t, db, plantID, waterID, day1)
	testutil.SeedActivity(t, db, plantID, soakID, day5)

	plant := handlers.GetPlant(db, strconv.Itoa(plantID))
	assert.Equal(t, day5.Format("2006-01-02"), plant.LastWaterDate.Format("2006-01-02"),
		"LastWaterDate must come from the most recent IS_WATERING-flagged activity, not from name='Water' / a hardcoded ID")
}

// TestActivityFlags_DashboardUsesFlagsNotName proves the dashboard
// "days since last watering" SELECT now keys off is_watering rather
// than name='Water'. The plan suggests renaming the built-in to
// confirm — that is exactly what we do.
func TestActivityFlags_DashboardUsesFlagsNotName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	plantID, waterID, _ := seedPlantWithBuiltinActivities(t, db)
	mustSetVegStatus(t, db, plantID)

	// Log a watering 3 days ago against the (still-named) built-in Water.
	threeDaysAgo := time.Now().AddDate(0, 0, -3)
	testutil.SeedActivity(t, db, plantID, waterID, threeDaysAgo)

	// Rename the built-in to remove the legacy `name='Water'` match path.
	// If the dashboard query still keyed off the name, days_since_last_watering
	// would now go to zero. Because is_watering is still TRUE, it stays > 0.
	testutil.MustExec(t, db, `UPDATE activity SET name = 'H2O Renamed' WHERE id = $1`, waterID)

	plants := handlers.GetLivingPlants(db)
	require.NotEmpty(t, plants, "GetLivingPlants must return the seeded plant")

	var found bool
	for _, p := range plants {
		if p.ID == plantID {
			found = true
			assert.GreaterOrEqualf(t, p.DaysSinceLastWatering, 3,
				"DaysSinceLastWatering must reflect the IS_WATERING-flagged activity even after renaming the built-in Water row (got %d)", p.DaysSinceLastWatering)
		}
	}
	assert.True(t, found, "seeded plant must be present in GetLivingPlants result")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// seedPlantWithBuiltinActivities sets up a living plant and returns its
// id alongside the resolved Water / Feed activity ids. The migration
// seeds those rows with stable names but install-dependent ids — tests
// must look them up rather than assume 1/2 (which is precisely the
// brittleness PR #160 fixed).
func seedPlantWithBuiltinActivities(t *testing.T, db *sql.DB) (plantID, waterID, feedID int) {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "Test Breeder")
	strainID := testutil.SeedStrain(t, db, breederID, "Test Strain")
	zoneID := testutil.SeedZone(t, db, "Test Zone")
	plantID = testutil.SeedPlant(t, db, "Test Plant", strainID, zoneID)
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name = 'Water'`).Scan(&waterID))
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name = 'Feed'`).Scan(&feedID))
	return plantID, waterID, feedID
}

// insertActivityWithFlags inserts a row directly so the test does not
// have to spin up the HTTP server just to plant a fixture.
func insertActivityWithFlags(t *testing.T, db *sql.DB, name string, isWatering, isFeeding bool) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(
		`INSERT INTO activity (name, is_watering, is_feeding) VALUES ($1, $2, $3) RETURNING id`,
		name, isWatering, isFeeding,
	).Scan(&id))
	return id
}

// mustSetVegStatus links the plant to an "active" plant_status row so
// it appears in GetLivingPlants. The dashboard query joins on the most
// recent plant_status_log entry; we insert one explicitly here.
func mustSetVegStatus(t *testing.T, db *sql.DB, plantID int) {
	t.Helper()
	var statusID int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Veg'`).Scan(&statusID))
	testutil.MustExec(t, db,
		`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`,
		plantID, statusID)
}

// cachedActivityHasFlags reports whether the activity slice contains an
// entry with the given id and flag state.
func cachedActivityHasFlags(activities []types.Activity, id int, isWatering, isFeeding bool) bool {
	for _, a := range activities {
		if a.ID == id {
			return a.IsWatering == isWatering && a.IsFeeding == isFeeding
		}
	}
	return false
}
