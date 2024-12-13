package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"isley/model"
	"isley/model/types"
	"net/http"
	"strconv"
	"time"
)

type LatestSensorData struct {
	TentTemp         float64
	TentHumidity     float64
	TentVpd          float64
	LungRoomTemp     float64
	LungRoomHumidity float64
	Soil1Moisture    float64
	Soil2Moisture    float64
	Soil3Moisture    float64
	Soil4Moisture    float64
	Soil5Moisture    float64
	Soil6Moisture    float64
}

func GetSensorLatest() LatestSensorData {
	var sensorData LatestSensorData
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return sensorData
	}
	rows, err := db.Query("SELECT * FROM sensor_data WHERE id IN (SELECT MAX(id) FROM sensor_data GROUP BY sensor_id)")
	if err != nil {
		fmt.Println(err)
		return sensorData
	}

	// Iterate over rows
	for rows.Next() {
		var id int
		var sensor_id int
		var value float64
		var create_dt time.Time
		err = rows.Scan(&id, &sensor_id, &value, &create_dt)
		if err != nil {
			fmt.Println(err)
			return sensorData
		}
		switch sensor_id {
		case 1:
			sensorData.TentTemp = value
		case 2:
			sensorData.TentHumidity = value
		case 3:
			sensorData.TentVpd = value
		case 4:
			sensorData.LungRoomTemp = value
		case 5:
			sensorData.LungRoomHumidity = value
		case 6:
			sensorData.Soil1Moisture = value
		case 7:
			sensorData.Soil2Moisture = value
		case 8:
			sensorData.Soil3Moisture = value
		case 9:
			sensorData.Soil4Moisture = value
		case 10:
			sensorData.Soil5Moisture = value
		case 11:
			sensorData.Soil6Moisture = value
		}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return sensorData
	}

	return sensorData
}

func ChartHandler(c *gin.Context, sensor string, timeMinutes string) {
	//Get sensor data for the last 24 hours for sensor 1
	sensorData := querySensorHistory(sensor, timeMinutes)
	c.JSON(http.StatusOK, sensorData)
}

func querySensorHistory(sensor string, timeMinutes string) []types.SensorData {
	var sensorData []types.SensorData
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return sensorData
	}

	//convert sensor and timeMinutes to int
	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		fmt.Println(err)
		return sensorData
	}
	timeMinutesInt, err := strconv.Atoi(timeMinutes)
	if err != nil {
		fmt.Println(err)
		return sensorData
	}

	//query sensor_data table for sensor_id = 1 and create_dt in last 24 hours
	timeThreshold := time.Now().Add(-time.Duration(timeMinutesInt) * time.Minute).Format("2006-01-02 15:04:05")
	rows, err := db.Query("SELECT * FROM sensor_data WHERE sensor_id = $1 AND create_dt > $2 ORDER BY create_dt", sensorInt, timeThreshold)
	if err != nil {
		fmt.Println(err)
		return sensorData
	}

	// Iterate over rows
	for rows.Next() {
		//write row
		var id int
		var sensor_id int
		var value float64
		var create_dt time.Time
		err = rows.Scan(&id, &sensor_id, &value, &create_dt)
		if err != nil {
			fmt.Println(err)
			return sensorData
		}
		sensorData = append(sensorData, types.SensorData{ID: uint(id), SensorID: sensor_id, Value: value, CreateDT: create_dt})
	}

	//Filter down the sensorData density.
	//If timeMinutes > 3 hours of data, show all data
	//If timeMinutes > 6+ hours of data, show every 5th data point
	//If timeMinutes > 48+ hours of data, show every 10th data point
	//If timeMinutes > 1+ week of data, show every 20th data point
	// end rules
	filteredSensorData := []types.SensorData{}
	if len(sensorData) > 0 {
		if timeMinutesInt > 60*24*7 {
			for i, v := range sensorData {
				if i%20 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		} else if timeMinutesInt > 60*24 {
			for i, v := range sensorData {
				if i%10 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		} else if timeMinutesInt > 60*6 {
			for i, v := range sensorData {
				if i%5 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		} else {
			filteredSensorData = sensorData
		}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return filteredSensorData
	}

	return filteredSensorData
}
