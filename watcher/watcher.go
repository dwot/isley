package watcher

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"isley/model"
	"isley/model/types"
	"log"
	"net/http"
	"strconv"
	"time"
)

func Watch() {
	fmt.Println("Started Sensor Watcher")

	for {
		aciEnabled := 0
		ecEnabled := 0
		aciToken := ""
		pollingInterval := 60

		// Init the db
		db, err := sql.Open("sqlite", model.DbPath())
		if err != nil {
			fmt.Println(err)
			return
		}

		// Query settings table and write result to console
		rows, err := db.Query("SELECT * FROM settings")
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
			if name == "aci.enabled" {
				iValue, err := strconv.Atoi(value)
				if err == nil {
					aciEnabled = iValue
				}
			}
			if name == "ec.enabled" {
				iValue, err := strconv.Atoi(value)
				if err == nil {
					ecEnabled = iValue
				}
			}
			if name == "aci.token" {
				aciToken = value
			}
			if name == "polling_interval" {
				iValue, err := strconv.Atoi(value)
				if err == nil {
					pollingInterval = iValue
				}
			}
		}

		//Iterate over sensors table, looking for distinct device with type ecowitt
		rows, err = db.Query("SELECT DISTINCT device FROM sensors WHERE source = 'ecowitt'")
		if err != nil {
			fmt.Println(err)
			return
		}
		//build a list of devices to scan
		var ecDevices []string
		for rows.Next() {
			var device string
			err = rows.Scan(&device)
			if err != nil {
				fmt.Println(err)
				return
			}
			ecDevices = append(ecDevices, device)
		}

		// Close the db
		err = db.Close()
		if err != nil {
			fmt.Println(err)
			return
		}
		if aciEnabled == 1 && aciToken != "" {
			updateACISensorData(aciToken)
		}
		if ecEnabled == 1 && len(ecDevices) > 0 {
			for _, ecServer := range ecDevices {
				updateEcoWittSensorData(ecServer)
			}
		}
		time.Sleep(time.Duration(pollingInterval) * time.Second)

	}

}

func updateEcoWittSensorData(server string) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return
	}
	defer db.Close()
	currentDate := time.Now().In(time.Local)
	fmt.Printf("Updating EC sensor data %v \n", currentDate)
	url := "http://" + server + "/get_livedata_info"
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

	device := server
	source := "ecowitt"
	var m = make(map[string]string)
	//Read the onboard sensors
	for _, wh := range apiResponse.WH25 {
		sensorType := "WH25.InTemp"
		//convert to wh.InTemp float64 and store in value
		value := wh.InTemp
		m[sensorType] = value

		sensorType = "WH25.InHumi"
		value = wh.InHumi[:len(wh.InHumi)-1]
		m[sensorType] = value
	}

	for _, ch := range apiResponse.CHSoil {
		sensorType := "Soil." + ch.Channel
		value := ch.Humidity[:len(ch.Humidity)-1]
		m[sensorType] = value
	}

	// Write to db
	for key, value := range m {
		addSensorData(source, device, key, value)
	}

}

func updateACISensorData(token string) {
	currentDate := time.Now().In(time.Local)
	fmt.Printf("Updating ACI sensor data %v \n", currentDate)
	url := "http://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + token
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return
	}

	req.Header.Add("token", token)
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

	source := "acinfinity"
	if len(jsonResponse.Data) > 0 {
		for _, deviceData := range jsonResponse.Data {
			var m = make(map[string]float64)

			device := deviceData.DevCode
			sensorType := "ACI.tempF"
			value := float64(deviceData.DeviceInfo.TemperatureF) / 100.0
			m[sensorType] = value

			sensorType = "ACI.humidity"
			value = float64(deviceData.DeviceInfo.Humidity) / 100.0
			m[sensorType] = value

			for _, sensor := range deviceData.DeviceInfo.Sensors {
				sensorType := fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)
				value := float64(sensor.SensorData) / 100.0
				m[sensorType] = value
			}

			// Write to db
			for key, value := range m {
				strValue := fmt.Sprintf("%f", value)
				addSensorData(source, device, key, strValue)
			}

		}
	}
}

func addSensorData(source string, device string, key string, value string) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return
	}

	// select sensor id by querying sensor table using key
	var sensorid int
	err = db.QueryRow("SELECT id FROM sensors WHERE source = ? and device = ? and type = ?", source, device, key).Scan(&sensorid)
	if err != nil {
		//log.Printf("Error querying sensor id for source: %v, device: %v, type: %v, %v", source, device, key, err)
		return
	} else {
		_, err = db.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES (?, ?)", sensorid, value)
		if err != nil {
			log.Printf("Error writing to db: %v", err)
			return
		}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

}
