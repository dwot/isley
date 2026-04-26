// Package watcher polls the AC Infinity and EcoWitt cloud APIs at a
// configurable interval, persists readings into sensor_data, prunes
// rows past their retention window, and refreshes the hourly rollup
// table.
//
// The polling loop is dependency-injected via the Watcher struct so
// tests can substitute the HTTP client, the clock, and the config
// getters without touching package globals. See docs/TEST_PLAN.md
// Phase 2 for the rationale.
package watcher

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
)

const (
	// httpClientTimeout is the default timeout for outbound sensor API requests.
	httpClientTimeout = 10 * time.Second
	// pruneInterval is how often the sensor data pruner runs inside Run.
	pruneInterval = 24 * time.Hour
	// rollupInterval is how often hourly rollups are refreshed inside Run.
	rollupInterval = 10 * time.Minute

	// defaultACIBaseURL is the production AC Infinity API host. Tests
	// override this on the Watcher struct to point at httptest fakes.
	defaultACIBaseURL = "https://www.acinfinityserver.com"
)

// HTTPDoer is the minimum surface from net/http needed by the watcher.
// *http.Client implements it directly. Tests can pass a stub that
// records request URLs and returns canned responses without standing
// up an httptest server, though most tests prefer httptest because the
// real client + fake server combination matches production behavior.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Watcher owns the dependencies the polling loop needs. Construct it
// once at process startup via New, or assemble one by hand in tests.
//
// All getter functions (PollingInterval, RestoreInProgress, etc.) are
// invoked on every iteration so a Watcher built from the config
// package globals stays current with runtime settings changes.
type Watcher struct {
	DB         *sql.DB
	HTTP       HTTPDoer
	ACIBaseURL string
	Logger     *logrus.Logger

	Now               func() time.Time
	PollingInterval   func() time.Duration
	RestoreInProgress func() bool
	ACIEnabled        func() bool
	ACIToken          func() string
	ECEnabled         func() bool
	ECDevices         func() []string
	SensorRetention   func() int
}

// New returns a Watcher wired to the production config and a default
// HTTP client. Production code (main.go) calls this; tests typically
// construct a Watcher literal so each field can be controlled.
func New(db *sql.DB) *Watcher {
	return &Watcher{
		DB:         db,
		HTTP:       &http.Client{Timeout: httpClientTimeout},
		ACIBaseURL: defaultACIBaseURL,
		Logger:     logger.Log,

		Now:               time.Now,
		PollingInterval:   func() time.Duration { return time.Duration(config.PollingInterval) * time.Second },
		RestoreInProgress: config.RestoreInProgress.Load,
		ACIEnabled:        func() bool { return config.ACIEnabled == 1 },
		ACIToken:          func() string { return config.ACIToken },
		ECEnabled:         func() bool { return config.ECEnabled == 1 },
		ECDevices:         func() []string { return config.ECDevices },
		SensorRetention:   func() int { return config.SensorRetention },
	}
}

// Run is the main polling loop. It blocks until ctx is cancelled and
// returns after the in-flight iteration completes — callers should
// run it in its own goroutine and use a sync.WaitGroup or similar to
// wait for shutdown.
//
// Each iteration:
//  1. If a backup restore is in progress, do nothing this cycle.
//  2. Otherwise poll AC Infinity and EcoWitt according to enabled flags.
//  3. Run prune and rollup if their tickers have fired.
//  4. Sleep for PollingInterval, or return early if ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	w.Logger.Info("Started Sensor Watcher")

	pruneTicker := time.NewTicker(pruneInterval)
	defer pruneTicker.Stop()

	rollupTicker := time.NewTicker(rollupInterval)
	defer rollupTicker.Stop()

	// Run an initial rollup at startup to backfill if needed.
	if err := w.RefreshHourlyRollups(); err != nil {
		w.Logger.WithError(err).Error("Initial hourly rollup failed")
	}

	for {
		if w.RestoreInProgress() {
			w.Logger.Debug("Backup restore in progress, skipping sensor poll")
		} else {
			if w.ACIEnabled() {
				if token := w.ACIToken(); token != "" {
					w.PollACI(ctx, token)
				}
			}
			if w.ECEnabled() {
				for _, ecServer := range w.ECDevices() {
					w.PollEcoWitt(ctx, ecServer)
				}
			}

			select {
			case <-pruneTicker.C:
				if err := w.PruneSensorData(); err != nil {
					w.Logger.WithError(err).Error("Scheduled sensor data prune failed")
				} else {
					w.Logger.Info("Scheduled sensor data prune completed")
				}
			default:
			}

			select {
			case <-rollupTicker.C:
				if err := w.RefreshHourlyRollups(); err != nil {
					w.Logger.WithError(err).Error("Scheduled hourly rollup failed")
				}
			default:
			}
		}

		// Wait for either the polling interval or context cancellation.
		select {
		case <-ctx.Done():
			w.Logger.Info("Sensor Watcher shutting down")
			return
		case <-time.After(w.PollingInterval()):
		}
	}
}

// PollEcoWitt fetches livedata from a single EcoWitt server and writes
// matching sensor rows. server is the host[:port] portion as it
// appears in config.ECDevices.
func (w *Watcher) PollEcoWitt(ctx context.Context, server string) {
	w.Logger.WithField("timestamp", w.Now()).Info("Updating EC sensor data")

	url := "http://" + server + "/get_livedata_info"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		w.Logger.WithError(err).Error("Error creating EcoWitt request")
		return
	}

	resp, err := w.HTTP.Do(req)
	if err != nil {
		w.Logger.WithError(err).Error("Error sending EcoWitt request")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Logger.WithError(err).Error("Error reading EcoWitt response body")
		return
	}

	var apiResponse types.ECWAPIResponse
	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		w.Logger.WithError(err).Error("Error parsing EcoWitt JSON response")
		return
	}

	device := server
	source := "ecowitt"
	dataMap := map[string]string{}

	for _, wh := range apiResponse.WH25 {
		dataMap["WH25.InTemp"] = wh.InTemp
		dataMap["WH25.InHumi"] = trimTrailingPercent(wh.InHumi)
	}

	for _, ch := range apiResponse.CHSoil {
		dataMap["Soil."+ch.Channel] = trimTrailingPercent(ch.Humidity)
	}

	for key, value := range dataMap {
		w.addSensorData(source, device, key, value)
	}
}

// PollACI fetches the device list from AC Infinity using the supplied
// access token and writes matching sensor rows.
func (w *Watcher) PollACI(ctx context.Context, token string) {
	w.Logger.WithField("timestamp", w.Now()).Info("Updating ACI sensor data")

	url := w.ACIBaseURL + "/api/user/devInfoListAll?userId=" + token
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		w.Logger.WithError(err).Error("Error creating ACI request")
		return
	}

	req.Header.Add("token", token)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	resp, err := w.HTTP.Do(req)
	if err != nil {
		w.Logger.WithError(err).Error("Error sending ACI request")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Logger.WithError(err).Error("Error reading ACI response body")
		return
	}

	var jsonResponse types.ACIResponse
	if err := json.Unmarshal(respBody, &jsonResponse); err != nil {
		w.Logger.WithError(err).Error("Error unmarshalling ACI JSON response")
		return
	}

	source := "acinfinity"
	for _, deviceData := range jsonResponse.Data {
		dataMap := map[string]float64{}
		device := deviceData.DevCode

		dataMap["ACI.tempF"] = float64(deviceData.DeviceInfo.TemperatureF) / 100.0
		dataMap["ACI.tempC"] = float64(deviceData.DeviceInfo.Temperature) / 100.0
		dataMap["ACI.humidity"] = float64(deviceData.DeviceInfo.Humidity) / 100.0

		for _, sensor := range deviceData.DeviceInfo.Sensors {
			dataMap[fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)] = float64(sensor.SensorData) / 100.0
		}

		for _, port := range deviceData.DeviceInfo.Ports {
			dataMap[fmt.Sprintf("ACIP.%d", port.Port)] = float64(port.Speak) * 10
		}

		for key, value := range dataMap {
			w.addSensorData(source, device, key, fmt.Sprintf("%f", value))
		}
	}
}

// addSensorData writes a single (source, device, key, value) tuple to
// sensor_data. Sensors that are not pre-registered in the sensors table
// are silently skipped — that's how the UI lets users opt in to
// tracking specific sensors. Non-numeric values are also silently
// skipped after logging.
func (w *Watcher) addSensorData(source string, device string, key string, value string) {
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		w.Logger.WithFields(logrus.Fields{
			"source": source,
			"device": device,
			"type":   key,
			"value":  value,
			"error":  err,
		}).Error("Error parsing sensor value")
		return
	}

	var sensorID int
	err := w.DB.QueryRow("SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3", source, device, key).Scan(&sensorID)
	if err != nil {
		w.Logger.WithFields(logrus.Fields{
			"source": source,
			"device": device,
			"type":   key,
		}).Debug("Sensor not tracked, skipping")
		return
	}

	if _, err := w.DB.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)", sensorID, value); err != nil {
		w.Logger.WithFields(logrus.Fields{
			"sensorID": sensorID,
			"value":    value,
			"error":    err,
		}).Error("Error writing sensor data to database")
	}
}

// PruneSensorData deletes sensor_data rows older than the configured
// retention window. With retention <= 0 pruning is disabled and the
// method returns nil. SQLite databases additionally run VACUUM,
// ANALYZE, and PRAGMA optimize after the delete so the file shrinks.
func (w *Watcher) PruneSensorData() error {
	w.Logger.Info("Pruning old sensor data")
	days := w.SensorRetention()
	if days <= 0 {
		w.Logger.Info("Sensor data pruning is disabled (sensor_retention_days = 0)")
		return nil
	}

	var pruneQuery string
	if model.IsPostgres() {
		pruneQuery = fmt.Sprintf("DELETE FROM sensor_data WHERE create_dt < NOW() - INTERVAL '%d days'", days)
	} else {
		pruneQuery = fmt.Sprintf("DELETE FROM sensor_data WHERE create_dt < datetime('now', 'localtime', '-%d days')", days)
	}
	if _, err := w.DB.Exec(pruneQuery); err != nil {
		w.Logger.WithError(err).Error("Error pruning sensor data")
		return err
	}
	// rolling_averages is not pruned — it's a trigger-maintained cache with only
	// one row per sensor, so it stays small and self-maintaining.

	if model.IsSQLite() {
		if _, err := w.DB.Exec("VACUUM"); err != nil {
			w.Logger.WithError(err).Warn("SQLite VACUUM failed")
		}
		if _, err := w.DB.Exec("ANALYZE"); err != nil {
			w.Logger.WithError(err).Warn("SQLite ANALYZE failed")
		}
		if _, err := w.DB.Exec("PRAGMA optimize"); err != nil {
			w.Logger.WithError(err).Warn("SQLite PRAGMA optimize failed")
		}
		w.Logger.Info("SQLite post-prune maintenance completed")
	}

	w.Logger.WithField("days", days).Info("Sensor data pruned")
	return nil
}

// trimTrailingPercent strips a single trailing '%' from a value like
// "55%" → "55". The EcoWitt firmware reports humidity values with the
// percent sign included; the historical implementation sliced the
// last byte unconditionally, which would corrupt a future firmware
// that ever drops the suffix. This safer variant matches behavior
// when the suffix is present and is a no-op when it isn't.
func trimTrailingPercent(v string) string {
	if n := len(v); n > 0 && v[n-1] == '%' {
		return v[:n-1]
	}
	return v
}
