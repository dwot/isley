package handlers_test

// HTTP-layer tests for handlers/sensors.go beyond what
// tests/integration/sensors_test.go and sensor_data_test.go already
// cover. Those exercise EditSensor's happy path, DeleteSensor's cascade,
// IngestSensorData's main scenarios, and ChartHandler. This file fills
// in:
//
//   - auth gating across the api-protected sensor endpoints
//   - ScanACInfinitySensors / ScanEcoWittSensors validation (bad JSON,
//     overlength fields) — the sensor-scan happy paths require live
//     external calls to AC Infinity / EcoWitt, which Phase 5 explicitly
//     forbids in tests
//   - DumpACInfinityJSON — error path when ACIToken is unconfigured
//   - EditSensor — additional validation branches not covered upstream

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/tests/testutil"
)

// newStoreWithACIToken returns a fresh *config.Store seeded with the
// given ACIToken. Used by ACI-related test helpers below.
func newStoreWithACIToken(token string) *config.Store {
	s := config.NewStore()
	s.SetACIToken(token)
	return s
}

// newStoreWithAPIIngest returns a fresh *config.Store with
// APIIngestEnabled set. Used by ingest validation tests.
func newStoreWithAPIIngest(enabled int) *config.Store {
	s := config.NewStore()
	s.SetAPIIngestEnabled(enabled)
	return s
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestSensorsHTTP_AuthGating(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/sensors/scanACI"},
		{http.MethodPost, "/sensors/scanEC"},
		{http.MethodPost, "/sensors/edit"},
		{http.MethodDelete, "/sensors/delete/1"},
		{http.MethodGet, "/sensors/dumpACI"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer testutil.DrainAndClose(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// ScanACInfinitySensors — validation only (success requires a live ACI API)
// ---------------------------------------------------------------------------

func TestSensorsHTTP_ScanACI_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "scan-aci-bad-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/scanACI", apiKey,
		bytes.NewBufferString(`{not-json`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_ScanACI_RejectsLongNewZone(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "scan-aci-long-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/scanACI", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"new_zone": strings.Repeat("z", 1024),
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// ScanEcoWittSensors — validation only
// ---------------------------------------------------------------------------

func TestSensorsHTTP_ScanEC_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "scan-ec-bad-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/scanEC", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_ScanEC_RejectsLongNewZone(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "scan-ec-zone-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/scanEC", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"new_zone":       strings.Repeat("z", 1024),
			"server_address": "192.168.1.1",
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_ScanEC_RejectsLongServerAddress(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "scan-ec-addr-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/scanEC", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"server_address": strings.Repeat("a", 1024),
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DumpACInfinityJSON
// ---------------------------------------------------------------------------

// TestSensorsHTTP_DumpACI_FailsWhenTokenUnconfigured exercises the only
// branch we can reach without a live ACI server: the up-front guard on
// the engine's ACIToken setting. Constructs a per-test Store with an
// empty token so the assertion is local to this engine instance.
func TestSensorsHTTP_DumpACI_FailsWhenTokenUnconfigured(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(newStoreWithACIToken("")))

	const apiKey = "dump-aci-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/sensors/dumpACI", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"missing ACIToken must surface as 500 with api_aci_token_not_configured")
}

// ---------------------------------------------------------------------------
// EditSensor — validation branches not exercised by the integration suite
// ---------------------------------------------------------------------------

func TestSensorsHTTP_EditSensor_RejectsBadJSON(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "edit-bad-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/edit", apiKey,
		bytes.NewBufferString(`{"id":}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_EditSensor_RejectsLongName(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "edit-long-name-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/edit", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"id":         1,
			"name":       strings.Repeat("n", 1024),
			"visibility": "zone",
			"zone_id":    1,
			"unit":       "C",
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_EditSensor_RejectsLongUnit(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "edit-long-unit-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/edit", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"id":         1,
			"name":       "ok",
			"visibility": "zone",
			"zone_id":    1,
			"unit":       strings.Repeat("u", 256),
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_EditSensor_RejectsUnknownVisibility(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "edit-bad-vis-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/sensors/edit", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"id":         1,
			"name":       "ok",
			"visibility": "totally-bogus", // not in {zone_plant, zone, plant, hide}
			"zone_id":    1,
			"unit":       "C",
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeleteSensor — additional path: deleting a non-existent sensor
//
// DeleteSensorByID issues two unconditional DELETEs; missing rows are a
// no-op, so the handler still reports success. This locks down that
// behavior so a later refactor doesn't accidentally start 404'ing.
// ---------------------------------------------------------------------------

func TestSensorsHTTP_DeleteSensor_NoOpOnMissing(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "del-missing-sensor-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/sensors/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// IngestSensorData — additional validation branches
// (happy paths and the disabled-by-config branch live in the integration suite)
// ---------------------------------------------------------------------------

func TestSensorsHTTP_Ingest_RejectsLongDevice(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(newStoreWithAPIIngest(1)))

	const apiKey = "ingest-long-device-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/api/sensors/ingest", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"source": "ecowitt",
			"device": strings.Repeat("d", 1024),
			"type":   "temp",
			"value":  21.5,
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_Ingest_RejectsLongType(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(newStoreWithAPIIngest(1)))

	const apiKey = "ingest-long-type-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/api/sensors/ingest", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"source": "ecowitt",
			"device": "dev1",
			"type":   strings.Repeat("t", 1024),
			"value":  21.5,
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorsHTTP_Ingest_RejectsLongUnit(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(newStoreWithAPIIngest(1)))

	const apiKey = "ingest-long-unit-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/api/sensors/ingest", apiKey,
		testutil.JSONBody(t, map[string]interface{}{
			"source": "ecowitt",
			"device": "dev1",
			"type":   "temp",
			"value":  21.5,
			"unit":   strings.Repeat("u", 1024),
		}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
