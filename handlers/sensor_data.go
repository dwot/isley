package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/logger"
	"isley/model/types"
	"isley/service"
)

// ChartHandler serves a sensor's history to the graph page: either the
// last `minutes`, or `start` to `end` (dates or date-times in the instance
// zone; an end given as a date takes in that whole day). Ranges over 24
// hours come back as hourly averages (id 0), shorter ones as raw readings.
func ChartHandler(c *gin.Context) {
	sensor := c.Query("sensor")
	timeMinutes := c.Query("minutes")
	startDate := c.Query("start")
	endDate := c.Query("end")

	if sensor == "" {
		apiBadRequest(c, "api_sensor_param_required")
		return
	}
	sensorID, err := strconv.Atoi(sensor)
	if err != nil {
		apiBadRequest(c, "api_sensor_param_required")
		return
	}

	svc := ServicesFromContext(c)
	zones := svc.TimeZones()
	var from, to time.Time
	switch {
	case startDate != "" && endDate != "":
		var errFrom, errTo error
		from, errFrom = chartTime(zones, startDate, false)
		to, errTo = chartTime(zones, endDate, true)
		if errFrom != nil || errTo != nil {
			apiBadRequest(c, "api_sensor_dates_required")
			return
		}
	case timeMinutes != "":
		minutes, err := strconv.Atoi(timeMinutes)
		// At most as far back as the hourly history can answer in one go;
		// this also keeps the duration from overflowing.
		if err != nil || minutes <= 0 || minutes > service.MaxBuckets*60 {
			apiBadRequest(c, "api_sensor_dates_required")
			return
		}
		to = zones.Now(svc.Clock())
		from = to.Add(-time.Duration(minutes) * time.Minute)
	default:
		apiBadRequest(c, "api_sensor_dates_required")
		return
	}

	cacheKey := generateCacheKey(sensor, timeMinutes, startDate, endDate)
	cache := SensorCacheServiceFromContext(c)
	pollInterval := ConfigStoreFromContext(c).PollingInterval()
	if cached, cachedAt, found := cache.DataGet(cacheKey); found &&
		time.Since(cachedAt) < time.Duration(pollInterval/10)*time.Second {
		c.JSON(http.StatusOK, cached)
		return
	}

	data, err := chartData(c, svc, sensorID, from, to)
	if err != nil {
		if service.IsKind(err, service.KindNotFound) || service.IsKind(err, service.KindInvalid) {
			apiServiceError(c, err, "api_sensor_query_failed")
			return
		}
		logger.Log.WithError(err).WithField("sensor", sensorID).Error("Failed to query sensor data")
		apiInternalError(c, "api_sensor_query_failed")
		return
	}
	cache.DataPut(cacheKey, data)
	c.JSON(http.StatusOK, data)
}

func chartData(c *gin.Context, svc *service.Services, sensorID int, from, to time.Time) ([]types.SensorData, error) {
	ctx := c.Request.Context()
	sn, err := svc.Sensors.Get(ctx, sensorID)
	if err != nil {
		return nil, err
	}
	out := []types.SensorData{}
	if to.Sub(from) > RollupThresholdHours*time.Hour {
		buckets, err := svc.Sensors.Buckets(ctx, sensorID, from, to, time.Hour)
		if err != nil {
			return nil, err
		}
		for _, b := range buckets {
			out = append(out, types.SensorData{SensorID: sensorID, SensorName: sn.Name, Value: b.Avg, CreateDT: b.Start})
		}
		return out, nil
	}
	points, _, err := svc.Sensors.Raw(ctx, sensorID, from, to, 0, MaxRawDataRows)
	if err != nil {
		return nil, err
	}
	for _, p := range points {
		out = append(out, types.SensorData{ID: uint(p.ID), SensorID: sensorID, SensorName: sn.Name, Value: p.Value, CreateDT: p.At})
	}
	return out, nil
}

// chartTime reads a range end from the graph page. A bare date is
// midnight, or the next midnight when it ends the range.
func chartTime(zones *chrono.Zones, s string, end bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) == len(chrono.DateLayout) {
		d, err := chrono.ParseDate(s)
		if err != nil {
			return time.Time{}, err
		}
		if end {
			d = d.AddDays(1)
		}
		return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, zones.Instance()), nil
	}
	return zones.ParseInput(s)
}

func generateCacheKey(sensor, timeMinutes, startDate, endDate string) string {
	return sensor + "|" + timeMinutes + "|" + startDate + "|" + endDate
}
