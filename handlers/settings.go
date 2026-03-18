package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"isley/config"
	"isley/logger"
	model "isley/model"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GenerateAPIKey() string {
	// Generate a random 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

// HashAPIKey returns the SHA-256 hex digest of the given plaintext API key.
func HashAPIKey(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func SaveSettings(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveSettings")
	var settings types.Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiBadRequest(c, "Invalid settings payload")
		return
	}

	// Generate new API key if requested
	if settings.APIKey == "generate" {
		plaintextKey := GenerateAPIKey()
		hashedKey := HashAPIKey(plaintextKey)

		// Store the hashed key in the database
		err := UpdateSetting("api_key", hashedKey)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save API key")
			apiInternalError(c, "Failed to save API key")
			return
		}
		config.APIKey = hashedKey

		// Return the plaintext key only once — it cannot be retrieved again
		c.JSON(http.StatusOK, gin.H{
			"message": T(c, "api_api_key_generated"),
			"api_key": plaintextKey,
		})
		return
	}

	// saveBool persists a boolean setting as "1"/"0" and updates the
	// corresponding config int in one step, removing the repetitive
	// if/else blocks that previously handled each boolean individually.
	saveBool := func(key string, val bool, configField *int) error {
		dbVal := "0"
		cfgVal := 0
		if val {
			dbVal = "1"
			cfgVal = 1
		}
		if err := UpdateSetting(key, dbVal); err != nil {
			return err
		}
		*configField = cfgVal
		return nil
	}

	boolSettings := []struct {
		key   string
		val   bool
		field *int
	}{
		{"aci.enabled", settings.ACI.Enabled, &config.ACIEnabled},
		{"ec.enabled", settings.EC.Enabled, &config.ECEnabled},
		{"guest_mode", settings.GuestMode, &config.GuestMode},
		{"stream_grab_enabled", settings.StreamGrabEnabled, &config.StreamGrabEnabled},
		{"api_ingest_enabled", !settings.DisableAPIIngest, &config.APIIngestEnabled},
	}

	for _, bs := range boolSettings {
		if err := saveBool(bs.key, bs.val, bs.field); err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			apiInternalError(c, "api_failed_to_save_settings")
			return
		}
	}

	err := UpdateSetting("polling_interval", settings.PollingInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	config.PollingInterval, _ = strconv.Atoi(settings.PollingInterval)

	err = UpdateSetting("stream_grab_interval", settings.StreamGrabInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	config.StreamGrabInterval, _ = strconv.Atoi(settings.StreamGrabInterval)

	// API key is managed separately via the "generate" flow.
	// Only update if a non-empty value is explicitly provided (backward compat).
	if settings.APIKey != "" {
		apiKeyToStore := settings.APIKey
		// Hash plaintext keys; skip if already a SHA-256 hex digest (64 chars).
		if len(settings.APIKey) != 64 {
			apiKeyToStore = HashAPIKey(settings.APIKey)
		}
		err = UpdateSetting("api_key", apiKeyToStore)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save API key")
			apiInternalError(c, "Failed to save API key")
			return
		}
		config.APIKey = apiKeyToStore
	}

	err = UpdateSetting("sensor_retention_days", settings.SensorRetentionDays)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save sensor retention setting")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	} else {
		config.SensorRetention, _ = strconv.Atoi(settings.SensorRetentionDays)
	}

	err = UpdateSetting("log_level", settings.LogLevel)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save log level setting")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	} else {
		config.LogLevel = settings.LogLevel
		logger.SetLevel(config.LogLevel)
	}

	//Load Settings
	LoadSettings()

	apiOK(c, "api_settings_saved")
}

func UpdateSetting(name string, value string) error {
	fieldLogger := logger.Log.WithField("func", "UpdateSetting")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return err
	}
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

	// Reload settings
	LoadSettings()

	return nil
}

func GetSettings() types.SettingsData {
	fieldLogger := logger.Log.WithField("func", "GetSettings")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return types.SettingsData{}
	}

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
				iValue = 3000
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
		apiBadRequest(c, "Invalid payload")
		return
	}
	if err := utils.ValidateRequiredString("zone_name", zone.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Add zone to database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add zone")
		return
	}

	// Insert new zone and return new id
	var id int
	err = db.QueryRow("INSERT INTO zones (name) VALUES ($1) RETURNING id", zone.Name).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add zone")
		apiInternalError(c, "api_failed_to_add_zone")
		return
	}
	//Add the new zone to the config
	config.Zones = append(config.Zones, types.Zone{ID: uint(id), Name: zone.Name})

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
		apiBadRequest(c, "Invalid payload")
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
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add metric")
		return
	}
	// Insert new metric and return new id
	var id int
	err = db.QueryRow("INSERT INTO metric (name, unit) VALUES ($1, $2) RETURNING id", metric.Name, metric.Unit).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add metric")
		apiInternalError(c, "api_failed_to_add_metric")
		return
	}
	config.Metrics = append(config.Metrics, types.Metric{ID: id, Name: metric.Name, Unit: metric.Unit})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func AddActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddActivityHandler")
	var activity struct {
		Name string `json:"activity_name"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fieldLogger.WithError(err).Error("Failed to add activity")
		apiBadRequest(c, "Invalid payload")
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
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add activity")
		return
	}

	// Insert new activity and return new id
	var id int
	err = db.QueryRow("INSERT INTO activity (name) VALUES ($1) RETURNING id", activity.Name).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add activity")
		apiInternalError(c, "api_failed_to_add_activity")
		return
	}
	config.Activities = append(config.Activities, types.Activity{ID: id, Name: activity.Name})

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
		apiBadRequest(c, "Invalid payload")
		return
	}
	if err := utils.ValidateRequiredString("zone_name", zone.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update zone in database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update zone")
		return
	}

	// Update zone in database
	_, err = db.Exec("UPDATE zones SET name = $1 WHERE id = $2", zone.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update zone")
		apiInternalError(c, "api_failed_to_update_zone")
		return
	}
	//Reload Config
	config.Zones = GetZones()

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
		apiBadRequest(c, "Invalid payload")
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
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update metric")
		return
	}

	// Check if metric exists and lock = TRUE
	var lock bool
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
	config.Metrics = GetMetrics()

	apiOK(c, "api_metric_updated")
}

func UpdateActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateActivityHandler")
	id := c.Param("id")
	var activity struct {
		Name string `json:"activity_name"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiBadRequest(c, "Invalid payload")
		return
	}
	if err := utils.ValidateRequiredString("activity_name", activity.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update activity in database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		return
	}

	// Check lock but do not block updates; log a warning if locked
	var lock bool
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to check activity lock; proceeding with update")
	} else if lock {
		fieldLogger.WithField("activityID", id).Warn("Activity is locked but update will be allowed")
	}

	// Perform the update
	_, err = db.Exec("UPDATE activity SET name = $1 WHERE id = $2", activity.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiInternalError(c, "api_failed_to_update_activity")
		return
	}

	//Reload Config
	config.Activities = GetActivities()

	apiOK(c, "api_activity_updated")
}
func DeleteZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteZoneHandler")
	id := c.Param("id")

	// Delete zone from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete zone")
		return
	}

	//Build a list of plants associated with this zoen to delete first
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
		DeletePlantById(fmt.Sprintf("%d", plantId))
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
		DeleteSensorByID(fmt.Sprintf("%d", sensorId))
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
		DeleteStreamByID(fmt.Sprintf("%d", streamId))
	}

	// Delete zone from database
	_, err = db.Exec("DELETE FROM zones WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete zone")
		apiInternalError(c, "api_failed_to_delete_zone")
		return
	}

	//Reload Config
	config.Zones = GetZones()

	apiOK(c, "api_zone_deleted")
}

func DeleteMetricHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteMetricHandler")
	id := c.Param("id")

	// Delete metric from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete metric")
		return
	}

	// Check if metric exists and lock = TRUE
	var lock bool
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
	config.Metrics = GetMetrics()

	apiOK(c, "api_metric_deleted")
}

func DeleteActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteActivityHandler")
	id := c.Param("id")

	// Delete activity from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		apiInternalError(c, "api_failed_to_delete_activity")
		return
	}

	// Check lock but do not block deletion; log a warning if locked
	var lock bool
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
	config.Activities = GetActivities()

	apiOK(c, "api_activity_deleted")
}

func GetSetting(name string) (string, error) {
	fieldLogger := logger.Log.WithField("func", "GetSetting")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return "", err
	}

	var value string
	err = db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&value)
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
	err := c.Request.ParseMultipartForm(10 << 20) // Limit to 10 MB
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

	// Generate a unique file path
	timestamp := time.Now().UnixNano()
	fileName := fmt.Sprintf("logo_image_%d%s", timestamp, filepath.Ext(fileHeader.Filename))
	savePath := filepath.Join("uploads", "logos", fileName)

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
	err = UpdateSetting("logo_image", fileName)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update logo setting")
		apiInternalError(c, "api_failed_to_update_logo")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_logo_uploaded"), "path": savePath})
}

func LoadEcDevices() ([]string, error) {
	fieldLogger := logger.Log.WithField("func", "LoadEcDevices")
	var ecDevices []string
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return ecDevices, err
	}

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

func ExistsSetting(s string) (bool, error) {
	fieldLogger := logger.Log.WithField("func", "ExistsSetting")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return false, err
	}

	var value string
	err = db.QueryRow("SELECT value FROM settings WHERE name = $1", s).Scan(&value)
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

// Helper functions
func LoadSettings() {
	fieldLogger := logger.Log.WithField("func", "LoadSettings")

	strPollingInterval, err := GetSetting("polling_interval")
	if err == nil {
		if iPollingInterval, err := strconv.Atoi(strPollingInterval); err == nil {
			config.PollingInterval = iPollingInterval
		}
	}

	strACIEnabled, err := GetSetting("aci.enabled")
	if err == nil {
		if iACIEnabled, err := strconv.Atoi(strACIEnabled); err == nil {
			config.ACIEnabled = iACIEnabled
		}
	}

	strECEnabled, err := GetSetting("ec.enabled")
	if err == nil {
		if iECEnabled, err := strconv.Atoi(strECEnabled); err == nil {
			config.ECEnabled = iECEnabled
		}
	}

	strACIToken, err := GetSetting("aci.token")
	if err == nil {
		config.ACIToken = strACIToken
	}

	strECDevices, err := LoadEcDevices()
	if err == nil {
		config.ECDevices = strECDevices
	}

	strGuestMode, err := GetSetting("guest_mode")
	if err == nil {
		if iGuestMode, err := strconv.Atoi(strGuestMode); err == nil {
			config.GuestMode = iGuestMode
		}
	}

	strStreamGrabEnabled, err := GetSetting("stream_grab_enabled")
	if err == nil {
		if iStreamGrabEnabled, err := strconv.Atoi(strStreamGrabEnabled); err == nil {
			config.StreamGrabEnabled = iStreamGrabEnabled
		}
	}

	strStreamGrabInterval, err := GetSetting("stream_grab_interval")
	if err == nil {
		if iStreamGrabInterval, err := strconv.Atoi(strStreamGrabInterval); err == nil {
			config.StreamGrabInterval = iStreamGrabInterval
		}
	}

	strAPIKey, err := GetSetting("api_key")
	if err == nil {
		fieldLogger.Debug("API key setting loaded")

		config.APIKey = strAPIKey
	} else {
		// Log out error
		fieldLogger.WithError(err).Error("Failed to get API key setting")
	}

	strSensorRetention, err := GetSetting("sensor_retention_days")
	if err == nil {
		if iSensorRetention, err := strconv.Atoi(strSensorRetention); err == nil {
			config.SensorRetention = iSensorRetention
		}
	}

	strLogLevel, err := GetSetting("log_level")
	if err == nil && strLogLevel != "" {
		config.LogLevel = strLogLevel
		logger.SetLevel(config.LogLevel)
	}

	config.Activities = GetActivities()
	config.Metrics = GetMetrics()
	config.Statuses = GetStatuses()
	config.Zones = GetZones()
	config.Strains = GetStrains()
	config.Breeders = GetBreeders()
	config.Streams = GetStreams()
}
// Breeder CRUD handlers have been moved to strain.go
// Stream CRUD handlers have been moved to stream.go

func GetLogs(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetLogs")

	linesParam := c.DefaultQuery("lines", "200")
	n, err := strconv.Atoi(linesParam)
	if err != nil || n < 1 {
		n = 200
	}
	if n > 2000 {
		n = 2000
	}

	fileParam := c.DefaultQuery("file", "app")
	var logPath string
	switch fileParam {
	case "access":
		logPath = "logs/access.log"
	default:
		logPath = "logs/app.log"
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
	var filePath, fileName string
	switch fileParam {
	case "access":
		filePath = "logs/access.log"
		fileName = "access.log"
	default:
		filePath = "logs/app.log"
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
