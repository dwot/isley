package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/service"
)

// CreatePlantMeasurement handles POST /plantMeasurement.
func CreatePlantMeasurement(c *gin.Context) {
	var in struct {
		PlantID  int     `json:"plant_id"`
		MetricID int     `json:"metric_id"`
		Value    float64 `json:"value"`
		Date     string  `json:"date"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	svc := ServicesFromContext(c)
	at, ok := parseFormTime(svc, in.Date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	input := service.MeasurementInput{MetricID: in.MetricID, Value: in.Value}
	if at != nil {
		input.At = *at
	}
	if _, err := svc.Measurements.Create(c.Request.Context(), in.PlantID, input); err != nil {
		apiServiceError(c, err, "api_failed_to_save_measurement")
		return
	}
	c.JSON(http.StatusCreated, in)
}

// EditMeasurement handles POST /plantMeasurement/edit.
func EditMeasurement(c *gin.Context) {
	var in struct {
		ID    int     `json:"id"`
		Date  string  `json:"date"`
		Value float64 `json:"value"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	svc := ServicesFromContext(c)
	at, ok := parseFormTime(svc, in.Date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	upd := service.MeasurementUpdate{Value: service.Put(&in.Value)}
	if at != nil {
		upd.At = service.Put(at)
	}
	if _, err := svc.Measurements.Update(c.Request.Context(), in.ID, upd); err != nil {
		apiServiceError(c, err, "api_failed_to_update_measurement")
		return
	}
	apiOK(c, "api_measurement_updated")
}

// DeleteMeasurement handles DELETE /plantMeasurement/delete/:id.
func DeleteMeasurement(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := ServicesFromContext(c).Measurements.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_measurement")
		return
	}
	apiOK(c, "api_measurement_deleted")
}
