package watcher

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"net/http"
	"time"
)

func Watch() {
	logger.Log.Info("Started Sensor Watcher")

	for {
		if config.ACIEnabled == 1 && config.ACIToken != "" {
			updateACISensorData(config.ACIToken)
		}
		if config.ECEnabled == 1 && len(config.ECDevices) > 0 {
			for _, ecServer := range config.ECDevices {
				updateEcoWittSensorData(ecServer)
			}
		}
		time.Sleep(time.Duration(config.PollingInterval) * time.Second)
	}
}

func updateEcoWittSensorData(server string) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return
	}
	defer db.Close()

	currentDate := time.Now().In(time.Local)
	logger.Log.WithField("timestamp", currentDate).Info("Updating EC sensor data")

	url := "http://" + server + "/get_livedata_info"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating EcoWitt request")
		return
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Error sending EcoWitt request")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Error reading EcoWitt response body")
		return
	}

	var apiResponse types.ECWAPIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		logger.Log.WithError(err).Error("Error parsing EcoWitt JSON response")
		return
	}

	device := server
	source := "ecowitt"
	dataMap := map[string]string{}

	for _, wh := range apiResponse.WH25 {
		dataMap["WH25.InTemp"] = wh.InTemp
		dataMap["WH25.InHumi"] = wh.InHumi[:len(wh.InHumi)-1]
	}

	for _, ch := range apiResponse.CHSoil {
		dataMap["Soil."+ch.Channel] = ch.Humidity[:len(ch.Humidity)-1]
	}

	for key, value := range dataMap {
		addSensorData(source, device, key, value)
	}
}

func updateACISensorData(token string) {
	currentDate := time.Now().In(time.Local)
	logger.Log.WithField("timestamp", currentDate).Info("Updating ACI sensor data")

	url := "http://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + token
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		logger.Log.WithError(err).Error("Error creating ACI request")
		return
	}

	req.Header.Add("token", token)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Log.WithError(err).Error("Error sending ACI request")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Log.WithError(err).Error("Error reading ACI response body")
		return
	}

	var jsonResponse types.ACIResponse
	err = json.Unmarshal(respBody, &jsonResponse)
	if err != nil {
		logger.Log.WithError(err).Error("Error unmarshalling ACI JSON response")
		return
	}

	source := "acinfinity"
	for _, deviceData := range jsonResponse.Data {
		dataMap := map[string]float64{}
		device := deviceData.DevCode

		dataMap["ACI.tempF"] = float64(deviceData.DeviceInfo.TemperatureF) / 100.0
		dataMap["ACI.humidity"] = float64(deviceData.DeviceInfo.Humidity) / 100.0

		for _, sensor := range deviceData.DeviceInfo.Sensors {
			dataMap[fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)] = float64(sensor.SensorData) / 100.0
		}

		for _, port := range deviceData.DeviceInfo.Ports {
			dataMap[fmt.Sprintf("ACIP.%d", port.Port)] = float64(port.Speak) * 10
		}

		for key, value := range dataMap {
			addSensorData(source, device, key, fmt.Sprintf("%f", value))
		}
	}
}

func addSensorData(source string, device string, key string, value string) {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return
	}
	defer db.Close()

	var sensorID int
	err = db.QueryRow("SELECT id FROM sensors WHERE source = ? AND device = ? AND type = ?", source, device, key).Scan(&sensorID)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"source": source,
			"device": device,
			"type":   key,
			"error":  err,
		}).Error("Error querying sensor ID")
		return
	}

	_, err = db.Exec("INSERT INTO sensor_data (sensor_id, value) VALUES (?, ?)", sensorID, value)
	if err != nil {
		logger.Log.WithFields(logrus.Fields{
			"sensorID": sensorID,
			"value":    value,
			"error":    err,
		}).Error("Error writing sensor data to database")
	}
}

func PruneSensorData() error {
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return err
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM sensor_data WHERE create_dt < datetime(datetime('now', 'localtime'), '-90 day')")
	if err != nil {
		logger.Log.WithError(err).Error("Error pruning sensor data")
		return err
	}

	return nil
}
