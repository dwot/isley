package watcher

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/logger"
	"isley/tests/testutil"
)

// newTestWatcher returns a Watcher pre-wired for tests. Every getter
// returns a sensible default that disables the production code paths;
// individual tests overwrite the fields they care about. This keeps
// each test focused on one behavior rather than re-stating the full
// dependency graph.
func newTestWatcher(t *testing.T, db *sql.DB) *Watcher {
	t.Helper()
	return &Watcher{
		DB:                db,
		HTTP:              http.DefaultClient,
		ACIBaseURL:        "http://localhost-not-used",
		Logger:            logger.Log,
		Now:               time.Now,
		PollingInterval:   func() time.Duration { return 10 * time.Millisecond },
		RestoreInProgress: func() bool { return false },
		ACIEnabled:        func() bool { return false },
		ACIToken:          func() string { return "" },
		ECEnabled:         func() bool { return false },
		ECDevices:         func() []string { return nil },
		SensorRetention:   func() int { return 0 },
	}
}

// seedSensor registers a sensor in the sensors table so addSensorData
// will recognize it. Returns the new id.
func seedSensor(t *testing.T, db *sql.DB, source, device, kind string) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO sensors (name, source, device, type) VALUES ($1, $2, $3, $4)`,
		device+":"+kind, source, device, kind,
	)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

func countSensorData(t *testing.T, db *sql.DB, sensorID int) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data WHERE sensor_id = $1`, sensorID).Scan(&n))
	return n
}

// ---------------------------------------------------------------------------
// addSensorData
// ---------------------------------------------------------------------------

func TestAddSensorData_WritesRow(t *testing.T) {
	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "test", "dev1", "temp")

	w := newTestWatcher(t, db)
	w.addSensorData("test", "dev1", "temp", "23.5")

	require.Equal(t, 1, countSensorData(t, db, id))

	var v float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, id).Scan(&v))
	assert.InDelta(t, 23.5, v, 0.0001)
}

func TestAddSensorData_RejectsNonNumeric(t *testing.T) {
	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "test", "dev1", "temp")

	w := newTestWatcher(t, db)
	w.addSensorData("test", "dev1", "temp", "not-a-number")

	assert.Zero(t, countSensorData(t, db, id), "non-numeric values must not be persisted")
}

func TestAddSensorData_SkipsUnknownSensor(t *testing.T) {
	db := testutil.NewTestDB(t)
	// Intentionally do NOT seed any sensor row.

	w := newTestWatcher(t, db)
	w.addSensorData("test", "ghost", "temp", "10")

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data`).Scan(&n))
	assert.Zero(t, n, "unknown sensors must not produce a row")
}

// ---------------------------------------------------------------------------
// PollACI
// ---------------------------------------------------------------------------

const aciCannedJSON = `{
  "data": [
    {
      "devCode": "TESTDEV",
      "deviceInfo": {
        "temperatureF": 7500,
        "temperature": 2400,
        "humidity": 5500,
        "sensors": [
          {"sensorType": 1, "accessPort": 0, "sensorData": 1234}
        ],
        "ports": [
          {"port": 0, "speak": 5}
        ]
      }
    }
  ]
}`

func TestPollACI_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)
	tempC := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")
	humidity := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.humidity")
	customSensor := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.0.1")
	port := seedSensor(t, db, "acinfinity", "TESTDEV", "ACIP.0")

	var hits int32
	var capturedToken, capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		capturedToken = r.Header.Get("token")
		capturedQuery = r.URL.RawQuery
		_, _ = fmt.Fprint(w, aciCannedJSON)
	}))
	t.Cleanup(server.Close)

	w := newTestWatcher(t, db)
	w.ACIBaseURL = server.URL
	w.PollACI(context.Background(), "test-token")

	assert.EqualValues(t, 1, atomic.LoadInt32(&hits), "exactly one HTTP request expected")
	assert.Equal(t, "test-token", capturedToken, "token header should be the supplied token")
	assert.Equal(t, "userId=test-token", capturedQuery, "query string should carry userId")

	// All four pre-seeded sensors should now have one row each.
	for _, id := range []int{tempC, humidity, customSensor, port} {
		assert.Equalf(t, 1, countSensorData(t, db, id), "sensor %d should have one row", id)
	}

	// Spot-check the temperature value: temperature field 2400 / 100 = 24.0.
	var v float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, tempC).Scan(&v))
	assert.InDelta(t, 24.0, v, 0.0001)
}

func TestPollACI_NetworkErrorIsSwallowed(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	w := newTestWatcher(t, db)
	w.ACIBaseURL = server.URL

	// The current implementation swallows network errors and 500s so a
	// flaky upstream doesn't crash the watcher loop. We assert that no
	// rows are written and the call returns normally.
	w.PollACI(context.Background(), "test-token")

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data`).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// PollEcoWitt
// ---------------------------------------------------------------------------

const ecowittCannedJSON = `{
  "wh25": [{"intemp": "21.4", "unit": "C", "inhumi": "55%", "abs": "1010", "rel": "1010"}],
  "ch_soil": [{"channel": "1", "name": "Bed1", "battery": "5", "humidity": "47%"}]
}`

func TestPollEcoWitt_HappyPath(t *testing.T) {
	db := testutil.NewTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/get_livedata_info", r.URL.Path)
		_, _ = fmt.Fprint(w, ecowittCannedJSON)
	}))
	t.Cleanup(server.Close)

	u, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Sensors are keyed by (source, device, type) where device is the
	// host[:port] string passed to PollEcoWitt.
	inTempID := seedSensor(t, db, "ecowitt", u.Host, "WH25.InTemp")
	inHumiID := seedSensor(t, db, "ecowitt", u.Host, "WH25.InHumi")
	soil1ID := seedSensor(t, db, "ecowitt", u.Host, "Soil.1")

	w := newTestWatcher(t, db)
	w.PollEcoWitt(context.Background(), u.Host)

	for _, id := range []int{inTempID, inHumiID, soil1ID} {
		assert.Equalf(t, 1, countSensorData(t, db, id), "sensor %d should have one row", id)
	}

	// Percent suffix on humidity should be trimmed before insertion.
	var humi, soil float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, inHumiID).Scan(&humi))
	assert.InDelta(t, 55.0, humi, 0.0001, "humidity value should be 55, not 55%%")
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, soil1ID).Scan(&soil))
	assert.InDelta(t, 47.0, soil, 0.0001)
}

func TestTrimTrailingPercent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"55%", "55"},
		{"100.0%", "100.0"},
		{"42", "42"},    // no suffix → unchanged
		{"", ""},        // empty input survives
		{"%", ""},       // single percent stripped
		{"7%5%", "7%5"}, // only trailing % is stripped
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, trimTrailingPercent(tc.in), "trimTrailingPercent(%q)", tc.in)
	}
}

// ---------------------------------------------------------------------------
// PruneSensorData
// ---------------------------------------------------------------------------

func TestPruneSensorData_RetentionDisabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "x", "y", "z")
	insertReadingAt(t, db, id, 1.0, time.Now().AddDate(0, 0, -100))

	w := newTestWatcher(t, db)
	// Default retention is 0 → pruning disabled.
	require.NoError(t, w.PruneSensorData())

	assert.Equal(t, 1, countSensorData(t, db, id), "row must survive when retention is disabled")
}

func TestPruneSensorData_DeletesOnlyOldRows(t *testing.T) {
	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "x", "y", "z")

	// Two rows: one well outside the window, one inside.
	insertReadingAt(t, db, id, 1.0, time.Now().AddDate(0, 0, -60)) // outside
	insertReadingAt(t, db, id, 2.0, time.Now().Add(-1*time.Hour))  // inside

	w := newTestWatcher(t, db)
	w.SensorRetention = func() int { return 30 }

	require.NoError(t, w.PruneSensorData())

	assert.Equal(t, 1, countSensorData(t, db, id), "only the inside-window row should remain")

	var keptValue float64
	require.NoError(t, db.QueryRow(`SELECT value FROM sensor_data WHERE sensor_id = $1`, id).Scan(&keptValue))
	assert.InDelta(t, 2.0, keptValue, 0.0001)
}

// insertReadingAt is the prune-test helper: it inserts a sensor_data
// row and then forces create_dt to a specific timestamp so the SQLite
// trigger doesn't reset it back to CURRENT_TIMESTAMP. SQLite stores
// datetimes as strings, so we use the same `YYYY-MM-DD HH:MM:SS`
// format the production code's prune query expects.
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

// ---------------------------------------------------------------------------
// Run loop
// ---------------------------------------------------------------------------

func TestRun_StopsOnContextCancel(t *testing.T) {
	db := testutil.NewTestDB(t)

	w := newTestWatcher(t, db)
	// Long polling interval; the cancel below is what should unblock Run.
	w.PollingInterval = func() time.Duration { return time.Hour }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after ctx cancel")
	}
}

func TestRun_RespectsRestoreInProgress(t *testing.T) {
	db := testutil.NewTestDB(t)

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = fmt.Fprint(w, aciCannedJSON)
	}))
	t.Cleanup(server.Close)

	w := newTestWatcher(t, db)
	w.ACIBaseURL = server.URL
	w.ACIEnabled = func() bool { return true }
	w.ACIToken = func() string { return "tok" }
	w.RestoreInProgress = func() bool { return true }
	w.PollingInterval = func() time.Duration { return time.Millisecond }

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	assert.EqualValues(t, 0, atomic.LoadInt32(&hits), "no HTTP calls should fire while restore is in progress")
}

func TestRun_PollsACIWhenEnabled(t *testing.T) {
	db := testutil.NewTestDB(t)
	seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = fmt.Fprint(w, aciCannedJSON)
	}))
	t.Cleanup(server.Close)

	w := newTestWatcher(t, db)
	w.ACIBaseURL = server.URL
	w.ACIEnabled = func() bool { return true }
	w.ACIToken = func() string { return "tok" }
	w.PollingInterval = func() time.Duration { return time.Millisecond }

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	assert.GreaterOrEqual(t, atomic.LoadInt32(&hits), int32(1), "at least one ACI request expected during the run window")
}
