package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"isley/config"
	model "isley/model"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Settings struct {
	ACI struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"`
	} `json:"aci"`
	EC struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
	} `json:"ec"`
	PollingInterval string `json:"polling_interval"`
}

type ACInfinitySettings struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

type EcoWittSettings struct {
	Enabled bool `json:"enabled"`
}

type SettingsData struct {
	ACI             ACInfinitySettings `json:"aci"`
	EC              EcoWittSettings    `json:"ec"`
	PollingInterval int                `json:"polling_interval"`
}

func SaveSettings(c *gin.Context) {
	var settings Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		fmt.Println("Failed to save settings", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Save settings logic (e.g., to a database or config file)
	//fmt.Printf("Received settings: %+v\n", settings)
	if settings.ACI.Enabled {
		err := UpdateSetting("aci.enabled", "1")
		if err != nil {
			fmt.Println("Failed to save settings", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ACIEnabled = 1
		}
	} else {
		err := UpdateSetting("aci.enabled", "0")
		if err != nil {
			fmt.Println("Failed to save settings", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ACIEnabled = 0
		}
	}
	err := UpdateSetting("aci.token", settings.ACI.Token)
	if err != nil {
		fmt.Println("Failed to save settings", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	} else {
		config.ACIToken = settings.ACI.Token
	}
	if settings.EC.Enabled {
		err = UpdateSetting("ec.enabled", "1")
		if err != nil {
			fmt.Println("Failed to save settings", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ECEnabled = 1
		}
	} else {
		err = UpdateSetting("ec.enabled", "0")
		if err != nil {
			fmt.Println("Failed to save settings", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
			return
		} else {
			config.ECEnabled = 0
		}
	}
	err = UpdateSetting("polling_interval", settings.PollingInterval)
	if err != nil {
		fmt.Println("Failed to save settings", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save settings"})
		return
	} else {
		config.PollingInterval, _ = strconv.Atoi(settings.PollingInterval)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings saved successfully"})
}

func UpdateSetting(name string, value string) error {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to open database", err)
		return err
	}
	existId := 0
	// Query settings table and write result to console
	rows, err := db.Query("SELECT * FROM settings where name = $1", name)
	if err != nil {
		fmt.Println("Failed to read settings", err)
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
			fmt.Println("Failed to read settings", err)
		}
		existId = id
	}

	if existId == 0 {
		//Insert new setting
		_, err = db.Exec("INSERT INTO settings (name, value) VALUES ($1, $2)", name, value)
		if err != nil {
			fmt.Println("Failed to insert setting", err)
		}
	} else {
		//Update existing setting
		_, err = db.Exec("UPDATE settings SET value = $1 WHERE id = $2", value, existId)
		if err != nil {
			fmt.Println("Failed to update setting", err)
		}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println("Failed to close database", err)
		return err
	}

	return nil
}

func GetSettings() SettingsData {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to read settings", err)
		return SettingsData{}
	}
	defer db.Close()

	settingsData := SettingsData{}

	rows, err := db.Query("SELECT * FROM settings")
	if err != nil {
		fmt.Println("Failed to read settings", err)
		return SettingsData{}
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, value, createDt, updateDt string
		err = rows.Scan(&id, &name, &value, &createDt, &updateDt)
		if err != nil {
			fmt.Println("Failed to read settings", err)
			continue
		}

		switch name {
		case "aci.enabled":
			settingsData.ACI.Enabled = value == "1"
		case "aci.token":
			settingsData.ACI.Token = value
		case "ec.enabled":
			settingsData.EC.Enabled = value == "1"
		case "polling_interval":
			settingsData.PollingInterval, _ = strconv.Atoi(value)
		}
	}

	return settingsData
}
func AddZoneHandler(c *gin.Context) {
	var zone struct {
		Name string `json:"zone_name"`
	}
	if err := c.ShouldBindJSON(&zone); err != nil {
		fmt.Println("Failed to add zone", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Add zone to database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to add zone", err)
		return
	}
	defer db.Close()

	// Insert new zone and return new id
	var id int
	err = db.QueryRow("INSERT INTO zones (name) VALUES ($1) RETURNING id", zone.Name).Scan(&id)
	if err != nil {
		fmt.Println("Failed to add zone", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add zone"})
		return
	}
	//Add the new zone to the config
	config.Zones = append(config.Zones, config.ZoneResponse{ID: uint(id), Name: zone.Name})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func AddMetricHandler(c *gin.Context) {
	var metric struct {
		Name string `json:"metric_name"`
		Unit string `json:"metric_unit"`
	}

	if err := c.ShouldBindJSON(&metric); err != nil {
		fmt.Println("Failed to add metric", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// metric name of "Height" is reserved
	if metric.Name == "Height" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This metric name is reserved and can't be added."})
		return
	}

	// Add metric to database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to add metric", err)
		return
	}
	defer db.Close()

	// Insert new metric and return new id
	var id int
	err = db.QueryRow("INSERT INTO metric (name, unit) VALUES ($1, $2) RETURNING id", metric.Name, metric.Unit).Scan(&id)
	if err != nil {
		fmt.Println("Failed to add metric", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add metric"})
		return
	}
	config.Metrics = append(config.Metrics, config.MetricResponse{ID: id, Name: metric.Name, Unit: metric.Unit})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func AddActivityHandler(c *gin.Context) {
	var activity struct {
		Name string `json:"activity_name"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fmt.Println("Failed to add activity", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	//Reserved names can't be added "Water", "Feed", "Note"
	if activity.Name == "Water" || activity.Name == "Feed" || activity.Name == "Note" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This activity name is reserved and can't be added."})
		return
	}

	// Add activity to database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to add activity", err)
		return
	}
	defer db.Close()

	// Insert new activity and return new id
	var id int
	err = db.QueryRow("INSERT INTO activity (name) VALUES ($1) RETURNING id", activity.Name).Scan(&id)
	if err != nil {
		fmt.Println("Failed to add activity", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add activity"})
		return
	}
	config.Activities = append(config.Activities, config.ActivityResponse{ID: id, Name: activity.Name})

	c.JSON(http.StatusCreated, gin.H{"id": id})
}
func UpdateZoneHandler(c *gin.Context) {
	id := c.Param("id")
	var zone struct {
		Name string `json:"zone_name"`
	}
	if err := c.ShouldBindJSON(&zone); err != nil {
		fmt.Println("Failed to update zone", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update zone in database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to update zone", err)
		return
	}
	defer db.Close()

	// Update zone in database
	_, err = db.Exec("UPDATE zones SET name = $1 WHERE id = $2", zone.Name, id)
	if err != nil {
		fmt.Println("Failed to update zone", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update zone"})
		return
	}
	//Reload Config
	config.Zones = GetZones()

	c.JSON(http.StatusOK, gin.H{"message": "Zone updated"})
}

func UpdateMetricHandler(c *gin.Context) {
	id := c.Param("id")
	var metric struct {
		Name string `json:"metric_name"`
		Unit string `json:"metric_unit"`
	}
	if err := c.ShouldBindJSON(&metric); err != nil {
		fmt.Println("Failed to update metric", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update metric in database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to update metric", err)
		return
	}
	defer db.Close()

	// Check if metric exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM metric WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fmt.Println("Failed to update metric", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
		return
	}
	if lock {
		//Update the unit only
		_, err = db.Exec("UPDATE metric SET unit = $1 WHERE id = $2", metric.Unit, id)
		if err != nil {
			fmt.Println("Failed to update metric", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{"error": "Editing this metric is not allowed, only unit changed."})
		return
	}

	// Update metric in database
	_, err = db.Exec("UPDATE metric SET name = $1, unit = $2 WHERE id = $3", metric.Name, metric.Unit, id)
	if err != nil {
		fmt.Println("Failed to update metric", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update metric"})
		return
	}

	//Reload Config
	config.Metrics = GetMetrics()

	c.JSON(http.StatusOK, gin.H{"message": "Metric updated"})
}

func UpdateActivityHandler(c *gin.Context) {
	id := c.Param("id")
	var activity struct {
		Name string `json:"activity_name"`
	}
	if err := c.ShouldBindJSON(&activity); err != nil {
		fmt.Println("Failed to update activity", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	// Update activity in database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to update activity", err)
		return
	}
	defer db.Close()

	// Check if activity exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fmt.Println("Failed to update activity", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}
	if lock {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Editing this activity is not allowed."})
		return
	}

	// Update activity in database
	_, err = db.Exec("UPDATE activity SET name = $1 WHERE id = $2", activity.Name, id)
	if err != nil {
		fmt.Println("Failed to update activity", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	//Reload Config
	config.Activities = GetActivities()

	c.JSON(http.StatusOK, gin.H{"message": "Activity updated"})
}
func DeleteZoneHandler(c *gin.Context) {
	id := c.Param("id")

	// Delete zone from database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Failed to delete zone", err)
		return
	}
	defer db.Close()

	//Build a list of plants associated with this zoen to delete first
	rows, err := db.Query("SELECT id FROM plant WHERE zone_id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete plants", err)
		return
	}
	defer rows.Close()

	plantList := []int{}
	for rows.Next() {
		var plantId int
		err = rows.Scan(&plantId)
		if err != nil {
			fmt.Println("Failed to delete plant", err)
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
		fmt.Println("Failed to delete sensors", err)
		return
	}
	defer rows.Close()

	sensorList := []int{}
	for rows.Next() {
		var sensorId int
		err = rows.Scan(&sensorId)
		if err != nil {
			fmt.Println("Failed to delete sensor", err)
			continue
		}
		sensorList = append(sensorList, sensorId)
	}

	for _, sensorId := range sensorList {
		DeleteSensorByID(fmt.Sprintf("%d", sensorId))
	}

	// Delete zone from database
	_, err = db.Exec("DELETE FROM zones WHERE id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete zone", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete zone"})
		return
	}

	//Reload Config
	config.Zones = GetZones()

	c.JSON(http.StatusOK, gin.H{"message": "Zone deleted"})
}

func DeleteMetricHandler(c *gin.Context) {
	id := c.Param("id")

	// Delete metric from database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// Check if metric exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM metric WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fmt.Println("Failed to delete metric", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete metric"})
		return
	}
	if lock {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting this metric is not allowed."})
		return
	}

	// Delete any measurements associated with this metric
	_, err = db.Exec("DELETE FROM plant_measurements WHERE metric_id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete measurements", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete measurements"})
		return
	}

	// Delete metric from database
	_, err = db.Exec("DELETE FROM metric WHERE id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete metric", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete metric"})
		return
	}

	//Reload Config
	config.Metrics = GetMetrics()

	c.JSON(http.StatusOK, gin.H{"message": "Metric deleted"})
}

func DeleteActivityHandler(c *gin.Context) {
	id := c.Param("id")

	// Delete activity from database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// Check if activity exists and lock = TRUE
	var lock bool
	err = db.QueryRow("SELECT lock FROM activity WHERE id = $1", id).Scan(&lock)
	if err != nil {
		fmt.Println("Failed to delete activity", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}
	if lock {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deleting this activity is not allowed."})
		return
	}

	// Delete any plant_activities associated with this activity
	_, err = db.Exec("DELETE FROM plant_activity WHERE activity_id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete plant_activities", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete plant_activities"})
		return
	}

	// Delete activity from database
	_, err = db.Exec("DELETE FROM activity WHERE id = $1", id)
	if err != nil {
		fmt.Println("Failed to delete activity", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	//Reload Config
	config.Activities = GetActivities()

	c.JSON(http.StatusOK, gin.H{"message": "Activity deleted"})
}

func GetSetting(name string) (string, error) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return "", err
	}
	defer db.Close()

	var value string
	err = db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&value)
	if err != nil {
		return "", err
	}

	return value, nil
}
func UploadLogo(c *gin.Context) {
	// Parse the multipart form data
	err := c.Request.ParseMultipartForm(10 << 20) // Limit to 10 MB
	if err != nil {
		log.Println("Error parsing form data:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Retrieve the file from the "logo" field
	fileHeader, err := c.FormFile("logo")
	if err != nil {
		log.Println("Error retrieving file:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to retrieve file"})
		return
	}

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		log.Println("Error opening file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer file.Close()

	// Generate a unique file path
	timestamp := time.Now().UnixNano()
	fileName := fmt.Sprintf("logo_image_%d%s", timestamp, filepath.Ext(fileHeader.Filename))
	savePath := filepath.Join("uploads", "logos", fileName)

	// Create the uploads/logos directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
	if err != nil {
		log.Println("Error creating directory:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
		return
	}

	// Save the file to the filesystem
	out, err := os.Create(savePath)
	if err != nil {
		log.Println("Error creating file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		log.Println("Error saving file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Update the database with the new logo path
	err = UpdateSetting("logo_image", fileName)
	if err != nil {
		log.Println("Error updating logo setting:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update logo setting"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logo uploaded successfully", "path": savePath})
}

func LoadEcDevices() ([]string, error) {
	var ecDevices []string
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return ecDevices, err
	}

	//Iterate over sensors table, looking for distinct device with type ecowitt
	rows, err := db.Query("SELECT DISTINCT device FROM sensors WHERE source = 'ecowitt'")
	if err != nil {
		fmt.Println(err)
		return ecDevices, err
	}
	//build a list of devices to scan

	for rows.Next() {
		var device string
		err = rows.Scan(&device)
		if err != nil {
			fmt.Println(err)
			return ecDevices, err
		}
		ecDevices = append(ecDevices, device)
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return ecDevices, err
	}

	return ecDevices, nil
}
