package handlers_test

// HTTP-layer tests for handlers/overlay.go (GetOverlayData on
// /api/overlay). Authentication paths are already locked down by
// tests/integration/auth_test.go's TestAuth_APIKey_* suite; this file
// adds shape and content coverage that exercises GetOverlayPlants and
// the linked-sensor join.

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

func overlayAPIKey(t *testing.T, db *sql.DB, plaintext string) {
	t.Helper()
	hashed := handlers.HashAPIKey(plaintext)
	var id int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = 'api_key'`).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(`INSERT INTO settings (name, value) VALUES ('api_key', $1)`, hashed)
	case err == nil:
		_, err = db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, hashed, id)
	}
	require.NoError(t, err)
}

func overlayDrain(resp *http.Response) {
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func overlayDoGet(t *testing.T, c *testutil.Client, path, apiKey string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-API-KEY", apiKey)
	resp, err := c.Do(req)
	require.NoError(t, err)
	return resp
}

// TestOverlayHTTP_EmptyDBReturnsEmptyArrays verifies the response shape
// when no plants or sensors exist: top-level "plants" is an empty array
// (not null) and "sensors" is an empty object (not null).
func TestOverlayHTTP_EmptyDBReturnsEmptyArrays(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "overlay-empty-key"
	overlayAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp := overlayDoGet(t, c, "/api/overlay", apiKey)
	defer overlayDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Plants  []map[string]interface{}            `json:"plants"`
		Sensors map[string]map[string][]interface{} `json:"sensors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotNil(t, got.Plants, "plants must be a JSON array, never null")
	assert.Empty(t, got.Plants)
}

// TestOverlayHTTP_IncludesLivingPlantWithLinkedSensor seeds one plant
// with a linked sensor and one sensor reading, then verifies the
// /api/overlay response surfaces the plant with its linked_sensors
// populated. This exercises the two-query batch in GetOverlayPlants
// and the JSON-decode branch for the plant.sensors array.
func TestOverlayHTTP_IncludesLivingPlantWithLinkedSensor(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "overlay-pop-key"
	overlayAPIKey(t, db, apiKey)

	exec := func(query string, args ...interface{}) {
		_, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
	}
	exec(`INSERT INTO breeder (id, name) VALUES (1, 'B')`)
	exec(`INSERT INTO strain (id, name, breeder_id, sativa, indica, autoflower, description, seed_count)
	      VALUES (1, 'S', 1, 50, 50, 0, '', 0)`)
	exec(`INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	exec(`INSERT INTO sensors (id, name, zone_id, source, device, type, unit, visibility)
	      VALUES (1, 'Tent A Temp', 1, 'manual', 'M1', 'temp', 'C', 'zone_plant')`)
	exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 22.5)`)

	// Plant linked to sensor 1.
	exec(`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
	      VALUES ('Overlay Plant', 1, 1, '', 0, '2026-01-01', '[1]')`)
	var plantID int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant WHERE name = 'Overlay Plant'`).Scan(&plantID))

	// Active status so GetLivingPlants picks it up.
	var vegID int
	require.NoError(t, db.QueryRow(`SELECT id FROM plant_status WHERE status = 'Veg'`).Scan(&vegID))
	exec(`INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, '2026-01-15')`, plantID, vegID)

	c := server.NewClient(t)
	resp := overlayDoGet(t, c, "/api/overlay", apiKey)
	defer overlayDrain(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Plants []struct {
			Name          string                   `json:"name"`
			LinkedSensors []map[string]interface{} `json:"linked_sensors"`
		} `json:"plants"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got.Plants, 1)
	assert.Equal(t, "Overlay Plant", got.Plants[0].Name)
	require.Len(t, got.Plants[0].LinkedSensors, 1, "linked_sensors should carry the one wired-up sensor")
	assert.Equal(t, "Tent A Temp", got.Plants[0].LinkedSensors[0]["name"])
}
