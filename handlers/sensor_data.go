package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"isley/config"
	"isley/model"
	"isley/model/types"
	"net/http"
	"strconv"
	"sync"
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

// Cache structure
var (
	sensorDataCache = make(map[string]cachedEntry)
	sdCacheMutex    sync.Mutex
)

// Cached entry structure
type cachedEntry struct {
	data      []types.SensorData
	timestamp time.Time
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

func ChartHandler(c *gin.Context) {
	// Extract query parameters
	sensor := c.Query("sensor")
	timeMinutes := c.Query("minutes")
	startDate := c.Query("start")
	endDate := c.Query("end")

	// Validate input
	if sensor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sensor parameter is required"})
		return
	}

	// Generate a cache key based on query parameters
	cacheKey := generateCacheKey(sensor, timeMinutes, startDate, endDate)

	sdCacheMutex.Lock()
	cached, found := sensorDataCache[cacheKey]
	sdCacheMutex.Unlock()

	// If cached data is valid, return it
	if found && time.Since(cached.timestamp) < time.Duration(config.PollingInterval)*time.Second {
		c.JSON(http.StatusOK, cached.data)
		return
	}

	var sensorData []types.SensorData
	var err error

	// Determine query type
	if startDate != "" && endDate != "" {
		sensorData, err = querySensorHistoryByDateRange(sensor, startDate, endDate)
	} else if timeMinutes != "" {
		sensorData, err = querySensorHistoryByTime(sensor, timeMinutes)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either minutes or start and end dates must be provided"})
		return
	}

	// Handle query errors
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Cache the new data
	sdCacheMutex.Lock()
	sensorDataCache[cacheKey] = cachedEntry{
		data:      sensorData,
		timestamp: time.Now(),
	}
	sdCacheMutex.Unlock()

	// Return data
	c.JSON(http.StatusOK, sensorData)
}

// Query sensor data by time range
func querySensorHistoryByTime(sensor string, timeMinutes string) ([]types.SensorData, error) {
	var sensorData []types.SensorData

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return sensorData, err
	}
	defer db.Close()

	// Convert sensor and timeMinutes to integers
	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		return sensorData, err
	}
	timeMinutesInt, err := strconv.Atoi(timeMinutes)
	if err != nil {
		return sensorData, err
	}

	// Query sensor_data table for the given time range
	timeThreshold := time.Now().Add(-time.Duration(timeMinutesInt) * time.Minute).Format("2006-01-02 15:04:05")
	query := "SELECT id, sensor_id, value, create_dt FROM sensor_data WHERE sensor_id = $1 AND create_dt > $2 ORDER BY create_dt"
	rows, err := db.Query(query, sensorInt, timeThreshold)
	if err != nil {
		return sensorData, err
	}
	defer rows.Close()

	// Parse query results
	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT); err != nil {
			return sensorData, err
		}
		sensorData = append(sensorData, record)
	}

	// Filter data density
	return filterSensorData(sensorData, timeMinutesInt), nil
}

// Query sensor data by custom date range
func querySensorHistoryByDateRange(sensor string, startDate string, endDate string) ([]types.SensorData, error) {
	var sensorData []types.SensorData

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return sensorData, err
	}
	defer db.Close()

	// Convert sensor ID to integer
	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		return sensorData, err
	}

	// Query sensor_data table for the given date range
	query := "SELECT id, sensor_id, value, create_dt FROM sensor_data WHERE sensor_id = $1 AND create_dt BETWEEN $2 AND $3 ORDER BY create_dt"
	rows, err := db.Query(query, sensorInt, startDate, endDate)
	if err != nil {
		return sensorData, err
	}
	defer rows.Close()

	// Parse query results
	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT); err != nil {
			return sensorData, err
		}
		sensorData = append(sensorData, record)
	}

	return sensorData, nil
}

// Filter sensor data density based on the time range
func filterSensorData(sensorData []types.SensorData, timeMinutes int) []types.SensorData {
	filteredSensorData := []types.SensorData{}

	if len(sensorData) > 0 {
		switch {
		case timeMinutes > 60*24*7:
			for i, v := range sensorData {
				if i%20 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		case timeMinutes > 60*24:
			for i, v := range sensorData {
				if i%10 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		case timeMinutes > 60*6:
			for i, v := range sensorData {
				if i%5 == 0 {
					filteredSensorData = append(filteredSensorData, v)
				}
			}
		default:
			filteredSensorData = sensorData
		}
	}

	return filteredSensorData
}

func generateCacheKey(sensor, timeMinutes, startDate, endDate string) string {
	return sensor + "|" + timeMinutes + "|" + startDate + "|" + endDate
}
