package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"isley/model"
	"isley/model/types"
	"log"
	"net/http"
)

type SensorResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Zone     string `json:"zone"`
	Source   string `json:"source"`
	Device   string `json:"device"`
	Type     string `json:"type"`
	Show     bool   `json:"show"`
	Unit     string `json:"unit"`
	CreateDT string `json:"create_dt"`
	UpdateDT string `json:"update_dt"`
}

func GetSensors() []map[string]interface{} {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Error opening database:", err)
		return nil
	}
	defer db.Close()

	// Query for sensor data
	rows, err := db.Query(`
        SELECT 
            s.id, s.name, z.name AS zone, s.source, s.device, s.type, s.show, s.create_dt, s.update_dt, s.zone_id, s.unit
        FROM sensors s
        LEFT JOIN zones z ON s.zone_id = z.id
        ORDER BY s.source, s.device, s.type, s.name
    `)
	if err != nil {
		fmt.Println("Error querying sensors:", err)
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
			fmt.Println("Error scanning row:", err)
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
	var input struct {
		ZoneID  *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone string `json:"new_zone"` // Optional new zone name
	}
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}
	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	aciToken := ""
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Query settings table and write result to console
	rows, err := db.Query("SELECT * FROM settings where name = 'aci.token'")
	if err != nil {
		fmt.Println(err)
		return
	}
	// Iterate over rows
	for rows.Next() {
		//write row
		var id int
		var name string
		var value string
		var create_dt string
		var update_dt string
		err = rows.Scan(&id, &name, &value, &create_dt, &update_dt)
		if err != nil {
			fmt.Println(err)
			return
		}
		if name == "aci.token" {
			aciToken = value
		}

	}

	fmt.Println("Scanning AC Infinity sensors...")

	url := "http://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + aciToken
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}

	req.Header.Add("token", aciToken)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}

	var jsonResponse types.ACIResponse
	err = json.Unmarshal(respBody, &jsonResponse)
	if err != nil {
		log.Printf("Error unmarshalling JSON: %v", err)
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
				}
			}
		}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "AC Infinity sensors scanned and added"})
}

func ScanEcoWittSensors(c *gin.Context) {
	var input struct {
		ZoneID  *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone string `json:"new_zone"` // Optional new zone name
	}
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}
	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}
	ecServer := ""
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Query settings table and write result to console
	rows, err := db.Query("SELECT * FROM settings where name = 'ec.server'")
	if err != nil {
		fmt.Println(err)
		return
	}
	// Iterate over rows
	for rows.Next() {
		//write row
		var id int
		var name string
		var value string
		var create_dt string
		var update_dt string
		err = rows.Scan(&id, &name, &value, &create_dt, &update_dt)
		if err != nil {
			fmt.Println(err)
			return
		}
		if name == "ec.server" {
			ecServer = value
		}
	}

	fmt.Println("Scanning EcoWitt sensors on server: ", ecServer)

	url := "http://" + ecServer + "/get_livedata_info"
	reqBody := bytes.NewBuffer([]byte(""))
	req, err := http.NewRequest("GET", url, reqBody)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return
	}

	// Parse the JSON into the struct
	var apiResponse types.ECWAPIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	//Add EcoWitt sensors to db
	for _, wh := range apiResponse.WH25 {
		fmt.Printf("Temperature: %s %s, Humidity: %s, Absolute Pressure: %s, Relative Pressure: %s\n",
			wh.InTemp, wh.Unit, wh.InHumi, wh.Abs, wh.Rel)

		sensorType := "WH25.InTemp"
		device := ecServer
		source := "ecowitt"
		name := "EC (" + ecServer + ") InTemp"
		unit := "°F"
		checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

		sensorType = "WH25.InHumi"
		name = "EC (" + ecServer + ") InHumi"
		unit = "%"
		checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)

	}

	//fmt.Println("\nCH Soil Data:")
	for _, ch := range apiResponse.CHSoil {
		fmt.Printf("Channel: %s, Name: %s, Battery: %s, Humidity: %s\n",
			ch.Channel, ch.Name, ch.Battery, ch.Humidity)

		sensorType := "Soil." + ch.Channel
		device := ecServer
		source := "ecowitt"
		name := ch.Name
		unit := "%"
		checkInsertSensor(db, source, device, sensorType, name, input.ZoneID, unit)
	}

	//Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "EcoWitt sensors scanned and added"})
}

func checkInsertSensor(db *sql.DB, source string, device string, sensorType string, name string, zoneId *int, unit string) {
	sensorid := 0
	err := db.QueryRow("SELECT id FROM sensors WHERE source = ? and device = ? and type = ?", source, device, sensorType).Scan(&sensorid)
	if err != nil {
		if err == sql.ErrNoRows {
			//fmt.Println("No rows found")
		} else {
			fmt.Println("Error querying sensors:", err)
			return
		}
	}
	if sensorid == 0 {
		_, err := db.Exec("INSERT INTO sensors (name, source, device, type, zone_id, unit) VALUES (?, ?, ?, ?, ?, ?)", name, source, device, sensorType, zoneId, unit)
		if err != nil {
			log.Printf("Error writing to db: %v", err)
			return
		}
	}
}

type Zone struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func GetZones() []Zone {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer db.Close()

	var zones []Zone
	rows, err := db.Query("SELECT id, name FROM zones")
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var zone Zone
		if err := rows.Scan(&zone.ID, &zone.Name); err != nil {
			fmt.Println(err)
			continue
		}
		zones = append(zones, zone)
	}

	return zones
}

func CreateNewZone(name string) (int, error) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return 0, err
	}
	defer db.Close()

	result, err := db.Exec("INSERT INTO zones (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}
func GetGroupedSensorsWithLatestReading() map[string]map[string][]map[string]interface{} {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query(`
        SELECT 
            s.id,
            z.name AS zone_name, 
            s.device, 
            s.type, 
            s.name, 
            sd.value, 
            s.unit
        FROM sensors s
        JOIN zones z ON s.zone_id = z.id
        LEFT JOIN sensor_data sd ON s.id = sd.sensor_id
        WHERE sd.id IN (
            SELECT MAX(id) FROM sensor_data GROUP BY sensor_id
        )
        AND s.show = 1
        ORDER BY z.name, s.device, s.type
    `)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	grouped := make(map[string]map[string][]map[string]interface{})

	for rows.Next() {
		var zoneName, device, sensorType, sensorName, unit string
		var value float64
		var id int

		if err := rows.Scan(&id, &zoneName, &device, &sensorType, &sensorName, &value, &unit); err != nil {
			fmt.Println(err)
			continue
		}

		// Initialize grouping maps if necessary
		if _, ok := grouped[zoneName]; !ok {
			grouped[zoneName] = make(map[string][]map[string]interface{})
		}

		grouped[zoneName][device] = append(grouped[zoneName][device], map[string]interface{}{
			"type":  sensorType,
			"name":  sensorName,
			"value": value,
			"id":    id,
			"unit":  unit,
		})
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return grouped
	}

	return grouped
}

func GetGroupedSensors() map[string]map[string]map[string][]SensorResponse {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer db.Close()

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
		fmt.Println(err)
		return nil
	}
	defer rows.Close()

	grouped := make(map[string]map[string]map[string][]SensorResponse)

	for rows.Next() {
		var sensor SensorResponse
		var zoneName, deviceType string

		if err := rows.Scan(&sensor.Name, &zoneName, &sensor.Device, &deviceType, &sensor.Source, &sensor.Unit); err != nil {
			fmt.Println(err)
			continue
		}

		// Initialize grouping maps if necessary
		if _, ok := grouped[zoneName]; !ok {
			grouped[zoneName] = make(map[string]map[string][]SensorResponse)
		}
		if _, ok := grouped[zoneName][sensor.Device]; !ok {
			grouped[zoneName][sensor.Device] = make(map[string][]SensorResponse)
		}

		// Add sensor to the appropriate group
		grouped[zoneName][sensor.Device][deviceType] = append(grouped[zoneName][sensor.Device][deviceType], sensor)
	}

	return grouped
}

func EditSensor(c *gin.Context) {
	var input struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Visible bool   `json:"visible"`
		ZoneID  int    `json:"zone_id"`
		Unit    string `json:"unit"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	_, err = db.Exec("UPDATE sensors SET name = ?, show = ?, zone_id = ?, unit = ? WHERE id = ?",
		input.Name, input.Visible, input.ZoneID, input.Unit, input.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update sensor"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor updated successfully"})
}

func DeleteSensor(c *gin.Context) {
	sensorID := c.Param("id")

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()
	_, err = db.Exec("DELETE FROM sensors WHERE id = ?", sensorID)
	if err != nil {
		fmt.Println("Error deleting sensor:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sensor"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor deleted successfully"})
}
