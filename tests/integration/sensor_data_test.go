package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/handlers"
	"isley/model/types"
	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// withAPIIngestEnabled snapshots config.APIIngestEnabled, sets it to
// the desired value, and restores on cleanup. Tests touching the
// ingest endpoint should call this so they don't permanently mutate
// the package-global config (Phase 7 will replace it with a Store).
func withAPIIngestEnabled(t *testing.T, enabled int) {
	t.Helper()
	prev := config.APIIngestEnabled
	config.APIIngestEnabled = enabled
	t.Cleanup(func() { config.APIIngestEnabled = prev })
}

// withPollingInterval pins config.PollingInterval for the duration of
// a test. ChartHandler caches results for `PollingInterval/10` seconds;
// pinning the value avoids cross-test interference via the cache.
func withPollingInterval(t *testing.T, seconds int) {
	t.Helper()
	prev := config.PollingInterval
	config.PollingInterval = seconds
	t.Cleanup(func() { config.PollingInterval = prev })
}

// resetIngestRateLimiter swaps in a fresh limiter so this test starts
// from a clean count. Restores the original on cleanup. Mirrors the
// resetRateLimit pattern used elsewhere for /login.
func resetIngestRateLimiter(t *testing.T) {
	t.Helper()
	prev := handlers.IngestRateLimiter
	handlers.IngestRateLimiter = handlers.NewRateLimiter(60, time.Minute)
	t.Cleanup(func() { handlers.IngestRateLimiter = prev })
}

// seedSensorIngestKey creates an API key suitable for X-API-KEY auth
// against the ingest endpoint and returns the plaintext.
func seedSensorIngestKey(t *testing.T, db *sql.DB) string {
	t.Helper()
	return testutil.SeedAPIKey(t, db, "test-sensor-ingest-key")
}

// ingestPostKeepsRateOK retries the ingest call once if the limiter
// blocks — the global IngestRateLimiter persists across tests and we
// guard explicitly via resetIngestRateLimiter, but adding a defensive
// retry here makes the suite robust to ordering changes.
func ingestPostKeepsRateOK(t *testing.T, c *testutil.Client, apiKey string, body interface{}) *http.Response {
	t.Helper()
	resp := c.APIPostJSON(t, "/api/sensors/ingest", apiKey, body)
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		resetIngestRateLimiter(t)
		resp = c.APIPostJSON(t, "/api/sensors/ingest", apiKey, body)
	}
	return resp
}

// ---------------------------------------------------------------------------
// POST /api/sensors/ingest
// ---------------------------------------------------------------------------

func TestSensorIngest_HappyPath(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	resp := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"source": "test-src",
		"device": "DEVICE-1",
		"type":   "temp",
		"value":  21.5,
		"unit":   "C",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		Message  string `json:"message"`
		SensorID int    `json:"sensor_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.SensorID, "ingest should auto-create the sensor and return its id")

	// Sensor row exists with default visibility.
	var name, source, deviceCol, kind, visibility string
	require.NoError(t, db.QueryRow(
		`SELECT name, source, device, type, visibility FROM sensors WHERE id = $1`, got.SensorID,
	).Scan(&name, &source, &deviceCol, &kind, &visibility))
	assert.Equal(t, "test-src", source)
	assert.Equal(t, "DEVICE-1", deviceCol)
	assert.Equal(t, "temp", kind)
	assert.Equal(t, "zone_plant", visibility, "auto-created sensors get zone_plant visibility")
	assert.Contains(t, name, "test-src", "auto-generated name uses source/device/type")

	// Reading row exists with the correct value.
	var v float64
	require.NoError(t, db.QueryRow(
		`SELECT value FROM sensor_data WHERE sensor_id = $1`, got.SensorID,
	).Scan(&v))
	assert.InDelta(t, 21.5, v, 0.0001)
}

func TestSensorIngest_ReusesExistingSensor(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	r1 := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"source": "src", "device": "D", "type": "temp", "value": 1.0,
	})
	r1.Body.Close()
	r2 := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"source": "src", "device": "D", "type": "temp", "value": 2.0,
	})
	r2.Body.Close()

	var sensorCount, readingCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensors WHERE source='src' AND device='D' AND type='temp'`).Scan(&sensorCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data`).Scan(&readingCount))
	assert.Equal(t, 1, sensorCount, "second ingest should reuse the existing sensor")
	assert.Equal(t, 2, readingCount, "both readings should be persisted")
}

func TestSensorIngest_CreatesNewZone(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	resp := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"source": "src", "device": "D", "type": "temp", "value": 1.0,
		"new_zone": "Greenhouse 7",
	})
	resp.Body.Close()

	// new_zone created.
	var zoneID int
	require.NoError(t, db.QueryRow(`SELECT id FROM zones WHERE name = 'Greenhouse 7'`).Scan(&zoneID))
	assert.NotZero(t, zoneID)

	// Auto-created sensor is linked to it.
	var linkedID sql.NullInt64
	require.NoError(t, db.QueryRow(`SELECT zone_id FROM sensors WHERE source='src' AND device='D' AND type='temp'`).Scan(&linkedID))
	assert.True(t, linkedID.Valid)
	assert.EqualValues(t, zoneID, linkedID.Int64)
}

func TestSensorIngest_DisabledByConfig(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 0) // disabled

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	resp := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"source": "src", "device": "D", "type": "temp", "value": 1.0,
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "ingest must reject when disabled in config")
}

func TestSensorIngest_RejectsMissingSource(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	// `source` is required by the binding tag — payload without it must
	// fail JSON binding and surface as 400.
	resp := ingestPostKeepsRateOK(t, c, apiKey, map[string]interface{}{
		"device": "D", "type": "temp", "value": 1.0,
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestSensorIngest_RejectsNonFiniteValue(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	c := server.NewClient(t)
	// json.Encoder cannot serialize NaN/Inf, so we inject the literal
	// strings via a custom encoder. The handler should reject these.
	// Here we use a JSON value that decodes to NaN by handcrafting the
	// body string.
	const body = `{"source":"src","device":"D","type":"temp","value":1e1000}`
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/sensors/ingest", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", apiKey)
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-finite values must be rejected")
}

// ---------------------------------------------------------------------------
// GET /sensorData (ChartHandler)
// ---------------------------------------------------------------------------

func TestChartHandler_RawShortRange(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withPollingInterval(t, 60)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	// Seed a sensor + two recent readings (within last 60 minutes).
	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type) VALUES (1, 'Tent Temp', 1, 'src', 'D', 'temp')`)

	now := time.Now().UTC()
	insertReadingAt(t, db, 1, 21.0, now.Add(-30*time.Minute))
	insertReadingAt(t, db, 1, 22.0, now.Add(-5*time.Minute))

	testutil.SeedAdmin(t, db, "chart-pw")
	c := server.LoginAsAdmin(t, "chart-pw")

	// 60 minutes is below the 24-hour rollup threshold → raw query.
	resp := c.Get("/sensorData?sensor=1&minutes=60")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []types.SensorData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 2)
	for _, r := range got {
		assert.Equal(t, "Tent Temp", r.SensorName)
		assert.NotZero(t, r.ID, "raw rows have a real sensor_data.id")
	}
}

func TestChartHandler_RollupLongRange(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withPollingInterval(t, 60)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.MustExec(t, db, `INSERT INTO zones (id, name) VALUES (1, 'Z')`)
	testutil.MustExec(t, db, `INSERT INTO sensors (id, name, zone_id, source, device, type) VALUES (1, 'Tent Temp', 1, 'src', 'D', 'temp')`)

	// Seed the rollup table directly so the query path is deterministic
	// regardless of the trigger that maintains it.
	bucket := time.Now().UTC().Add(-3 * time.Hour).Format("2006-01-02 15:00:00")
	testutil.MustExec(t, db,
		`INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		 VALUES (1, $1, 18.0, 22.0, 20.0, 4)`,
		bucket,
	)

	testutil.SeedAdmin(t, db, "chart-pw-2")
	c := server.LoginAsAdmin(t, "chart-pw-2")

	// 26 hours > 24-hour rollup threshold → rollup query.
	minutes := strconv.Itoa(26 * 60)
	resp := c.Get("/sensorData?sensor=1&minutes=" + minutes)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []types.SensorData
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.EqualValues(t, 0, got[0].ID, "rollup rows surface with id=0 by convention")
	assert.InDelta(t, 20.0, got[0].Value, 0.0001, "rollup avg_val should be returned")
	assert.Equal(t, "Tent Temp", got[0].SensorName)
}

func TestChartHandler_RejectsMissingSensorParam(t *testing.T) {
	resetRateLimit(t)
	withPollingInterval(t, 60)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, "chart-pw-3")
	c := server.LoginAsAdmin(t, "chart-pw-3")
	resp := c.Get("/sensorData?minutes=10")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestChartHandler_RejectsMissingTimeParams(t *testing.T) {
	resetRateLimit(t)
	withPollingInterval(t, 60)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, "chart-pw-4")
	c := server.LoginAsAdmin(t, "chart-pw-4")
	// neither minutes nor start/end → 400
	resp := c.Get("/sensorData?sensor=1")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Concurrent ingest doesn't corrupt the sensor row (smoke check that
// the duplicate-key path is exercised under contention).
// ---------------------------------------------------------------------------

func TestSensorIngest_ConcurrentSameSensor(t *testing.T) {
	resetRateLimit(t)
	resetIngestRateLimiter(t)
	withAPIIngestEnabled(t, 1)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	apiKey := seedSensorIngestKey(t, db)

	const writers = 4
	const perWriter = 3
	var ok atomic.Int32

	type ingest struct {
		Source string  `json:"source"`
		Device string  `json:"device"`
		Type   string  `json:"type"`
		Value  float64 `json:"value"`
	}
	body := ingest{Source: "concurrent", Device: "D", Type: "temp", Value: 42.0}

	done := make(chan struct{})
	for w := 0; w < writers; w++ {
		go func() {
			c := server.NewClient(t)
			for i := 0; i < perWriter; i++ {
				resp := ingestPostKeepsRateOK(t, c, apiKey, body)
				if resp.StatusCode == http.StatusOK {
					ok.Add(1)
				}
				resp.Body.Close()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < writers; i++ {
		<-done
	}

	assert.GreaterOrEqual(t, int(ok.Load()), 1, "at least one ingest should succeed")

	// Even under contention, only one sensor row exists for the tuple.
	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM sensors WHERE source='concurrent' AND device='D' AND type='temp'`,
	).Scan(&n))
	assert.Equal(t, 1, n, "concurrent ingests must NOT duplicate the sensor row")
}

// ---------------------------------------------------------------------------
// Helpers shared with this file
// ---------------------------------------------------------------------------

// insertReadingAt inserts a sensor_data row and forces create_dt to a
// specific timestamp. SQLite stores datetimes as strings in
// LayoutDB-shaped form.
func insertReadingAt(t *testing.T, db *sql.DB, sensorID int, value float64, ts time.Time) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)`, sensorID, value)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	_, err = db.Exec(
		`UPDATE sensor_data SET create_dt = $1 WHERE id = $2`,
		ts.Format("2006-01-02 15:04:05"), id,
	)
	require.NoError(t, err)
}
