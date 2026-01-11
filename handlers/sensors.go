package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
		var show bool // SQLite represents booleans as integers

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
			"visible":   show, // Convert to boolean
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
		// Insert a new zone into the database
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
					unit := "\u00b0F"
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
					unit := "\u00b0F"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 6: //Outside Humidity
					name := "ACI (" + device + ") outside humidity"
					unit := "%"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				case 7: //Outside VPD
					name := "ACI (" + device + ") outside VPD"
					unit := "kPa"
					checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
				default:
					// Unknown sensorType: create a generic sensor so users can see and rename it.
					// Determine a reasonable default unit:
					unit := ""
					// If device-level unit is present: 0 = °F, 1 = °C
					switch deviceData.DeviceInfo.Unit {
					case 0:
						// Fahrenheit default for temperature-like types
						if sensor.SensorType == 1 || sensor.SensorType == 4 || sensor.SensorType == 5 {
							unit = "\u00b0F"
						}
					case 1:
						if sensor.SensorType == 1 || sensor.SensorType == 4 || sensor.SensorType == 5 {
							unit = "\u00b0C"
						}
					}
					// Known percent/humidity-like types
					if sensor.SensorType == 2 || sensor.SensorType == 6 {
						unit = "%"
					}
					name := fmt.Sprintf("ACI (%s) sensor %d.%d", device, sensor.AccessPort, sensor.SensorType)
					// Log for analytics so we can update mappings later
					logger.Log.WithFields(logrus.Fields{"device": device, "accessPort": sensor.AccessPort, "sensorType": sensor.SensorType, "sensorUnit": sensor.SensorUnit}).Info("Discovered unknown ACI sensor type - inserting generic sensor")
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

// prettyMarshalSorted marshals an unmarshaled JSON structure (interface{}) into
// a pretty-printed JSON byte slice where object keys are sorted alphabetically.
// It expects numbers to be json.Number when possible to preserve original formatting.
func prettyMarshalSorted(v interface{}, indent string) ([]byte, error) {
	var buf bytes.Buffer
	// recursive writer
	var writeValue func(interface{}, int) error
	writeIndent := func(level int) {
		for i := 0; i < level; i++ {
			buf.WriteString(indent)
		}
	}

	writeValue = func(val interface{}, level int) error {
		switch t := val.(type) {
		case map[string]interface{}:
			// sort keys
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			buf.WriteString("{")
			if len(keys) > 0 {
				buf.WriteString("\n")
			}
			for i, k := range keys {
				writeIndent(level + 1)
				// write key
				kb, _ := json.Marshal(k)
				buf.Write(kb)
				buf.WriteString(": ")
				if err := writeValue(t[k], level+1); err != nil {
					return err
				}
				if i != len(keys)-1 {
					buf.WriteString(",")
				}
				buf.WriteString("\n")
			}
			if len(keys) > 0 {
				writeIndent(level)
			}
			buf.WriteString("}")
		case []interface{}:
			buf.WriteString("[")
			if len(t) > 0 {
				buf.WriteString("\n")
			}
			for i, item := range t {
				writeIndent(level + 1)
				if err := writeValue(item, level+1); err != nil {
					return err
				}
				if i != len(t)-1 {
					buf.WriteString(",")
				}
				buf.WriteString("\n")
			}
			if len(t) > 0 {
				writeIndent(level)
			}
			buf.WriteString("]")
		case string:
			b, _ := json.Marshal(t)
			buf.Write(b)
		case json.Number:
			buf.WriteString(t.String())
		case float64:
			// fallback for plain float64 values
			buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
		case bool:
			if t {
				buf.WriteString("true")
			} else {
				buf.WriteString("false")
			}
		case nil:
			buf.WriteString("null")
		default:
			// final fallback: use json.Marshal
			b, err := json.Marshal(t)
			if err != nil {
				return err
			}
			buf.Write(b)
		}
		return nil
	}

	if err := writeValue(v, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// New handler: DumpACInfinityJSON
func DumpACInfinityJSON(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DumpACInfinityJSON")
	fieldLogger.Info("Dumping raw AC Infinity JSON response")

	// Determine whether to redact sensitive fields (default: true)
	redact := true
	if v := c.Query("redact"); v != "" {
		if strings.ToLower(v) == "false" || v == "0" {
			redact = false
		}
	}

	if config.ACIToken == "" {
		fieldLogger.Error("ACIToken is not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ACIToken is not configured"})
		return
	}

	url := "http://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + config.ACIToken
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating request"})
		return
	}

	req.Header.Add("token", config.ACIToken)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		c.JSON(http.StatusBadGateway, gin.H{"error": "Error sending request to AC Infinity"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading response body"})
		return
	}

	// Try to unmarshal into a generic structure using UseNumber so we preserve numeric formatting.
	var data interface{}
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		// Not valid JSON: fall back to returning raw body
		fieldLogger.WithError(err).Warn("Response is not valid JSON; returning raw body")
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
		return
	}

	// Keys to redact
	redactKeys := map[string]struct{}{
		"devId":       {},
		"devCode":     {},
		"devMacAddr":  {},
		"appEmail":    {},
		"devTimeZone": {},
		"zoneId":      {},
		"wifiName":    {},
	}

	// recursive redaction
	var redactFunc func(interface{})
	redactFunc = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			for k, val := range t {
				if _, ok := redactKeys[k]; ok {
					// Replace with a standard string marker
					t[k] = "[REDACTED]"
					continue
				}
				// Recurse into nested values
				switch val.(type) {
				case map[string]interface{}, []interface{}:
					redactFunc(val)
				}
			}
		case []interface{}:
			for _, item := range t {
				switch item.(type) {
				case map[string]interface{}, []interface{}:
					redactFunc(item)
				}
			}
		}
	}

	if redact {
		redactFunc(data)
		prettyBytes, err := prettyMarshalSorted(data, "  ")
		if err != nil {
			fieldLogger.WithError(err).Warn("Failed to marshal redacted JSON; returning raw body")
			c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
			return
		}

		c.Data(http.StatusOK, "application/json; charset=utf-8", prettyBytes)
		return
	}

	// Non-redacted: pretty print with sorted keys for consistent ordering
	prettyBytes, err := prettyMarshalSorted(data, "  ")
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to marshal pretty JSON; returning raw body")
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", prettyBytes)
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
		// Insert a new zone into the database
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
	err := db.QueryRow("SELECT id FROM sensors WHERE source = $1 and device = $2 and type = $3", source, device, sensorType).Scan(&sensorid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			//fmt.Println("No rows found")
		} else {
			fieldLogger.WithError(err).Error("Error querying for sensor")
			return
		}
	}
	if sensorid == 0 {
		_, err := db.Exec("INSERT INTO sensors (name, source, device, type, zone_id, unit) VALUES ($1, $2, $3, $4, $5, $6)", name, source, device, sensorType, zoneId, unit)
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

	var id int
	err = db.QueryRow("INSERT INTO zones (name) VALUES ($1) RETURNING id", name).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error inserting new zone")
		return 0, err
	}

	config.Zones = GetZones()

	return id, nil
}
func GetGroupedSensorsWithLatestReading() map[string]map[string][]map[string]interface{} {
	fieldLogger := logger.Log.WithField("func", "GetGroupedSensorsWithLatestReading")
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Check if the cache is still valid
	if time.Since(cacheLastUpdatedTime) < time.Duration(config.PollingInterval/10)*time.Second {
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
AND s.show
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

	_, err = db.Exec("UPDATE sensors SET name = $1, show = $2, zone_id = $3, unit = $4, device = $5 WHERE id = $6",
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
	_, err = db.Exec("DELETE FROM sensor_data WHERE sensor_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor data")
		return err
	}

	_, err = db.Exec("DELETE FROM sensors WHERE id = $1", id)
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
	err = db.QueryRow("SELECT name FROM sensors WHERE id = $1", id).Scan(&name)
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

// SensorDataPayload represents the expected structure of incoming sensor data
type SensorDataPayload struct {
	Source  string  `json:"source" binding:"required"`
	Device  string  `json:"device" binding:"required"`
	Type    string  `json:"type" binding:"required"`
	Value   float64 `json:"value" binding:"required"`
	Name    string  `json:"name"`
	Unit    string  `json:"unit"`
	ZoneID  *int    `json:"zone_id"`
	NewZone string  `json:"new_zone"`
}

// IngestSensorData handles the ingestion of sensor data from external sources
func IngestSensorData(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "IngestSensorData")

	// Check whether API ingest is enabled in config
	if config.APIIngestEnabled == 0 {
		fieldLogger.Warn("API ingest is disabled via settings")
		c.JSON(http.StatusForbidden, gin.H{"error": "API ingest is disabled"})
		return
	}

	var payload SensorDataPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		fieldLogger.WithError(err).Error("Invalid payload")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	// If no name is provided, generate one
	if payload.Name == "" {
		payload.Name = fmt.Sprintf("%s (%s) %s", payload.Source, payload.Device, payload.Type)
	}

	// Handle new zone creation if specified
	if payload.ZoneID == nil && payload.NewZone != "" {
		// Try to find an existing zone with the given new_zone name first
		existingZoneID, err := GetZoneIDByName(payload.NewZone)

		if err == nil && existingZoneID != 0 {
			// If an existing zone is found, use its ID
			payload.ZoneID = &existingZoneID
		} else {
			zoneID, err := CreateNewZone(payload.NewZone)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to create new zone")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
				return
			}
			payload.ZoneID = &zoneID
		}
	}

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// First ensure the sensor exists
	var sensorID int
	err = db.QueryRow(`
        SELECT id FROM sensors
        WHERE source = $1 AND device = $2 AND type = $3`,
		payload.Source, payload.Device, payload.Type).Scan(&sensorID)

	if errors.Is(err, sql.ErrNoRows) {
		// Create a new sensor if it doesn't exist
		err = db.QueryRow(`
            INSERT INTO sensors (name, source, device, type, zone_id, unit, show)
            VALUES ($1, $2, $3, $4, $5, $6, true)
            RETURNING id`,
			payload.Name, payload.Source, payload.Device, payload.Type, payload.ZoneID, payload.Unit).Scan(&sensorID)

		if err != nil {
			fieldLogger.WithError(err).Error("Error creating sensor")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create sensor"})
			return
		}
	} else if err != nil {
		fieldLogger.WithError(err).Error("Error querying sensor")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Insert the sensor reading
	_, err = db.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)",
		sensorID, payload.Value)
	if err != nil {
		fieldLogger.WithError(err).Error("Error inserting sensor data")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save sensor data"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Sensor data ingested successfully",
		"sensor_id": sensorID,
	})
}

func GetZoneIDByName(zoneName string) (int, error) {
	fieldLogger := logger.Log.WithField("func", "GetZoneIDByName")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return 0, err
	}

	var zoneID int
	err = db.QueryRow("SELECT id FROM zones WHERE name = $1", zoneName).Scan(&zoneID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil // No zone found
		}
		fieldLogger.WithError(err).Error("Error querying zone ID")
		return 0, err
	}

	return zoneID, nil
}
