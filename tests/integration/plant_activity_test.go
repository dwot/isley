package integration

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// activityFixture wires up the FK chain (breeder → strain → zone →
// plant) plus the API key, ready for activity/measurement endpoints.
type activityHTTPFixture struct {
	APIKey   string
	PlantID  int64
	WaterID  int
	FeedID   int
	HeightID int
}

func seedActivityHTTP(t *testing.T, db *sql.DB) activityHTTPFixture {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	plantID := int64(testutil.SeedPlant(t, db, "Plant 1", strainID, zoneID))

	var waterID, feedID, heightID int
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Water'`).Scan(&waterID))
	require.NoError(t, db.QueryRow(`SELECT id FROM activity WHERE name='Feed'`).Scan(&feedID))
	require.NoError(t, db.QueryRow(`SELECT id FROM metric WHERE name='Height'`).Scan(&heightID))

	return activityHTTPFixture{
		APIKey:  testutil.SeedAPIKey(t, db, "test-activity-key"),
		PlantID: plantID,
		WaterID: waterID, FeedID: feedID, HeightID: heightID,
	}
}

// ---------------------------------------------------------------------------
// POST /plantActivity
// ---------------------------------------------------------------------------

func TestActivity_CreateHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plantActivity", fix.APIKey, map[string]interface{}{
		"plant_id":    fix.PlantID,
		"activity_id": fix.WaterID,
		"note":        "first water",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE plant_id = $1`, fix.PlantID).Scan(&n))
	assert.Equal(t, 1, n, "activity row should be persisted")
}

func TestActivity_CreateRejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/plantActivity", strings.NewReader("{not json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", fix.APIKey)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /plantActivity/edit
// ---------------------------------------------------------------------------

func TestActivity_EditUpdatesRow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'orig note', '2026-04-01')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)
	actID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/plantActivity/edit", fix.APIKey, map[string]interface{}{
		"id":          actID,
		"date":        "2026-04-15",
		"activity_id": fix.FeedID, // change activity from Water → Feed
		"note":        "updated note",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var note string
	var actName int
	require.NoError(t, db.QueryRow(
		`SELECT note, activity_id FROM plant_activity WHERE id = $1`, actID,
	).Scan(&note, &actName))
	assert.Equal(t, "updated note", note)
	assert.Equal(t, fix.FeedID, actName)
}

// ---------------------------------------------------------------------------
// DELETE /plantActivity/delete/:id
// ---------------------------------------------------------------------------

func TestActivity_DeleteRemovesRow(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'doomed', '2026-04-25')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)
	actID, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plantActivity/delete/"+strconv.FormatInt(actID, 10), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE id = $1`, actID).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// POST /record-multi-activity
// ---------------------------------------------------------------------------

func TestActivity_MultiPlantHappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	// Add a second plant so multi-plant insert covers >1 row.
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Plant 2', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	plant2, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/record-multi-activity", fix.APIKey, map[string]interface{}{
		"plant_ids":   []int64{fix.PlantID, plant2},
		"activity_id": fix.WaterID,
		"note":        "scheduled water",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_activity WHERE note = 'scheduled water'`).Scan(&n))
	assert.Equal(t, 2, n, "one row per plant in plant_ids")
}

func TestActivity_MultiPlantRejectsEmptyList(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/record-multi-activity", fix.APIKey, map[string]interface{}{
		"plant_ids":   []int{},
		"activity_id": fix.WaterID,
		"note":        "no plants",
		"date":        "2026-04-25",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GET /activities/list  (basic / session-gated)
// ---------------------------------------------------------------------------

func TestActivity_ListPaginated(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	// 3 activity rows.
	for i, dt := range []string{"2026-04-10", "2026-04-12", "2026-04-15"} {
		_, err := db.Exec(
			`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)`,
			fix.PlantID, fix.WaterID, "row "+strconv.Itoa(i), dt,
		)
		require.NoError(t, err)
	}

	testutil.SeedAdmin(t, db, "act-pw")
	c := server.LoginAsAdmin(t, "act-pw")

	resp := c.Get("/activities/list?page=1&page_size=2")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Entries  []handlers.ActivityLogEntry `json:"entries"`
		Total    int                         `json:"total"`
		Page     int                         `json:"page"`
		PageSize int                         `json:"page_size"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, 3, got.Total)
	assert.Len(t, got.Entries, 2, "page_size=2 returns at most 2 entries")
	assert.Equal(t, 1, got.Page)
}

// TestActivity_ListPaginationCap exercises the page_size guard in
// ListAllActivities. The SPA-style activities page requests
// page_size=10000 to render the full log client-side; the cap was
// raised from 1000 to 100000 to support that without removing the
// defense-in-depth ceiling.
func TestActivity_ListPaginationCap(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	// Seed a small number of rows; we're testing the page_size guard,
	// not pagination behavior at scale.
	for i := 0; i < 5; i++ {
		_, err := db.Exec(
			`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)`,
			fix.PlantID, fix.WaterID, "row "+strconv.Itoa(i), "2026-04-10",
		)
		require.NoError(t, err)
	}

	testutil.SeedAdmin(t, db, "cap-pw")
	c := server.LoginAsAdmin(t, "cap-pw")

	type listResp struct {
		Entries  []handlers.ActivityLogEntry `json:"entries"`
		Total    int                         `json:"total"`
		Page     int                         `json:"page"`
		PageSize int                         `json:"page_size"`
	}

	cases := []struct {
		name         string
		query        string
		wantPageSize int
	}{
		// Within the new ceiling — request honored.
		{"under_new_ceiling", "?page_size=5000", 5000},
		// At the SPA's typical request size — honored.
		{"spa_request_size", "?page_size=10000", 10000},
		// Above the new ceiling — falls back to default.
		{"above_ceiling", "?page_size=200000", 100},
		// Zero — falls back to default (n > 0 guard).
		{"zero", "?page_size=0", 100},
		// Negative — falls back to default.
		{"negative", "?page_size=-5", 100},
		// Pre-existing small value — unchanged.
		{"small_value", "?page_size=50", 50},
		// Unset — default applies.
		{"unset", "", 100},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resp := c.Get("/activities/list" + tc.query)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var got listResp
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
			assert.Equal(t, tc.wantPageSize, got.PageSize,
				"page_size=%q should report PageSize=%d", tc.query, tc.wantPageSize)
			assert.Equal(t, 5, got.Total, "all 5 seeded rows should be counted")
		})
	}
}

func TestActivity_ListFilterByPlantID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	// Add a second plant + activity so the filter has something to drop.
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ('Plant 2', 1, 1, '', 0, '2026-01-01', '[]')`,
	)
	require.NoError(t, err)
	plant2, _ := res.LastInsertId()

	_, _ = db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'p1', '2026-04-10')`,
		fix.PlantID, fix.WaterID,
	)
	_, _ = db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'p2', '2026-04-10')`,
		plant2, fix.WaterID,
	)

	testutil.SeedAdmin(t, db, "filter-pw")
	c := server.LoginAsAdmin(t, "filter-pw")

	resp := c.Get("/activities/list?plant_id=" + strconv.FormatInt(fix.PlantID, 10))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Entries []handlers.ActivityLogEntry `json:"entries"`
		Total   int                         `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, 1, got.Total)
	assert.Equal(t, "p1", got.Entries[0].Note)
}

// ---------------------------------------------------------------------------
// GET /activities/export/csv  (protected — session-gated)
// ---------------------------------------------------------------------------

func TestActivity_ExportCSV(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedActivityHTTP(t, db)

	_, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, 'csv test row', '2026-04-25')`,
		fix.PlantID, fix.WaterID,
	)
	require.NoError(t, err)

	testutil.SeedAdmin(t, db, "csv-pw")
	c := server.LoginAsAdmin(t, "csv-pw")

	resp := c.Get("/activities/export/csv")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/csv")
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "attachment")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	csv := string(body)
	assert.Contains(t, csv, "Date,Plant,Strain,Zone,Activity,Note", "header row")
	assert.Contains(t, csv, "csv test row", "seeded note must appear")
}
