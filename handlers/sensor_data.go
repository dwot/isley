package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/config"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"isley/utils"
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
		apiBadRequest(c, "api_sensor_param_required")
		return
	}

	cacheKey := generateCacheKey(sensor, timeMinutes, startDate, endDate)

	sdCacheMutex.Lock()
	cached, found := sensorDataCache[cacheKey]
	sdCacheMutex.Unlock()

	if found && time.Since(cached.timestamp) < time.Duration(config.PollingInterval/10)*time.Second {
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
		apiBadRequest(c, "api_sensor_dates_required")
		return
	}

	if err != nil {
		sensorLogger.WithError(err).Error("Failed to query sensor data")
		apiInternalError(c, "api_sensor_query_failed")
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

	db, err := model.GetDB()
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to open database")
		return sensorData, err
	}

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

	// For ranges >24 hours, use the hourly rollup table for much better performance
	if timeMinutesInt > 60*24 {
		timeThreshold := time.Now().In(time.UTC).Add(-time.Duration(timeMinutesInt) * time.Minute).Format(utils.LayoutDB)
		query := `SELECT 0, sd.sensor_id, sd.avg_val, sd.bucket, s.name
			FROM sensor_data_hourly sd
			LEFT OUTER JOIN sensors s ON s.id = sd.sensor_id
			WHERE sd.sensor_id = $1 AND sd.bucket > $2
			ORDER BY sd.bucket`
		rows, err := db.Query(query, sensorInt, timeThreshold)
		if err != nil {
			sensorLogger.WithError(err).Error("Failed to execute rollup query")
			return sensorData, err
		}
		defer rows.Close()

		for rows.Next() {
			var record types.SensorData
			if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT, &record.SensorName); err != nil {
				sensorLogger.WithError(err).Error("Failed to scan rollup row")
				return sensorData, err
			}
			record.CreateDT = record.CreateDT.Local()
			sensorData = append(sensorData, record)
		}

		sensorLogger.WithField("source", "rollup").Info("Query completed successfully")
		return sensorData, nil
	}

	// For ranges ≤24 hours, use raw sensor_data for full resolution
	timeThreshold := time.Now().In(time.UTC).Add(-time.Duration(timeMinutesInt) * time.Minute).Format(utils.LayoutDB)
	query := "SELECT sd.id, sd.sensor_id, sd.value, sd.create_dt, s.name FROM sensor_data sd left outer join sensors s on s.id = sd.sensor_id WHERE sd.sensor_id = $1 AND sd.create_dt > $2 ORDER BY sd.create_dt LIMIT 10000"
	rows, err := db.Query(query, sensorInt, timeThreshold)
	if err != nil {
		sensorLogger.WithError(err).Error("Failed to execute query")
		return sensorData, err
	}
	defer rows.Close()

	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT, &record.SensorName); err != nil {
			sensorLogger.WithError(err).Error("Failed to scan row")
			return sensorData, err
		}
		record.CreateDT = record.CreateDT.Local()
		sensorData = append(sensorData, record)
	}

	sensorLogger.Info("Query completed successfully")
	return sensorData, nil
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
	db, err := model.GetDB()
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}

	// Convert sensor ID to integer
	sensorInt, err := strconv.Atoi(sensor)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}

	//Convert startDate and endDate strings from Local to UTC and back to strings
	startDateUTC, err := timeConversion(startDate)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}
	endDateUTC, err := timeConversion(endDate)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}

	// Determine if the date range spans more than 24 hours — if so, use rollups
	startParsed, _ := time.Parse(utils.LayoutDB, startDateUTC)
	endParsed, _ := time.Parse(utils.LayoutDB, endDateUTC)
	rangeHours := endParsed.Sub(startParsed).Hours()

	if rangeHours > 24 {
		query := `SELECT 0, sd.sensor_id, sd.avg_val, sd.bucket, s.name
			FROM sensor_data_hourly sd
			LEFT OUTER JOIN sensors s ON s.id = sd.sensor_id
			WHERE sd.sensor_id = $1 AND sd.bucket BETWEEN $2 AND $3
			ORDER BY sd.bucket`
		rows, err := db.Query(query, sensorInt, startDateUTC, endDateUTC)
		if err != nil {
			sensorLogger.WithError(err).Error(err)
			return sensorData, err
		}
		defer rows.Close()

		for rows.Next() {
			var record types.SensorData
			if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT, &record.SensorName); err != nil {
				sensorLogger.WithError(err).Error(err)
				return sensorData, err
			}
			record.CreateDT = record.CreateDT.Local()
			sensorData = append(sensorData, record)
		}

		return sensorData, nil
	}

	// For short ranges (≤24h), use raw sensor_data for full resolution
	query := "SELECT sd.id, sd.sensor_id, sd.value, sd.create_dt, s.name FROM sensor_data sd left outer join sensors s on s.id = sd.sensor_id WHERE sd.sensor_id = $1 AND sd.create_dt BETWEEN $2 AND $3 ORDER BY sd.create_dt LIMIT 10000"
	rows, err := db.Query(query, sensorInt, startDateUTC, endDateUTC)
	if err != nil {
		sensorLogger.WithError(err).Error(err)
		return sensorData, err
	}
	defer rows.Close()

	// Parse query results
	for rows.Next() {
		var record types.SensorData
		if err := rows.Scan(&record.ID, &record.SensorID, &record.Value, &record.CreateDT, &record.SensorName); err != nil {
			sensorLogger.WithError(err).Error(err)
			return sensorData, err
		}
		record.CreateDT = record.CreateDT.Local()
		sensorData = append(sensorData, record)
	}

	return sensorData, nil
}

func timeConversion(date string) (string, error) {
	if len(date) == 10 {
		date += " 00:00:00"
	}
	t, err := time.Parse(utils.LayoutDB, date)
	if err != nil {
		return date, err
	}
	return t.UTC().Format(utils.LayoutDB), nil
}

func generateCacheKey(sensor, timeMinutes, startDate, endDate string) string {
	return sensor + "|" + timeMinutes + "|" + startDate + "|" + endDate
}
