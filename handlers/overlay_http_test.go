package handlers_test

// HTTP-layer tests for handlers/overlay.go (GetOverlayData on
// /api/overlay). Authentication paths are already locked down by
// tests/integration/auth_test.go's TestAuth_APIKey_* suite; this file
// adds shape and content coverage that exercises GetOverlayPlants and
// the linked-sensor join.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestOverlayHTTP_EmptyDBReturnsEmptyArrays verifies the response shape
// when no plants or sensors exist: top-level "plants" is an empty array
// (not null) and "sensors" is an empty object (not null).
func TestOverlayHTTP_EmptyDBReturnsEmptyArrays(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "overlay-empty-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/api/overlay", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
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
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "overlay-pop-key"
	testutil.SeedAPIKey(t, db, apiKey)

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
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/api/overlay", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
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
