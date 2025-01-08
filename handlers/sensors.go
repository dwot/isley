package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"
)

var (
	sensorCache          map[string]map[string][]map[string]interface{}
	cacheLastUpdatedTime time.Time
	cacheMutex           sync.Mutex
)

func GetSensors() []map[string]interface{} {
	fieldLogger := logger.Log.WithField("func", "GetSensors")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return nil
	}

	// Query for sensor data
	rows, err := db.Query(`
        SELECT 
            s.id, s.name, z.name AS zone, s.source, s.device, s.type, s.show, s.create_dt, s.update_dt, s.zone_id, s.unit
        FROM sensors s
        LEFT JOIN zones z ON s.zone_id = z.id
        ORDER BY s.source, s.device, s.type, s.name
    `)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensors")
		return nil
	}
	defer rows.Close()

	// Prepare the data structure
	var sensors []map[string]interface{}

	for rows.Next() {
		var id, zoneId int
		var name, zone, source, device, sensorType, createDT, updateDT, unit string
		var show int // SQLite represents booleans as integers

		// Scan the row data
		err := rows.Scan(&id, &name, &zone, &source, &device, &sensorType, &show, &createDT, &updateDT, &zoneId, &unit)
		if err != nil {
			fieldLogger.WithError(err).Error("Error scanning row")
			continue
		}

		// Build a map for each sensor
		sensors = append(sensors, map[string]interface{}{
			"id":        id,
			"name":      name,
			"zone":      zone,
			"source":    source,
			"device":    device,
			"type":      sensorType,
			"visible":   show == 1, // Convert to boolean
			"create_dt": createDT,
			"update_dt": updateDT,
			"zone_id":   zoneId,
			"unit":      unit,
		})
	}

	return sensors
}

func ScanACInfinitySensors(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "ScanACInfinitySensors")
	var input struct {
		ZoneID  *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone string `json:"new_zone"` // Optional new zone name
	}
	fieldLogger = fieldLogger.WithField("input", input)
	fieldLogger.Info("Scanning AC Infinity sensors")
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}
	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return
	}

	// Query settings table and write result to console
	url := "http://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + config.ACIToken
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		return
	}

	req.Header.Add("token", config.ACIToken)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		return
	}

	var jsonResponse types.ACIResponse
	err = json.Unmarshal(respBody, &jsonResponse)
	if err != nil {
		fieldLogger.WithError(err).Error("Error unmarshalling JSON response")
		return
	}

	if len(jsonResponse.Data) > 0 {
		for _, deviceData := range jsonResponse.Data {
			device := deviceData.DevCode
			source := "acinfinity"
			sensorType := "ACI.tempF"
			name := "AC Infinity (" + device + ") Temp"
			unit := "°F"

			//Check to see if exists by type / device / source combo
			checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

			sensorType = "ACI.humidity"
			name = "AC Infinity (" + device + ") Humidity"
			unit = "%"
			checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

			sensorType = "ACI.tempC"
			name = "AC Infinity (" + device + ") Temp"
			unit = "°C"
			checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

			for _, sensor := range deviceData.DeviceInfo.Sensors {
				sensorType := fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)
				switch sensor.SensorType {
				case 0: //Inside Temp
					name := "ACI (" + device + ") inside temp"
					unit := "°F"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 2: //Inside Humidity
					name := "ACI (" + device + ") inside humidity"
					unit := "%"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 3: //Inside VPD
					name := "ACI (" + device + ") inside VPD"
					unit := "kPa"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 4: //Outside Temp
					name := "ACI (" + device + ") outside temp"
					unit := "°F"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 6: //Outside Humidity
					name := "ACI (" + device + ") outside humidity"
					unit := "%"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 7: //Outside VPD
					name := "ACI (" + device + ") outside VPD"
					unit := "kPa"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				}
			}

			for _, port := range deviceData.DeviceInfo.Ports {
				sensorType := fmt.Sprintf("ACIP.%d", port.Port)
				name := port.PortName
				unit := "%"
				checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "AC Infinity sensors scanned and added"})
}

func ScanEcoWittSensors(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "ScanEcoWittSensors")
	var input struct {
		ZoneID        *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone       string `json:"new_zone"` // Optional new zone name
		ServerAddress string `json:"server_address"`
	}
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}
	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return
	}

	url := "http://" + input.ServerAddress + "/get_livedata_info"

	// Validate the input server address
	if !ValidateServerAddress(input.ServerAddress) {
		fieldLogger.Error("Invalid server address")
		return
	}

	reqBody := bytes.NewBuffer([]byte(""))
	req, err := http.NewRequest("GET", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		return
	}

	// Create a restricted HTTP client
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		return
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		return
	}

	// Parse the JSON into the struct
	var apiResponse types.ECWAPIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		fieldLogger.WithError(err).Error("Error unmarshalling JSON response")
		return
	}

	//Add EcoWitt sensors to db
	sensorType := "WH25.InTemp"
	device := input.ServerAddress
	source := "ecowitt"
	name := "EC (" + input.ServerAddress + ") InTemp"
	unit := "°F"
	checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

	sensorType = "WH25.InHumi"
	name = "EC (" + input.ServerAddress + ") InHumi"
	unit = "%"
	checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

	for _, ch := range apiResponse.CHSoil {
		sensorType := "Soil." + ch.Channel
		device := input.ServerAddress
		source := "ecowitt"
		name := ch.Name
		unit := "%"
		checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
	}
	//Update ECOWitt sensors
	//Set ECDevices
	strECDevices, err := LoadEcDevices()
	if err == nil {
		config.ECDevices = strECDevices
	} else {
		fieldLogger.WithError(err).Error("Error loading EC devices")
	}

	c.JSON(http.StatusOK, gin.H{"message": "EcoWitt sensors scanned and added"})
}

func checkInsertSensor(db *sql.DB, source string, device string, sensorType string, name string, zoneId *int, unit string) {
	fieldLogger := logger.Log.WithField("func", "checkInsertSensor")
	sensorid := 0
	err := db.QueryRow("SELECT id FROM sensors WHERE source = ? and device = ? and type = ?", source, device, sensorType).Scan(&sensorid)
	if err != nil {
		if err == sql.ErrNoRows {
			//fmt.Println("No rows found")
		} else {
			fieldLogger.WithError(err).Error("Error querying for sensor")
			return
		}
	}
	if sensorid == 0 {
		_, err := db.Exec("INSERT INTO sensors (name, source, device, type, zone_id, unit) VALUES (?, ?, ?, ?, ?, ?)", name, source, device, sensorType, zoneId, unit)
		if err != nil {
			fieldLogger.WithError(err).Error("Error inserting sensor")
			return
		}
	}
}

func GetZones() []types.Zone {
	fieldLogger := logger.Log.WithField("func", "GetZones")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return nil
	}

	var zones []types.Zone
	rows, err := db.Query("SELECT id, name FROM zones")
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying zones")
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var zone types.Zone
		if err := rows.Scan(&zone.ID, &zone.Name); err != nil {
			fieldLogger.WithError(err).Error("Error scanning row")
			continue
		}
		zones = append(zones, zone)
	}

	return zones
}

func CreateNewZone(name string) (int, error) {
	fieldLogger := logger.Log.WithField("func", "CreateNewZone")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return 0, err
	}

	result, err := db.Exec("INSERT INTO zones (name) VALUES (?)", name)
	if err != nil {
		fieldLogger.WithError(err).Error("Error inserting new zone")
		return 0, err
	}

	config.Zones = GetZones()

	id, err := result.LastInsertId()
	if err != nil {
		fieldLogger.WithError(err).Error("Error getting last insert ID")
		return 0, err
	}
	return int(id), nil
}
func GetGroupedSensorsWithLatestReading() map[string]map[string][]map[string]interface{} {
	fieldLogger := logger.Log.WithField("func", "GetGroupedSensorsWithLatestReading")
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Check if the cache is still valid
	if time.Since(cacheLastUpdatedTime) < time.Duration(config.PollingInterval)*time.Second {
		return sensorCache
	}

	// Refresh the cache
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return nil
	}

	rows, err := db.Query(`SELECT 
    s.id AS sensor_id,
    z.name AS zone_name,
    s.device,
    s.type,
    s.name,
    sd.value AS current_value,
    s.unit,
    --ra.avg_value AS rolling_avg,
    CASE
        WHEN sd.value > ra.avg_value THEN 'up'
        WHEN sd.value < ra.avg_value THEN 'down'
        ELSE 'flat'
    END AS trend
FROM sensors s
JOIN zones z ON s.zone_id = z.id
JOIN sensor_data sd ON s.id = sd.sensor_id
LEFT JOIN rolling_averages ra ON ra.sensor_id = s.id AND ra.create_dt = sd.create_dt
WHERE sd.id = (
    SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id
)
AND s.show = 1
ORDER BY z.name, s.device, s.type;

`)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensors")
		return nil
	}
	defer rows.Close()

	// Create a new cache map
	newCache := make(map[string]map[string][]map[string]interface{})

	for rows.Next() {
		var zoneName, device, sensorType, sensorName, unit string
		var value float64
		var id int
		var trend string

		if err := rows.Scan(&id, &zoneName, &device, &sensorType, &sensorName, &value, &unit, &trend); err != nil {
			fieldLogger.WithError(err).Error("Error scanning row")
			continue
		}

		// Initialize grouping maps if necessary
		if _, ok := newCache[zoneName]; !ok {
			newCache[zoneName] = make(map[string][]map[string]interface{})
		}

		newCache[zoneName][device] = append(newCache[zoneName][device], map[string]interface{}{
			"type":  sensorType,
			"name":  sensorName,
			"value": value,
			"id":    id,
			"unit":  unit,
			"trend": trend, // "up","down","flat"
		})
	}

	// Update the global cache and timestamp
	sensorCache = newCache
	cacheLastUpdatedTime = time.Now().In(time.Local)

	return sensorCache
}

func GetGroupedSensors() map[string]map[string]map[string][]types.Sensor {
	fieldLogger := logger.Log.WithField("func", "GetGroupedSensors")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return nil
	}

	rows, err := db.Query(`
        SELECT 
            s.name, 
            z.name AS zone_name, 
            s.device, 
            s.type, 
            s.source, 
            s.unit
        FROM sensors s
        JOIN zones z ON s.zone_id = z.id
        WHERE s.show = 1
        ORDER BY z.name, s.device, s.type, s.name
    `)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensors")
		return nil
	}
	defer rows.Close()

	grouped := make(map[string]map[string]map[string][]types.Sensor)

	for rows.Next() {
		var sensor types.Sensor
		var zoneName, deviceType string

		if err := rows.Scan(&sensor.Name, &zoneName, &sensor.Device, &deviceType, &sensor.Source, &sensor.Unit); err != nil {
			fieldLogger.WithError(err).Error("Error scanning row")
			continue
		}

		// Initialize grouping maps if necessary
		if _, ok := grouped[zoneName]; !ok {
			grouped[zoneName] = make(map[string]map[string][]types.Sensor)
		}
		if _, ok := grouped[zoneName][sensor.Device]; !ok {
			grouped[zoneName][sensor.Device] = make(map[string][]types.Sensor)
		}

		// Add sensor to the appropriate group
		grouped[zoneName][sensor.Device][deviceType] = append(grouped[zoneName][sensor.Device][deviceType], sensor)
	}

	return grouped
}

func EditSensor(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "EditSensor")
	var input struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Device  string `json:"device"`
		Visible bool   `json:"visible"`
		ZoneID  int    `json:"zone_id"`
		Unit    string `json:"unit"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return
	}

	_, err = db.Exec("UPDATE sensors SET name = ?, show = ?, zone_id = ?, unit = ?, device = ? WHERE id = ?",
		input.Name, input.Visible, input.ZoneID, input.Unit, input.Device, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Error updating sensor")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update sensor"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor updated successfully"})
}

func DeleteSensor(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteSensor")
	sensorID := c.Param("id")

	err := DeleteSensorByID(sensorID)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sensor"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor deleted successfully"})
}

func DeleteSensorByID(id string) error {
	fieldLogger := logger.Log.WithField("func", "DeleteSensorByID")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return err
	}

	// Delete sensor_data for this sensor
	_, err = db.Exec("DELETE FROM sensor_data WHERE sensor_id = ?", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor data")
		return err
	}

	_, err = db.Exec("DELETE FROM sensors WHERE id = ?", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor")
		return err
	}
	return nil
}

func GetSensorName(id string) string {
	fieldLogger := logger.Log.WithField("func", "GetSensorName")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return ""
	}

	var name string
	err = db.QueryRow("SELECT name FROM sensors WHERE id = ?", id).Scan(&name)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensor name")
		return ""
	}

	return name
}

// ValidateServerAddress ensures the input is a valid private IP or hostname
func ValidateServerAddress(address string) bool {
	// Check if it's a valid IP
	ip := net.ParseIP(address)
	if ip != nil {
		// Ensure the IP is private
		if ip.IsLoopback() || ip.IsPrivate() {
			return true
		}
		return false
	}

	// Check if it's a valid hostname (local hostnames only)
	validHostname := regexp.MustCompile(`^([a-zA-Z0-9_-]+\.)*[a-zA-Z0-9_-]+$`).MatchString
	return validHostname(address)
}
