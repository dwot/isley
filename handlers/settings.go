package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"isley/config"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func GenerateAPIKey() string {
	// Generate a random 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// HashAPIKey returns a bcrypt hash of the given plaintext API key.
// bcrypt is preferred over fast hashes like SHA-256 for secret storage
// because its adaptive cost factor resists brute-force attacks.
func HashAPIKey(plaintext string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to hash API key")
		return ""
	}
	return string(hash)
}

// CheckAPIKey compares a plaintext API key against a stored hash.
// It supports bcrypt hashes (preferred), legacy SHA-256 hex digests,
// and plaintext matches for backward compatibility.
// Returns (match bool, legacy bool) — legacy is true when the key matched
// via SHA-256 or plaintext so the caller can auto-upgrade to bcrypt.
func CheckAPIKey(plaintext, stored string) (match bool, legacy bool) {
	// Try bcrypt first (hashes start with "$2a$" or "$2b$")
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plaintext)) == nil, false
	}
	// Fall back to SHA-256 hex comparison for legacy hashes (64 hex chars)
	if len(stored) == 64 {
		h := sha256.Sum256([]byte(plaintext))
		incomingHex := hex.EncodeToString(h[:])
		if subtle.ConstantTimeCompare([]byte(incomingHex), []byte(stored)) == 1 {
			return true, true
		}
		return false, false
	}
	// Fall back to direct comparison for very old plaintext keys
	if subtle.ConstantTimeCompare([]byte(plaintext), []byte(stored)) == 1 {
		return true, true
	}
	return false, false
}

func SaveSettings(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveSettings")
	var settings types.Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiBadRequest(c, "api_invalid_settings_payload")
		return
	}

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	// Generate new API key if requested
	if settings.APIKey == "generate" {
		plaintextKey := GenerateAPIKey()
		hashedKey := HashAPIKey(plaintextKey)

		// Store the hashed key in the database
		err := UpdateSetting(db, store, "api_key", hashedKey)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save API key")
			apiInternalError(c, "api_failed_to_save_api_key")
			return
		}
		store.SetAPIKey(hashedKey)

		// Return the plaintext key only once — it cannot be retrieved again
		c.JSON(http.StatusOK, gin.H{
			"message": T(c, "api_api_key_generated"),
			"api_key": plaintextKey,
		})
		return
	}

	// saveBool persists a boolean setting as "1"/"0" and pushes the
	// matching value into the Store via the supplied setter. Replaces
	// the prev/cleanup pattern that mutated package globals directly.
	saveBool := func(key string, val bool, set func(int)) error {
		dbVal := "0"
		cfgVal := 0
		if val {
			dbVal = "1"
			cfgVal = 1
		}
		if err := UpdateSetting(db, store, key, dbVal); err != nil {
			return err
		}
		set(cfgVal)
		return nil
	}

	boolSettings := []struct {
		key string
		val bool
		set func(int)
	}{
		{"aci.enabled", settings.ACI.Enabled, store.SetACIEnabled},
		{"ec.enabled", settings.EC.Enabled, store.SetECEnabled},
		{"guest_mode", settings.GuestMode, store.SetGuestMode},
		{"stream_grab_enabled", settings.StreamGrabEnabled, store.SetStreamGrabEnabled},
		{"api_ingest_enabled", !settings.DisableAPIIngest, store.SetAPIIngestEnabled},
	}

	for _, bs := range boolSettings {
		if err := saveBool(bs.key, bs.val, bs.set); err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			apiInternalError(c, "api_failed_to_save_settings")
			return
		}
	}

	err := UpdateSetting(db, store, "polling_interval", settings.PollingInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	if v, convErr := strconv.Atoi(settings.PollingInterval); convErr == nil {
		store.SetPollingInterval(v)
	}

	err = UpdateSetting(db, store, "stream_grab_interval", settings.StreamGrabInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	if v, convErr := strconv.Atoi(settings.StreamGrabInterval); convErr == nil {
		store.SetStreamGrabInterval(v)
	}

	// API key is managed separately via the "generate" flow.
	// Only update if a non-empty value is explicitly provided (backward compat).
	if settings.APIKey != "" {
		apiKeyToStore := settings.APIKey
		// Hash plaintext keys; skip if already a bcrypt hash or legacy SHA-256 hex digest.
		isBcrypt := strings.HasPrefix(settings.APIKey, "$2a$") || strings.HasPrefix(settings.APIKey, "$2b$")
		isSHA256 := len(settings.APIKey) == 64
		if !isBcrypt && !isSHA256 {
			apiKeyToStore = HashAPIKey(settings.APIKey)
		}
		err = UpdateSetting(db, store, "api_key", apiKeyToStore)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save API key")
			apiInternalError(c, "api_failed_to_save_api_key")
			return
		}
		store.SetAPIKey(apiKeyToStore)
	}

	err = UpdateSetting(db, store, "sensor_retention_days", settings.SensorRetentionDays)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save sensor retention setting")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	if v, convErr := strconv.Atoi(settings.SensorRetentionDays); convErr == nil {
		store.SetSensorRetention(v)
	}

	err = UpdateSetting(db, store, "log_level", settings.LogLevel)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save log level setting")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	store.SetLogLevel(settings.LogLevel)
	logger.SetLevel(settings.LogLevel)

	if settings.MaxBackupSizeMB != "" {
		if mb, convErr := strconv.Atoi(settings.MaxBackupSizeMB); convErr == nil && mb >= MinBackupSizeMB {
			err = UpdateSetting(db, store, "max_backup_size_mb", settings.MaxBackupSizeMB)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to save max backup size setting")
				apiInternalError(c, "api_failed_to_save_settings")
				return
			}
			store.SetMaxBackupSize(int64(mb) * 1024 * 1024)
		}
	}

	// Timezone setting — always persist (empty = system default)
	err = UpdateSetting(db, store, "timezone", settings.Timezone)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save timezone setting")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	store.SetTimezone(settings.Timezone)
	// Capture shadow metadata for future UTC migration
	if settings.Timezone != "" {
		captureTimezoneMetadata(db, store, settings.Timezone)
	}

	//Load Settings
	LoadSettings(db, store)

	apiOK(c, "api_settings_saved")
}

// UpdateSetting persists a key-value setting to the settings table and
// reloads the in-memory Store from DB so handlers see the new value
// immediately. The store argument is required when callers expect the
// reload behavior; main.go's first-boot bootstrap path passes the
// engine's Store, and SaveSettings does the same.
func UpdateSetting(db *sql.DB, store *config.Store, name string, value string) error {
	fieldLogger := logger.Log.WithField("func", "UpdateSetting")
	existId := 0
	// Query settings table and write result to console
	rows, err := db.Query("SELECT * FROM settings where name = $1", name)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read settings")
		return err
	}
	// Iterate over rows
	for rows.Next() {
		//update existId with id of row
		var id int
		var name string
		var value string
		var create_dt string
		var update_dt string
		err = rows.Scan(&id, &name, &value, &create_dt, &update_dt)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read settings")
			return err
		}
		existId = id
	}

	if existId == 0 {
		//Insert new setting
		_, err = db.Exec("INSERT INTO settings (name, value) VALUES ($1, $2)", name, value)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert setting")
		}
	} else {
		//Update existing setting
		_, err = db.Exec("UPDATE settings SET value = $1 WHERE id = $2", value, existId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to update setting")
		}
	}

	// Reload settings if a Store was provided. Some callers (e.g. the
	// first-boot bootstrap or tests touching settings without a Store)
	// pass nil to skip the reload.
	if store != nil {
		LoadSettings(db, store)
	}

	return nil
}

func GetSettings(db *sql.DB) types.SettingsData {
	fieldLogger := logger.Log.WithField("func", "GetSettings")

	settingsData := types.SettingsData{}

	rows, err := db.Query("SELECT * FROM settings")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read settings")
		return types.SettingsData{}
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, value, createDt, updateDt string
		err = rows.Scan(&id, &name, &value, &createDt, &updateDt)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read settings")
			continue
		}

		switch name {
		case "aci.enabled":
			settingsData.ACI.Enabled = value == "1"
		case "ec.enabled":
			settingsData.EC.Enabled = value == "1"
		case "aci.token":
			if value != "" {
				settingsData.ACI.TokenSet = true
			}
		case "polling_interval":
			settingsData.PollingInterval, _ = strconv.Atoi(value)
		case "guest_mode":
			settingsData.GuestMode = value == "1"
		case "stream_grab_enabled":
			settingsData.StreamGrabEnabled = value == "1"
		case "stream_grab_interval":
			iValue, err := strconv.Atoi(value)
			if err != nil {
				iValue = DefaultStreamGrabIntervalMs
			}
			settingsData.StreamGrabInterval = iValue
		case "api_key":
			// Only indicate that a key is set; never reveal the stored hash
			if value != "" {
				settingsData.APIKey = "********"
			}
		case "api_ingest_enabled":
			settingsData.APIIngestEnabled = value == "1"
		case "sensor_retention_days":
			settingsData.SensorRetentionDays, _ = strconv.Atoi(value)
		case "log_level":
			settingsData.LogLevel = value
		case "max_backup_size_mb":
			settingsData.MaxBackupSizeMB, _ = strconv.Atoi(value)
		case "timezone":
			settingsData.Timezone = value
		default:
			fieldLogger.WithField("name", name).Debug("Unrecognised setting skipped")
		}
	}

	return settingsData
}
func AddZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddZoneHandler")
	var zone struct {
		Name string `json:"zone_name"`
	}
	if err := c.ShouldBindJSON(&zone); err != nil {
		fieldLogger.WithError(err).Error("Failed to add zone")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("zone_name", zone.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Add zone to database
	db := DBFromContext(c)

	// Insert new zone and return new id
	var id int
	err := db.QueryRow("INSERT INTO zones (name) VALUES ($1) RETURNING id", zone.Name).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add zone")
		apiInternalError(c, "api_failed_to_add_zone")
		return
	}
	//Add the new zone to the config
	ConfigStoreFromContext(c).AppendZone(types.Zone{ID: uint(id), Name: zone.Name})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func AddMetricHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddMetricHandler")
	var metric struct {
		Name string `json:"metric_name"`
		Unit string `json:"metric_unit"`
	}

	if err := c.ShouldBindJSON(&metric); err != nil {
		fieldLogger.WithError(err).Error("Failed to add metric")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("metric_name", metric.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("metric_unit", metric.Unit, utils.MaxUnitLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// metric name of "Height" is reserved
	// if metric.Name == "Height" {
	// 	fieldLogger.Error("Failed to add metric")
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "This metric name is reserved and can't be added."})
	// 	return
	// }
	// No reserved metric names; treat like other metrics

	// Add metric to database
	db := DBFromContext(c)
	// Insert new metric and return new id
	var id int
	err := db.QueryRow("INSERT INTO metric (name, unit) VALUES ($1, $2) RETURNING id", metric.Name, metric.Unit).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add metric")
		apiInternalError(c, "api_failed_to_add_metric")
		return
	}
	ConfigStoreFromContext(c).AppendMetric(types.Metric{ID: id, Name: metric.Name, Unit: metric.Unit})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func AddActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddActivityHandler")
	var activity struct {
		Name       string `json:"activity_name"`
		IsWatering bool   `json:"is_watering"`
		IsFeeding  bool   `json:"is_feeding"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fieldLogger.WithError(err).Error("Failed to add activity")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("activity_name", activity.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	//Reserved names can't be added "Water", "Feed", "Note"
	if activity.Name == "Water" || activity.Name == "Feed" || activity.Name == "Note" {
		fieldLogger.Error("Failed to add activity")
		apiBadRequest(c, "api_activity_name_reserved")
		return
	}

	// Add activity to database
	db := DBFromContext(c)

	// Insert new activity and return new id
	var id int
	err := db.QueryRow(`
		INSERT INTO activity (name, is_watering, is_feeding)
		VALUES ($1, $2, $3)
		RETURNING id`, activity.Name, activity.IsWatering, activity.IsFeeding).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add activity")
		apiInternalError(c, "api_failed_to_add_activity")
		return
	}
	ConfigStoreFromContext(c).AppendActivity(types.Activity{ID: id, Name: activity.Name, IsWatering: activity.IsWatering, IsFeeding: activity.IsFeeding})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}
func UpdateZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateZoneHandler")
	id := c.Param("id")
	var zone struct {
		Name string `json:"zone_name"`
	}
	if err := c.ShouldBindJSON(&zone); err != nil {
		fieldLogger.WithError(err).Error("Failed to update zone")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("zone_name", zone.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update zone in database
	db := DBFromContext(c)

	// Update zone in database
	_, err := db.Exec("UPDATE zones SET name = $1 WHERE id = $2", zone.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update zone")
		apiInternalError(c, "api_failed_to_update_zone")
		return
	}
	//Reload Config
	ConfigStoreFromContext(c).SetZones(GetZones(db))

	apiOK(c, "api_zone_updated")
}

func UpdateMetricHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateMetricHandler")
	id := c.Param("id")
	var metric struct {
		Name string `json:"metric_name"`
		Unit string `json:"metric_unit"`
	}
	if err := c.ShouldBindJSON(&metric); err != nil {
		fieldLogger.WithError(err).Error("Failed to update metric")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("metric_name", metric.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("metric_unit", metric.Unit, utils.MaxUnitLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update metric in database
	db := DBFromContext(c)

	// Check if metric exists and lock = TRUE
	var lock bool
	var err error
	err = db.QueryRow("SELECT lock FROM metric WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update metric")
		apiInternalError(c, "api_failed_to_update_metric")
		return
	}
	if lock {
		// Previously edits were blocked when lock==true; allow update but warn in logs.
		fieldLogger.WithField("metricID", id).Warn("Metric is locked but update will be allowed")
	}

	// Update metric in database
	_, err = db.Exec("UPDATE metric SET name = $1, unit = $2 WHERE id = $3", metric.Name, metric.Unit, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update metric")
		apiInternalError(c, "api_failed_to_update_metric")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetMetrics(GetMetrics(db))

	apiOK(c, "api_metric_updated")
}

func UpdateActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateActivityHandler")
	id := c.Param("id")
	var activity struct {
		Name       string `json:"activity_name"`
		IsWatering bool   `json:"is_watering"`
		IsFeeding  bool   `json:"is_feeding"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("activity_name", activity.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update activity in database
	db := DBFromContext(c)

	// Check lock but do not block updates; log a warning if locked
	var lock bool
	var err error
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to check activity lock; proceeding with update")
	} else if lock {
		fieldLogger.WithField("activityID", id).Warn("Activity is locked but update will be allowed")
	}

	// Perform the update
	_, err = db.Exec(`
		UPDATE activity
		SET name = $1, is_watering = $2, is_feeding = $3
		WHERE id = $4`, activity.Name, activity.IsWatering, activity.IsFeeding, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiInternalError(c, "api_failed_to_update_activity")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetActivities(GetActivities(db))

	apiOK(c, "api_activity_updated")
}
func DeleteZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteZoneHandler")
	id := c.Param("id")

	// Delete zone from database
	db := DBFromContext(c)

	//Build a list of plants associated with this zoen to delete first
	var err error
	rows, err := db.Query("SELECT id FROM plant WHERE zone_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plants")
		return
	}
	defer rows.Close()

	plantList := []int{}
	for rows.Next() {
		var plantId int
		err = rows.Scan(&plantId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to delete plant")
			continue
		}
		plantList = append(plantList, plantId)
	}

	for _, plantId := range plantList {
		DeletePlantById(db, fmt.Sprintf("%d", plantId))
	}

	//Build a list of sensors associated with this zoen to delete first
	rows, err = db.Query("SELECT id FROM sensors WHERE zone_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete sensors")
		return
	}
	defer rows.Close()

	sensorList := []int{}
	for rows.Next() {
		var sensorId int
		err = rows.Scan(&sensorId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to delete sensor")
			continue
		}
		sensorList = append(sensorList, sensorId)
	}

	for _, sensorId := range sensorList {
		DeleteSensorByID(db, fmt.Sprintf("%d", sensorId))
	}

	// Build a list of streams associated with this zone to delete first
	rows, err = db.Query("SELECT id FROM streams WHERE zone_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete streams")
		return
	}
	defer rows.Close()

	streamList := []int{}
	for rows.Next() {
		var streamId int
		err = rows.Scan(&streamId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to delete stream")
			continue
		}
		streamList = append(streamList, streamId)
	}
	for _, streamId := range streamList {
		DeleteStreamByID(db, fmt.Sprintf("%d", streamId))
	}

	// Delete zone from database
	_, err = db.Exec("DELETE FROM zones WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete zone")
		apiInternalError(c, "api_failed_to_delete_zone")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetZones(GetZones(db))

	apiOK(c, "api_zone_deleted")
}

func DeleteMetricHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteMetricHandler")
	id := c.Param("id")

	// Delete metric from database
	db := DBFromContext(c)

	// Check if metric exists and lock = TRUE
	var lock bool
	var err error
	err = db.QueryRow("SELECT lock FROM metric WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete metric")
		apiInternalError(c, "api_failed_to_delete_metric")
		return
	}
	if lock {
		fieldLogger.Error("Failed to delete metric")
		apiBadRequest(c, "api_metric_cannot_delete")
		return
	}

	// Delete any measurements associated with this metric
	_, err = db.Exec("DELETE FROM plant_measurements WHERE metric_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete measurements")
		apiInternalError(c, "api_failed_to_delete_measurements")
		return
	}

	// Delete metric from database
	_, err = db.Exec("DELETE FROM metric WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete metric")
		apiInternalError(c, "api_failed_to_delete_metric")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetMetrics(GetMetrics(db))

	apiOK(c, "api_metric_deleted")
}

func DeleteActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteActivityHandler")
	id := c.Param("id")

	// Delete activity from database
	db := DBFromContext(c)

	// Check lock but do not block deletion; log a warning if locked
	var lock bool
	var err error
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to check activity lock; proceeding with deletion")
	} else if lock {
		fieldLogger.WithField("activityID", id).Warn("Activity is locked but deletion will be allowed")
	}

	// Delete any plant_activities associated with this activity
	_, err = db.Exec("DELETE FROM plant_activity WHERE activity_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant_activities")
		apiInternalError(c, "api_failed_to_delete_plant_activities")
		return
	}

	// Delete activity from database
	_, err = db.Exec("DELETE FROM activity WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		apiInternalError(c, "api_failed_to_delete_activity")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetActivities(GetActivities(db))

	apiOK(c, "api_activity_deleted")
}

func GetSetting(db *sql.DB, name string) (string, error) {
	fieldLogger := logger.Log.WithField("func", "GetSetting")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&value)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			fieldLogger.WithField("name", name).Debug("Setting not found")
			return "", nil
		} else {
			fieldLogger.WithError(err).Error("Failed to read setting")
			return "", err
		}
	}

	return value, nil
}
func UploadLogo(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UploadLogo")
	// Parse the multipart form data
	err := c.Request.ParseMultipartForm(MaxMultipartFormSize) // Limit to 10 MB
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to parse form data")
		apiBadRequest(c, "api_failed_to_parse_form")
		return
	}

	// Retrieve the file from the "logo" field
	fileHeader, err := c.FormFile("logo")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to retrieve file")
		apiBadRequest(c, "api_failed_to_retrieve_file")
		return
	}

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open file")
		apiInternalError(c, "api_failed_to_open_file")
		return
	}
	defer file.Close()

	// Validate MIME type
	sniff := make([]byte, 512)
	n, _ := file.Read(sniff)
	mimeType := http.DetectContentType(sniff[:n])
	allowedMIME := map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}
	if !allowedMIME[mimeType] {
		fieldLogger.WithField("mimeType", mimeType).Warn("Rejected logo upload with disallowed MIME type")
		apiBadRequest(c, "api_invalid_file_type")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		fieldLogger.WithError(err).Error("Failed to seek logo file")
		apiInternalError(c, "api_failed_to_process_file")
		return
	}

	// Generate a unique file path under the per-engine upload root.
	timestamp := time.Now().UnixNano()
	fileName := fmt.Sprintf("logo_image_%d%s", timestamp, filepath.Ext(fileHeader.Filename))
	savePath := filepath.Join(UploadDirFromContext(c), "logos", fileName)

	// Create the uploads/logos directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create directory")
		apiInternalError(c, "api_failed_to_create_directory")
		return
	}

	// Save the file to the filesystem
	out, err := os.Create(savePath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		apiInternalError(c, "api_failed_to_save_file")
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		apiInternalError(c, "api_failed_to_save_file")
		return
	}

	// Update the database with the new logo path
	db := DBFromContext(c)
	err = UpdateSetting(db, ConfigStoreFromContext(c), "logo_image", fileName)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update logo setting")
		apiInternalError(c, "api_failed_to_update_logo")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_logo_uploaded"), "path": savePath})
}

func LoadEcDevices(db *sql.DB) ([]string, error) {
	fieldLogger := logger.Log.WithField("func", "LoadEcDevices")
	var ecDevices []string

	//Iterate over sensors table, looking for distinct device with type ecowitt
	rows, err := db.Query("SELECT DISTINCT device FROM sensors WHERE source = 'ecowitt'")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read sensors")
		return ecDevices, err
	}
	//build a list of devices to scan

	for rows.Next() {
		var device string
		err = rows.Scan(&device)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read sensors")
			return ecDevices, err
		}
		ecDevices = append(ecDevices, device)
	}

	return ecDevices, nil
}

func ExistsSetting(db *sql.DB, s string) (bool, error) {
	fieldLogger := logger.Log.WithField("func", "ExistsSetting")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE name = $1", s).Scan(&value)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		} else {
			fieldLogger.WithError(err).Error("Failed to read setting")
			return false, err
		}
	}

	return true, nil

}

// directUpdateSetting persists a key-value setting WITHOUT calling LoadSettings.
// Use this inside LoadSettings itself to avoid infinite recursion.
func directUpdateSetting(db *sql.DB, name string, value string) error {
	fieldLogger := logger.Log.WithField("func", "directUpdateSetting")
	existId := 0
	rows, err := db.Query("SELECT id FROM settings WHERE name = $1", name)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read settings")
		return err
	}
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&existId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read settings")
			return err
		}
	}

	if existId == 0 {
		_, err = db.Exec("INSERT INTO settings (name, value) VALUES ($1, $2)", name, value)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert setting")
		}
	} else {
		_, err = db.Exec("UPDATE settings SET value = $1 WHERE id = $2", value, existId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to update setting")
		}
	}
	return err
}

// captureTimezoneMetadata records shadow metadata about the timezone state
// of the installation. This data will be used in a future release to inform
// automated data migration when we normalise all timestamps to UTC.
func captureTimezoneMetadata(db *sql.DB, store *config.Store, userTZ string) {
	fieldLogger := logger.Log.WithField("func", "captureTimezoneMetadata")

	// tz_system: the Go process's system timezone (from TZ env or OS)
	sysTZ := time.Now().Location().String()
	if err := UpdateSetting(db, store, "tz_system", sysTZ); err != nil {
		fieldLogger.WithError(err).Error("Failed to save tz_system")
	}

	// tz_database: query the DB engine's idea of current time
	dbTZ := ""
	row := db.QueryRow("SELECT CURRENT_TIMESTAMP")
	var dbTime string
	if err := row.Scan(&dbTime); err == nil {
		dbTZ = dbTime
	} else {
		fieldLogger.WithError(err).Warn("Failed to query DB timestamp")
	}
	if err := UpdateSetting(db, store, "tz_database", dbTZ); err != nil {
		fieldLogger.WithError(err).Error("Failed to save tz_database")
	}

	// tz_user: what the user selected in the UI
	if err := UpdateSetting(db, store, "tz_user", userTZ); err != nil {
		fieldLogger.WithError(err).Error("Failed to save tz_user")
	}

	// tz_snapshot_at: when this snapshot was taken (server wall-clock)
	snapshot := time.Now().Format(time.RFC3339)
	if err := UpdateSetting(db, store, "tz_snapshot_at", snapshot); err != nil {
		fieldLogger.WithError(err).Error("Failed to save tz_snapshot_at")
	}
}

// LoadSettings refreshes the supplied *config.Store from the settings
// table. It takes the *sql.DB and *config.Store explicitly so tests
// (and the in-process harness) construct one Store per engine and the
// production path threads the engine's Store through the same code.
func LoadSettings(db *sql.DB, store *config.Store) {
	fieldLogger := logger.Log.WithField("func", "LoadSettings")

	if db == nil {
		fieldLogger.Error("LoadSettings called with nil DB")
		return
	}
	if store == nil {
		fieldLogger.Error("LoadSettings called with nil Store")
		return
	}

	var err error
	strPollingInterval, err := GetSetting(db, "polling_interval")
	if err == nil {
		if iPollingInterval, err := strconv.Atoi(strPollingInterval); err == nil {
			store.SetPollingInterval(iPollingInterval)
		}
	}

	strACIEnabled, err := GetSetting(db, "aci.enabled")
	if err == nil {
		if iACIEnabled, err := strconv.Atoi(strACIEnabled); err == nil {
			store.SetACIEnabled(iACIEnabled)
		}
	}

	strECEnabled, err := GetSetting(db, "ec.enabled")
	if err == nil {
		if iECEnabled, err := strconv.Atoi(strECEnabled); err == nil {
			store.SetECEnabled(iECEnabled)
		}
	}

	strACIToken, err := GetSetting(db, "aci.token")
	if err == nil {
		store.SetACIToken(strACIToken)
	}

	strECDevices, err := LoadEcDevices(db)
	if err == nil {
		store.SetECDevices(strECDevices)
	}

	strGuestMode, err := GetSetting(db, "guest_mode")
	if err == nil {
		if iGuestMode, err := strconv.Atoi(strGuestMode); err == nil {
			store.SetGuestMode(iGuestMode)
		}
	}

	strStreamGrabEnabled, err := GetSetting(db, "stream_grab_enabled")
	if err == nil {
		if iStreamGrabEnabled, err := strconv.Atoi(strStreamGrabEnabled); err == nil {
			store.SetStreamGrabEnabled(iStreamGrabEnabled)
		}
	}

	strStreamGrabInterval, err := GetSetting(db, "stream_grab_interval")
	if err == nil {
		if iStreamGrabInterval, err := strconv.Atoi(strStreamGrabInterval); err == nil {
			store.SetStreamGrabInterval(iStreamGrabInterval)
		}
	}

	strAPIKey, err := GetSetting(db, "api_key")
	if err == nil {
		fieldLogger.Debug("API key setting loaded")

		store.SetAPIKey(strAPIKey)
	} else {
		// Log out error
		fieldLogger.WithError(err).Error("Failed to get API key setting")
	}

	strSensorRetention, err := GetSetting(db, "sensor_retention_days")
	if err == nil {
		if iSensorRetention, err := strconv.Atoi(strSensorRetention); err == nil {
			store.SetSensorRetention(iSensorRetention)
		}
	}

	strLogLevel, err := GetSetting(db, "log_level")
	if err == nil && strLogLevel != "" {
		store.SetLogLevel(strLogLevel)
		logger.SetLevel(strLogLevel)
	}

	strMaxBackupSize, err := GetSetting(db, "max_backup_size_mb")
	if err == nil {
		if mb, err := strconv.Atoi(strMaxBackupSize); err == nil && mb >= MinBackupSizeMB {
			store.SetMaxBackupSize(int64(mb) * 1024 * 1024)
		}
	}

	strTimezone, err := GetSetting(db, "timezone")
	if err == nil && strTimezone != "" {
		store.SetTimezone(strTimezone)
	}

	// On first boot after the timezone migration, capture a baseline snapshot
	// of the system timezone state even before the user touches settings.
	// IMPORTANT: use directUpdateSetting here, NOT UpdateSetting, to avoid
	// infinite recursion (UpdateSetting calls LoadSettings).
	strSnapshot, snapshotErr := GetSetting(db, "tz_snapshot_at")
	if snapshotErr == nil && strSnapshot == "" {
		sysTZ := time.Now().Location().String()
		_ = directUpdateSetting(db, "tz_system", sysTZ)
		dbTZ := ""
		row := db.QueryRow("SELECT CURRENT_TIMESTAMP")
		var dbTime string
		if err := row.Scan(&dbTime); err == nil {
			dbTZ = dbTime
		}
		_ = directUpdateSetting(db, "tz_database", dbTZ)
		_ = directUpdateSetting(db, "tz_snapshot_at", time.Now().Format(time.RFC3339))
		fieldLogger.Info("Captured initial timezone metadata snapshot")
	}

	store.SetActivities(GetActivities(db))
	store.SetMetrics(GetMetrics(db))
	store.SetStatuses(GetStatuses(db))
	store.SetZones(GetZones(db))
	store.SetStrains(GetStrains(db))
	store.SetBreeders(GetBreeders(db))
	store.SetStreams(GetStreams(db))
}

// Breeder CRUD handlers have been moved to strain.go
// Stream CRUD handlers have been moved to stream.go

func GetLogs(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetLogs")

	linesParam := c.DefaultQuery("lines", strconv.Itoa(DefaultLogLines))
	n, err := strconv.Atoi(linesParam)
	if err != nil || n < MinLogLines {
		n = DefaultLogLines
	}
	if n > MaxLogLines {
		n = MaxLogLines
	}

	fileParam := c.DefaultQuery("file", "app")
	logsDir := LogsDirFromContext(c)
	var logPath string
	switch fileParam {
	case "access":
		logPath = filepath.Join(logsDir, "access.log")
	default:
		logPath = filepath.Join(logsDir, "app.log")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read log file")
		apiInternalError(c, "api_failed_to_read_log_file")
		return
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	c.JSON(http.StatusOK, gin.H{
		"lines": strings.Join(lines, "\n"),
		"total": len(lines),
	})
}

func DownloadLogs(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DownloadLogs")

	fileParam := c.DefaultQuery("file", "app")
	logsDir := LogsDirFromContext(c)
	var filePath, fileName string
	switch fileParam {
	case "access":
		filePath = filepath.Join(logsDir, "access.log")
		fileName = "access.log"
	default:
		filePath = filepath.Join(logsDir, "app.log")
		fileName = "app.log"
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fieldLogger.WithError(err).Error("Log file not found")
		apiNotFound(c, "api_log_file_not_found")
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Type", "text/plain")
	c.File(filePath)
}

// DeleteStreamByID has been moved to stream.go
