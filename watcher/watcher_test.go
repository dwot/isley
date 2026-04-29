package watcher

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/logger"
	"isley/tests/testutil"
	"isley/tests/testutil/fakes"
)

// stubHTTPDoer is a synctest-friendly HTTPDoer. The watcher's Run-loop
// tests live inside a synctest bubble; a real httptest server (or
// http.Transport speaking to one) spawns connection read/write loops
// that block on network syscalls, which synctest considers
// "non-durably blocked". Those loops would prevent the bubble from
// ever observing all goroutines as durably blocked, so synctest.Wait
// would hang. The stub returns a canned body inline with no goroutines
// or I/O, keeping the bubble deterministic.
type stubHTTPDoer struct {
	hits int32
	body []byte
}

func (s *stubHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&s.hits, 1)
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(s.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

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

// seedSensor delegates to testutil.SeedSensor; kept as a named alias
// because the watcher test file refers to it from many call sites.
func seedSensor(t *testing.T, db *sql.DB, source, device, kind string) int {
	t.Helper()
	return testutil.SeedSensor(t, db, source, device, kind)
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
	t.Parallel()

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
	t.Parallel()

	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "test", "dev1", "temp")

	w := newTestWatcher(t, db)
	w.addSensorData("test", "dev1", "temp", "not-a-number")

	assert.Zero(t, countSensorData(t, db, id), "non-numeric values must not be persisted")
}

func TestAddSensorData_SkipsUnknownSensor(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	// Intentionally do NOT seed any sensor row.

	w := newTestWatcher(t, db)
	w.addSensorData("test", "ghost", "temp", "10")

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sensor_data`).Scan(&n))
	assert.Zero(t, n, "unknown sensors must not produce a row")
}

// ---------------------------------------------------------------------------
// PollACI — fixture lives at tests/fixtures/aci/happy.json
// ---------------------------------------------------------------------------

func TestPollACI_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	tempC := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")
	humidity := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.humidity")
	customSensor := seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.0.1")
	port := seedSensor(t, db, "acinfinity", "TESTDEV", "ACIP.0")

	// Wrap fakes.FakeACI to capture the token header and query string —
	// FakeACI alone serves the body, but this test also asserts on the
	// request shape, so it inspects the header/query inline.
	body := testutil.MustReadFixture(t, "aci/happy.json")
	var hits int32
	var capturedToken, capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		capturedToken = r.Header.Get("token")
		capturedQuery = r.URL.RawQuery
		_, _ = w.Write(body)
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
	t.Parallel()

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
// PollEcoWitt — fixture lives at tests/fixtures/ecowitt/happy.json
// ---------------------------------------------------------------------------

func TestPollEcoWitt_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	// fakes.FakeEcoWitt enforces the /get_livedata_info path so a
	// regression that changes the request URL surfaces as a missing-row
	// failure below rather than passing silently.
	server := fakes.FakeEcoWitt(t, "happy")

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
	t.Parallel()

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
	t.Parallel()

	db := testutil.NewTestDB(t)
	id := seedSensor(t, db, "x", "y", "z")
	insertReadingAt(t, db, id, 1.0, time.Now().AddDate(0, 0, -100))

	w := newTestWatcher(t, db)
	// Default retention is 0 → pruning disabled.
	require.NoError(t, w.PruneSensorData())

	assert.Equal(t, 1, countSensorData(t, db, id), "row must survive when retention is disabled")
}

func TestPruneSensorData_DeletesOnlyOldRows(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	db := testutil.NewTestDB(t)

	synctest.Test(t, func(t *testing.T) {
		w := newTestWatcher(t, db)
		// Long polling interval; the cancel below is what should unblock Run.
		// Under synctest the "hour" never elapses in real time — only ctx
		// cancellation can wake the loop.
		w.PollingInterval = func() time.Duration { return time.Hour }

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			w.Run(ctx)
			close(done)
		}()

		// Park the loop on its select<-ctx.Done()/time.After(1h).
		synctest.Wait()

		cancel()

		// After cancel, the loop's ctx.Done case fires and Run returns.
		<-done
	})
}

func TestRun_RespectsRestoreInProgress(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	body := testutil.MustReadFixture(t, "aci/happy.json")

	synctest.Test(t, func(t *testing.T) {
		h := &stubHTTPDoer{body: body}

		w := newTestWatcher(t, db)
		w.HTTP = h
		w.ACIBaseURL = "http://aci.example"
		w.ACIEnabled = func() bool { return true }
		w.ACIToken = func() string { return "tok" }
		w.RestoreInProgress = func() bool { return true }
		w.PollingInterval = func() time.Duration { return time.Millisecond }

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			w.Run(ctx)
			close(done)
		}()

		// Advance virtual time well past dozens of polling intervals; if
		// RestoreInProgress weren't honored, many HTTP calls would fire
		// during this window.
		time.Sleep(100 * time.Millisecond)

		cancel()
		<-done

		assert.EqualValues(t, 0, atomic.LoadInt32(&h.hits), "no HTTP calls should fire while restore is in progress")
	})
}

func TestRun_PollsACIWhenEnabled(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	seedSensor(t, db, "acinfinity", "TESTDEV", "ACI.tempC")
	body := testutil.MustReadFixture(t, "aci/happy.json")

	synctest.Test(t, func(t *testing.T) {
		h := &stubHTTPDoer{body: body}

		w := newTestWatcher(t, db)
		w.HTTP = h
		w.ACIBaseURL = "http://aci.example"
		w.ACIEnabled = func() bool { return true }
		w.ACIToken = func() string { return "tok" }
		w.PollingInterval = func() time.Duration { return time.Millisecond }

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			w.Run(ctx)
			close(done)
		}()

		// Wait for the first iteration: PollACI fires once, the loop falls
		// through the prune/rollup defaults, then parks on time.After.
		synctest.Wait()

		assert.GreaterOrEqual(t, atomic.LoadInt32(&h.hits), int32(1), "at least one ACI request expected")

		cancel()
		<-done
	})
}
