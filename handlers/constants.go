package handlers

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Shared HTTP clients — reusing clients enables TCP connection pooling and
// avoids the overhead of a fresh TLS handshake on every request.
// ---------------------------------------------------------------------------

var (
	// httpClient is a general-purpose client for outbound API requests
	// (AC Infinity, EcoWitt, etc.) with a sensible default timeout.
	httpClient = &http.Client{Timeout: HTTPTimeoutDefault}

	// httpClientShort is a client with a shorter timeout for quick probes
	// like EcoWitt device scanning where slow responses likely mean failure.
	httpClientShort = &http.Client{Timeout: HTTPTimeoutShort}
)

// ---------------------------------------------------------------------------
// Sensor type IDs — AC Infinity sensor classification
// ---------------------------------------------------------------------------

const (
	// SensorTypeTemperature is the primary temperature sensor (type 1).
	SensorTypeTemperature = 1
	// SensorTypeHumidity is the primary humidity sensor (type 2).
	SensorTypeHumidity = 2
	// SensorTypeTempProbe is an external temperature probe (type 4).
	SensorTypeTempProbe = 4
	// SensorTypeTempController is a temperature controller reading (type 5).
	SensorTypeTempController = 5
	// SensorTypeHumidityExt is an external humidity sensor (type 6).
	SensorTypeHumidityExt = 6
	// SensorTypeVPD is the VPD sensor (type 7).
	SensorTypeVPD = 7
)

// isTemperatureSensor returns true for sensor types that report temperature.
func isTemperatureSensor(sensorType int) bool {
	return sensorType == SensorTypeTemperature ||
		sensorType == SensorTypeTempProbe ||
		sensorType == SensorTypeTempController
}

// isHumiditySensor returns true for sensor types that report percentage values.
func isHumiditySensor(sensorType int) bool {
	return sensorType == SensorTypeHumidity || sensorType == SensorTypeHumidityExt
}

// ---------------------------------------------------------------------------
// Activity type IDs — built-in activity identifiers
// ---------------------------------------------------------------------------

const (
	// ActivityWater is the built-in "Water" activity.
	ActivityWater = 1
	// ActivityFeed is the built-in "Feed" activity.
	ActivityFeed = 2
)

// ---------------------------------------------------------------------------
// HTTP client timeouts
// ---------------------------------------------------------------------------

const (
	// HTTPTimeoutDefault is the default timeout for outbound API requests.
	HTTPTimeoutDefault = 10 * time.Second
	// HTTPTimeoutShort is used for quick probes like EcoWitt device scanning.
	HTTPTimeoutShort = 5 * time.Second
	// HTTPTimeoutLong is used for image/stream downloads that may be slow.
	HTTPTimeoutLong = 30 * time.Second
	// HTTPTimeoutMedium is used for HLS thumbnail probes.
	HTTPTimeoutMedium = 15 * time.Second
)

// ---------------------------------------------------------------------------
// Watcher intervals
// ---------------------------------------------------------------------------

const (
	// PruneInterval is how often the sensor data pruner runs.
	PruneInterval = 24 * time.Hour
	// RollupInterval is how often hourly rollups are refreshed.
	RollupInterval = 10 * time.Minute
	// DefaultStreamGrabInterval is the fallback interval (in seconds)
	// between stream image captures when no config value is set.
	DefaultStreamGrabInterval = 60
	// defaultPollingIntervalSeconds is the fallback polling-interval value
	// the cache TTL helpers fall back to when no Store is supplied.
	// Mirrors config.NewStore's default; a duplicated literal is cheaper
	// than a circular import.
	defaultPollingIntervalSeconds = 60
)

// ---------------------------------------------------------------------------
// Data query thresholds
// ---------------------------------------------------------------------------

const (
	// RollupThresholdMinutes is the number of minutes beyond which chart
	// queries switch from raw sensor_data to the hourly rollup table.
	RollupThresholdMinutes = 60 * 24 // 24 hours
	// RollupThresholdHours is the hour equivalent used in date-range queries.
	RollupThresholdHours = 24
	// MaxRawDataRows caps the number of rows returned for raw sensor queries.
	MaxRawDataRows = 10000
)

// ---------------------------------------------------------------------------
// Pagination and history limits
// ---------------------------------------------------------------------------

const (
	// PlantHistoryLimit caps the number of rows fetched for measurements,
	// activities, and status log history on the plant detail page.
	PlantHistoryLimit = 500
	// DefaultPlantImageOrder is the sort order assigned to a placeholder
	// image when no explicit order is set.
	DefaultPlantImageOrder = 100
	// DefaultLogLines is the number of log lines returned by the log viewer.
	DefaultLogLines = 200
	// MinLogLines is the minimum allowed log lines.
	MinLogLines = 1
	// MaxLogLines is the maximum allowed log lines.
	MaxLogLines = 2000
)

// ---------------------------------------------------------------------------
// Security and validation
// ---------------------------------------------------------------------------

const (
	// MaxLoginAttempts is the number of failed login attempts within the
	// rate-limit window before further attempts are blocked.
	MaxLoginAttempts = 5
	// MinPasswordLength is the minimum acceptable password length.
	MinPasswordLength = 8
	// MaxACIPasswordLength is the max password length accepted by the
	// AC Infinity API.
	MaxACIPasswordLength = 25
	// MaxAncestryDepth caps the recursion depth when building strain
	// ancestry trees to prevent infinite loops from circular lineage data.
	MaxAncestryDepth = 10
	// ACISuccessCode is the HTTP-level success code returned by the
	// AC Infinity API (inside the JSON response body, not the HTTP status).
	ACISuccessCode = 200
)

// ---------------------------------------------------------------------------
// Upload and file limits
// ---------------------------------------------------------------------------

const (
	// MaxMultipartFormSize is the size limit for multipart form uploads (10 MB).
	MaxMultipartFormSize = 10 << 20
	// MinBackupSizeMB is the minimum allowed value for the max backup size setting.
	MinBackupSizeMB = 100
	// DefaultStreamGrabIntervalMs is the default stream grab interval in
	// milliseconds, used when the stored setting is unparseable.
	DefaultStreamGrabIntervalMs = 3000
)
