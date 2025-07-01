package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
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
	"time"
)

func GenerateAPIKey() string {
	// Generate a random 32-character hex string
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return hex.EncodeToString(bytes)
}

func SaveSettings(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveSettings")
	var settings types.Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate new API key if requested
	if settings.APIKey == "generate" {
		settings.APIKey = GenerateAPIKey()

		// Save the new API key to the database
		err := UpdateSetting("api_key", settings.APIKey)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save API key")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save API key"})
			return
		} else {
			config.APIKey = settings.APIKey
		}

		// Return the new API key in the response
		c.JSON(http.StatusOK, gin.H{"message": "API key generated successfully", "api_key": settings.APIKey})
		return
	}

	// Save settings logic (e.g., to a database or config file)
	//fmt.Printf("Received settings: %+v\n", settings)
	if settings.ACI.Enabled {
		err := UpdateSetting("aci.enabled", "1")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ACIEnabled = 1
		}
	} else {
		err := UpdateSetting("aci.enabled", "0")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ACIEnabled = 0
		}
	}

	if settings.EC.Enabled {
		err := UpdateSetting("ec.enabled", "1")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ECEnabled = 1
		}
	} else {
		err := UpdateSetting("ec.enabled", "0")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ECEnabled = 0
		}
	}
	err := UpdateSetting("polling_interval", settings.PollingInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	} else {
		config.PollingInterval, _ = strconv.Atoi(settings.PollingInterval)
	}
	if settings.GuestMode {
		err := UpdateSetting("guest_mode", "1")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.GuestMode = 1
		}
	} else {
		err := UpdateSetting("guest_mode", "0")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.GuestMode = 0
		}
	}
	if settings.StreamGrabEnabled {
		err := UpdateSetting("stream_grab_enabled", "1")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.StreamGrabEnabled = 1
		}
	} else {
		err := UpdateSetting("stream_grab_enabled", "0")
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.StreamGrabEnabled = 0
		}
	}
	err = UpdateSetting("stream_grab_interval", settings.StreamGrabInterval)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	} else {
		config.StreamGrabInterval, _ = strconv.Atoi(settings.StreamGrabInterval)
	}

	err = UpdateSetting("api_key", settings.APIKey)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save API key")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save API key"})
		return
	} else {
		config.APIKey = settings.APIKey
	}

	//Load Settings
	LoadSettings()

	c.JSON(http.StatusOK, gin.H{"message": "Settings saved successfully"})
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
			settingsData.APIKey = value
		default:
			fieldLogger.WithField("name", name).Warn("Unknown setting found")
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add zone"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// metric name of "Height" is reserved
	if metric.Name == "Height" {
		fieldLogger.Error("Failed to add metric")
		c.JSON(http.StatusBadRequest, gin.H{"error": "This metric name is reserved and can't be added."})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add metric"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	//Reserved names can't be added "Water", "Feed", "Note"
	if activity.Name == "Water" || activity.Name == "Feed" || activity.Name == "Note" {
		fieldLogger.Error("Failed to add activity")
		c.JSON(http.StatusBadRequest, gin.H{"error": "This activity name is reserved and can't be added."})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add activity"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update zone"})
		return
	}
	//Reload Config
	config.Zones = GetZones()

	c.JSON(http.StatusOK, gin.H{"message": "Zone updated"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
		return
	}
	if lock {
		//Update the unit only
		_, err = db.Exec("UPDATE metric SET unit = $1 WHERE id = $2", metric.Unit, id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to update metric")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"error": "Editing this metric is not allowed, only unit changed."})

		// Reload Config
		config.Metrics = GetMetrics()

		return
	}

	// Update metric in database
	_, err = db.Exec("UPDATE metric SET name = $1, unit = $2 WHERE id = $3", metric.Name, metric.Unit, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update metric")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
		return
	}

	//Reload Config
	config.Metrics = GetMetrics()

	c.JSON(http.StatusOK, gin.H{"message": "Metric updated"})
}

func UpdateActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateActivityHandler")
	id := c.Param("id")
	var activity struct {
		Name string `json:"activity_name"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update activity in database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		return
	}

	// Check if activity exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}
	if lock {
		fieldLogger.Error("Failed to update activity")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Editing this activity is not allowed."})
		return
	}

	// Update activity in database
	_, err = db.Exec("UPDATE activity SET name = $1 WHERE id = $2", activity.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	//Reload Config
	config.Activities = GetActivities()

	c.JSON(http.StatusOK, gin.H{"message": "Activity updated"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete zone"})
		return
	}

	//Reload Config
	config.Zones = GetZones()

	c.JSON(http.StatusOK, gin.H{"message": "Zone deleted"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete metric"})
		return
	}
	if lock {
		fieldLogger.Error("Failed to delete metric")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting this metric is not allowed."})
		return
	}

	// Delete any measurements associated with this metric
	_, err = db.Exec("DELETE FROM plant_measurements WHERE metric_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete measurements")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete measurements"})
		return
	}

	// Delete metric from database
	_, err = db.Exec("DELETE FROM metric WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete metric")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete metric"})
		return
	}

	//Reload Config
	config.Metrics = GetMetrics()

	c.JSON(http.StatusOK, gin.H{"message": "Metric deleted"})
}

func DeleteActivityHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteActivityHandler")
	id := c.Param("id")

	// Delete activity from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		return
	}

	// Check if activity exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}
	if lock {
		fieldLogger.Error("Failed to delete activity")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting this activity is not allowed."})
		return
	}

	// Delete any plant_activities associated with this activity
	_, err = db.Exec("DELETE FROM plant_activity WHERE activity_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant_activities")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete plant_activities"})
		return
	}

	// Delete activity from database
	_, err = db.Exec("DELETE FROM activity WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	//Reload Config
	config.Activities = GetActivities()

	c.JSON(http.StatusOK, gin.H{"message": "Activity deleted"})
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
			fieldLogger.WithField("name", name).Warn("Setting not found")
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Retrieve the file from the "logo" field
	fileHeader, err := c.FormFile("logo")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to retrieve file")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to retrieve file"})
		return
	}

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open file")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer file.Close()

	// Generate a unique file path
	timestamp := time.Now().In(time.Local).UnixNano()
	fileName := fmt.Sprintf("logo_image_%d%s", timestamp, filepath.Ext(fileHeader.Filename))
	savePath := filepath.Join("uploads", "logos", fileName)

	// Create the uploads/logos directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create directory")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Save the file to the filesystem
	out, err := os.Create(savePath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Update the database with the new logo path
	err = UpdateSetting("logo_image", fileName)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update logo setting")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update logo setting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logo uploaded successfully", "path": savePath})
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
		// Log out API key setting
		fieldLogger.WithField("api_key", strAPIKey).Debug("API key setting")

		config.APIKey = strAPIKey
	} else {
		// Log out error
		fieldLogger.WithError(err).Error("Failed to get API key setting")
	}

	config.Activities = GetActivities()
	config.Metrics = GetMetrics()
	config.Statuses = GetStatuses()
	config.Zones = GetZones()
	config.Strains = GetStrains()
	config.Breeders = GetBreeders()
	config.Streams = GetStreams()
}
func AddBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddBreederHandler")
	var breeder struct {
		Name string `json:"breeder_name"`
	}
	if err := c.ShouldBindJSON(&breeder); err != nil {
		fieldLogger.WithError(err).Error("Failed to add breeder")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Add breeder to database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add breeder")
		return
	}

	// Insert new breeder and return new id
	var id int
	err = db.QueryRow("INSERT INTO breeder (name) VALUES ($1) RETURNING id", breeder.Name).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add breeder")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add breeder"})
		return
	}
	config.Breeders = append(config.Breeders, types.Breeder{ID: id, Name: breeder.Name})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}
func UpdateBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateBreederHandler")
	id := c.Param("id")
	var breeder struct {
		Name string `json:"breeder_name"`
	}
	if err := c.ShouldBindJSON(&breeder); err != nil {
		fieldLogger.WithError(err).Error("Failed to update breeder")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update breeder in database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update breeder")
		return
	}

	// Update breeder in database
	_, err = db.Exec("UPDATE breeder SET name = $1 WHERE id = $2", breeder.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update breeder")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update breeder"})
		return
	}

	//Reload Config
	config.Breeders = GetBreeders()

	c.JSON(http.StatusOK, gin.H{"message": "Breeder updated"})
}

func DeleteBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteBreederHandler")
	id := c.Param("id")

	// Delete breeder from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete breeder")
		return
	}

	// Delete any plants associated with this breeder
	rows, err := db.Query("SELECT p.id FROM plant p LEFT OUTER JOIN strain s on s.id = p.strain_id WHERE s.breeder_id = $1", id)
	if err != nil {
		if err.Error() != "sql: no rows in result set" {

		} else {
			fieldLogger.WithError(err).Error("Failed to delete plants")
			return
		}
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

	// Delete any strains associated with this breeder
	_, err = db.Exec("DELETE FROM strain WHERE breeder_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete strains")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete strains"})
	}

	// Delete breeder from database
	_, err = db.Exec("DELETE FROM breeder WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete breeder")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete breeder"})
		return
	}

	//Reload Config
	config.Breeders = GetBreeders()

	c.JSON(http.StatusOK, gin.H{"message": "Breeder deleted"})
}

func GetStreams() []types.Stream {
	streams := []types.Stream{}
	fieldLogger := logger.Log.WithField("func", "GetStreams")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return streams
	}
	rows, err := db.Query("SELECT s.id, s.name, url, zone_id, visible, z.name as zone_name FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read stream")
		return streams
	}
	defer rows.Close()

	stream := types.Stream{}
	for rows.Next() {
		var id, zoneID uint
		var visible bool
		var name, url, zoneName string
		err = rows.Scan(&id, &name, &url, &zoneID, &visible, &zoneName)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read stream")
			continue
		}
		stream = types.Stream{ID: id, Name: name, URL: url, ZoneID: zoneID, ZoneName: zoneName, Visible: visible}
		streams = append(streams, stream)
	}

	return streams
}

func AddStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddStreamHandler")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  string `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Add stream to database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		return
	}

	// Insert new stream and return new id
	var id int
	err = db.QueryRow("INSERT INTO streams (name, url, zone_id, visible) VALUES ($1, $2, $3, $4) RETURNING id", stream.Name, stream.URL, stream.ZoneID, stream.Visible).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add stream"})
		return
	}

	streams := GetStreams()
	config.Streams = streams

	latestFileName := fmt.Sprintf("stream_%d_latest%s", id, filepath.Ext(".jpg"))
	latestSavePath := filepath.Join("uploads", "streams", latestFileName)
	utils.GrabWebcamImage(stream.URL, latestSavePath)

	c.JSON(http.StatusCreated, gin.H{"id": id, "streams": streams})
}

func UpdateStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateStreamHandler")
	id := c.Param("id")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  int    `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update stream in database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		return
	}

	//convert visible to int
	var visibleInt int
	if stream.Visible {
		visibleInt = 1
	} else {
		visibleInt = 0
	}

	// Update stream in database
	_, err = db.Exec("UPDATE streams SET name = $1, url = $2, zone_id = $3, visible = $4 WHERE id = $5", stream.Name, stream.URL, stream.ZoneID, visibleInt, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update stream"})
		return
	}

	streams := GetStreams()
	config.Streams = streams

	c.JSON(http.StatusOK, gin.H{"message": "Stream updated", "streams": streams})
}

func DeleteStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamHandler")
	id := c.Param("id")

	// Delete stream from database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete stream")
		return
	}

	// Delete stream from database
	_, err = db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete stream")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete stream"})
		return
	}

	streams := GetStreams()
	config.Streams = streams

	c.JSON(http.StatusOK, gin.H{"message": "Stream deleted", "streams": streams})
}

func GetStreamsByZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetStreamsByZoneHandler")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open database"})
		return
	}

	rows, err := db.Query("SELECT s.id, s.name, s.url, z.name as zone_name, visible FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read streams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read streams"})
		return
	}
	defer rows.Close()

	streamsByZone := make(map[string][]types.Stream)
	for rows.Next() {
		var id int
		var name, url, zoneName string
		var visible bool
		err = rows.Scan(&id, &name, &url, &zoneName, &visible)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read streams")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read streams"})
			return
		}
		streamsByZone[zoneName] = append(streamsByZone[zoneName], types.Stream{ID: uint(id), Name: name, URL: url, ZoneName: zoneName, Visible: visible})
	}

	c.JSON(http.StatusOK, streamsByZone)
}

func DeleteStreamByID(id string) error {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamByID")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return err
	}

	_, err = db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor")
		return err
	}
	return nil
}
