package handlers

// Settings-page endpoints for zones, statuses, metrics and activity types.
// They keep their request and response shapes; the work is done by
// service.ZoneService and service.LookupService.

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/service"
)

// pathID reads the :id parameter. On a bad id it answers 400 and returns
// false.
func pathID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		apiBadRequest(c, "api_invalid_payload")
		return 0, false
	}
	return id, true
}

func AddZoneHandler(c *gin.Context) {
	var req struct {
		Name string `json:"zone_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	zone, err := ServicesFromContext(c).Zones.Create(c.Request.Context(), service.ZoneInput{Name: req.Name})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_zone")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": zone.ID})
}

func UpdateZoneHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	// The settings form always sends every field; null clears one.
	var req struct {
		Name                string   `json:"zone_name"`
		LeafTempOffset      *float64 `json:"leaf_temp_offset"`
		VPDTempSensorID     *int     `json:"vpd_temp_sensor_id"`
		VPDHumiditySensorID *int     `json:"vpd_humidity_sensor_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	_, err := ServicesFromContext(c).Zones.Update(c.Request.Context(), id, service.ZoneUpdate{
		Name:                service.Put(&req.Name),
		LeafTempOffsetF:     service.Put(req.LeafTempOffset),
		VPDTempSensorID:     service.Put(req.VPDTempSensorID),
		VPDHumiditySensorID: service.Put(req.VPDHumiditySensorID),
	})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_update_zone")
		return
	}
	apiOK(c, "api_zone_updated")
}

// DeleteZoneHandler deletes a zone together with its plants, sensors and
// streams, as the settings page has always done. (/api/v1 refuses to
// delete a zone that isn't empty.)
func DeleteZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteZoneHandler")
	id, ok := pathID(c)
	if !ok {
		return
	}
	db := DBFromContext(c)
	contents := []struct {
		table  string
		delete func(id string)
	}{
		{"plant", func(id string) {
			plantID, _ := strconv.Atoi(id)
			if err := DeletePlantById(c.Request.Context(), ServicesFromContext(c), plantID); err != nil {
				fieldLogger.WithError(err).WithField("plant", plantID).Error("Failed to delete plant")
			}
		}},
		{"sensors", func(id string) {
			sensorID, _ := strconv.Atoi(id)
			if err := ServicesFromContext(c).Sensors.Delete(c.Request.Context(), sensorID); err != nil {
				fieldLogger.WithError(err).WithField("sensor", sensorID).Error("Failed to delete sensor")
			}
		}},
		{"streams", func(id string) {
			streamID, _ := strconv.Atoi(id)
			if err := ServicesFromContext(c).Streams.Delete(c.Request.Context(), streamID); err != nil {
				fieldLogger.WithError(err).WithField("stream", streamID).Error("Failed to delete stream")
			}
		}},
	}
	for _, kind := range contents {
		rows, err := db.QueryContext(c.Request.Context(),
			fmt.Sprintf("SELECT id FROM %s WHERE zone_id = $1", kind.table), id)
		if err != nil {
			fieldLogger.WithError(err).WithField("table", kind.table).Error("Failed to list zone contents")
			apiInternalError(c, "api_failed_to_delete_zone")
			return
		}
		var ids []int
		for rows.Next() {
			var rowID int
			if err := rows.Scan(&rowID); err == nil {
				ids = append(ids, rowID)
			}
		}
		rows.Close()
		for _, rowID := range ids {
			kind.delete(strconv.Itoa(rowID))
		}
	}
	if err := ServicesFromContext(c).Zones.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_zone")
		return
	}
	apiOK(c, "api_zone_deleted")
}

// UpdateStatusVPDHandler sets a status's target VPD range.
// Route: POST /statuses/:id/vpd
func UpdateStatusVPDHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req struct {
		VPDLow  *float64 `json:"vpd_low"`
		VPDHigh *float64 `json:"vpd_high"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if _, err := ServicesFromContext(c).Lookups.SetStatusVPD(c.Request.Context(), id, req.VPDLow, req.VPDHigh); err != nil {
		apiServiceError(c, err, "api_failed_to_update_status_vpd")
		return
	}
	apiOK(c, "api_status_vpd_updated")
}

func GetMetricsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, ConfigStoreFromContext(c).Metrics())
}

type metricRequest struct {
	Name string `json:"metric_name"`
	Unit string `json:"metric_unit"`
}

func AddMetricHandler(c *gin.Context) {
	var req metricRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	m, err := ServicesFromContext(c).Lookups.CreateMetric(c.Request.Context(), service.MetricInput{Name: req.Name, Unit: req.Unit})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_metric")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": m.ID})
}

func UpdateMetricHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req metricRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if _, err := ServicesFromContext(c).Lookups.UpdateMetric(c.Request.Context(), id, service.MetricInput{Name: req.Name, Unit: req.Unit}); err != nil {
		apiServiceError(c, err, "api_failed_to_update_metric")
		return
	}
	apiOK(c, "api_metric_updated")
}

func DeleteMetricHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := ServicesFromContext(c).Lookups.DeleteMetric(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_metric")
		return
	}
	apiOK(c, "api_metric_deleted")
}

type activityRequest struct {
	Name       string `json:"activity_name"`
	IsWatering bool   `json:"is_watering"`
	IsFeeding  bool   `json:"is_feeding"`
	Metrics    []struct {
		MetricID int  `json:"metric_id"`
		Required bool `json:"required"`
	} `json:"activity_metrics"`
}

func (r activityRequest) input() service.ActivityTypeInput {
	in := service.ActivityTypeInput{Name: r.Name, IsWatering: r.IsWatering, IsFeeding: r.IsFeeding}
	for _, m := range r.Metrics {
		in.Metrics = append(in.Metrics, service.ActivityMetricInput{MetricID: m.MetricID, Required: m.Required})
	}
	return in
}

func AddActivityHandler(c *gin.Context) {
	var req activityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	a, err := ServicesFromContext(c).Lookups.CreateActivityType(c.Request.Context(), req.input())
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_activity")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": a.ID})
}

func UpdateActivityHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req activityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if _, err := ServicesFromContext(c).Lookups.UpdateActivityType(c.Request.Context(), id, req.input()); err != nil {
		apiServiceError(c, err, "api_failed_to_update_activity")
		return
	}
	apiOK(c, "api_activity_updated")
}

func DeleteActivityHandler(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := ServicesFromContext(c).Lookups.DeleteActivityType(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_activity")
		return
	}
	apiOK(c, "api_activity_deleted")
}
