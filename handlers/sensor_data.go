package handlers

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Cache structure
var (
	sensorDataCache = make(map[string]cachedEntry)
	sdCacheMutex    sync.Mutex
)

type cachedEntry struct {
	data      []types.SensorData
	timestamp time.Time
}

func ChartHandler(c *gin.Context) {
	sensorLogger := logger.Log.WithField("handler", "ChartHandler")

	sensor := c.Query("sensor")
	timeMinutes := c.Query("minutes")
	startDate := c.Query("start")
	endDate := c.Query("end")

	sensorLogger = sensorLogger.WithFields(logrus.Fields{
		"sensor":      sensor,
		"timeMinutes": timeMinutes,
		"startDate":   startDate,
		"endDate":     endDate,
	})

	if sensor == "" {
		sensorLogger.Error("Sensor parameter is required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "sensor parameter is required"})
		return
	}

	cacheKey := generateCacheKey(sensor, timeMinutes, startDate, endDate)

	sdCacheMutex.Lock()
	cached, found := sensorDataCache[cacheKey]
	sdCacheMutex.Unlock()

	if found && time.Since(cached.timestamp) < time.Duration(config.PollingInterval)*time.Second {
		sensorLogger.Info("Serving data from cache")
		c.JSON(http.StatusOK, cached.data)
		return
	}

	var sensorData []types.SensorData
	var err error

	if startDate != "" && endDate != "" {
		sensorData, err = querySensorHistoryByDateRange(sensor, startDate, endDate)
	} else if timeMinutes != "" {
		sensorData, err = querySensorHistoryByTime(sensor, timeMinutes)
	} else {
		sensorLogger.Error("Invalid query parameters: Either minutes or start/end dates must be provided")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either minutes or start and end dates must be provided"})
		return
	}

	if err != nil {
		sensorLogger.WithError(err).Error("Failed to query sensor data")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sdCacheMutex.Lock()
	sensorDataCache[cacheKey] = cachedEntry{
		data:      sensorData,
		timestamp: time.Now().In(time.Local),
	}
	sdCacheMutex.Unlock()

	sensorLogger.Info("Returning queried sensor data")
	c.JSON(http.StatusOK, sensorData)
}

func querySensorHistoryByTime(sensor string, timeMinutes string) ([]types.SensorData, error) {
	sensorLogger := logger.Log.WithFields(logrus.Fields{
		"function":    "querySensorHistoryByTime",
		"sensor":      sensor,
		"timeMinutes": timeMinutes,
	})
	var sensorData []types.SensorData

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to open database")
		return sensorData, err
	}
	defer db.Close()

	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to convert sensor to integer")
		return sensorData, err
	}
	timeMinutesInt, err := strconv.Atoi(timeMinutes)
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to convert timeMinutes to integer")
		return sensorData, err
	}

	timeThreshold := time.Now().In(time.Local).Add(-time.Duration(timeMinutesInt) * time.Minute).Format("2006-01-02 15:04:05")
	query := "SELECT id, sensor_id, value, create_dt FROM sensor_data WHERE sensor_id = $1 AND create_dt > $2 ORDER BY create_dt"
	rows, err := db.Query(query, sensorInt, timeThreshold)
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to execute query")
		return sensorData, err
	}
	defer rows.Close()

	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT); err != nil {
			sensorLogger.WithError(err).Error("Failed to scan row")
			return sensorData, err
		}
		sensorData = append(sensorData, record)
	}

	sensorLogger.Info("Query completed successfully")
	return filterSensorData(sensorData, timeMinutesInt), nil
}

// Query sensor data by custom date range
func querySensorHistoryByDateRange(sensor string, startDate string, endDate string) ([]types.SensorData, error) {
	sensorLogger := logger.Log.WithFields(logrus.Fields{
		"function":  "querySensorHistoryByDateRange",
		"sensor":    sensor,
		"startDate": startDate,
		"endDate":   endDate,
	})

	var sensorData []types.SensorData

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}
	defer db.Close()

	// Convert sensor ID to integer
	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}

	// Query sensor_data table for the given date range
	query := "SELECT id, sensor_id, value, create_dt FROM sensor_data WHERE sensor_id = $1 AND create_dt BETWEEN $2 AND $3 ORDER BY create_dt"
	rows, err := db.Query(query, sensorInt, startDate, endDate)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}
	defer rows.Close()

	// Parse query results
	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT); err != nil {
			sensorLogger.WithError(err).Error(err)
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
