package handlers

// Web adapters for activities and measurements. The request and response
// shapes are the ones the plant pages have always used; the service does
// the work.

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/service"
)

type measurementInput struct {
	MetricID int     `json:"metric_id"`
	Value    float64 `json:"value"`
}

func toMeasurementValues(in []measurementInput) []service.MeasurementValue {
	out := make([]service.MeasurementValue, len(in))
	for i, m := range in {
		out[i] = service.MeasurementValue{MetricID: m.MetricID, Value: m.Value}
	}
	return out
}

// activityInput builds a service input from a web form; ok is false (and
// the request answered) when the date can't be read.
func activityInput(c *gin.Context, activityID int, note, date string, ms []measurementInput) (service.ActivityInput, bool) {
	at, ok := parseFormTime(ServicesFromContext(c), date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return service.ActivityInput{}, false
	}
	in := service.ActivityInput{TypeID: activityID, Note: note, Measurements: toMeasurementValues(ms)}
	if at != nil {
		in.At = *at
	}
	return in, true
}

// CreatePlantActivity handles POST /plantActivity.
func CreatePlantActivity(c *gin.Context) {
	var in struct {
		PlantID      int                `json:"plant_id"`
		ActivityID   int                `json:"activity_id"`
		Note         string             `json:"note"`
		Date         string             `json:"date"`
		Measurements []measurementInput `json:"measurements"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	input, ok := activityInput(c, in.ActivityID, in.Note, in.Date, in.Measurements)
	if !ok {
		return
	}
	if _, err := ServicesFromContext(c).Activities.Create(c.Request.Context(), in.PlantID, input); err != nil {
		apiServiceError(c, err, "api_failed_to_create_activity")
		return
	}
	c.JSON(http.StatusCreated, in)
}

// EditActivity handles POST /plantActivity/edit: every field is sent, and the
// measurements replace the old ones.
func EditActivity(c *gin.Context) {
	var in struct {
		ID           int                `json:"id"`
		Date         string             `json:"date"`
		ActivityID   int                `json:"activity_id"`
		Note         string             `json:"note"`
		Measurements []measurementInput `json:"measurements"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	input, ok := activityInput(c, in.ActivityID, in.Note, in.Date, in.Measurements)
	if !ok {
		return
	}
	upd := service.ActivityUpdate{
		TypeID:       service.Put(&input.TypeID),
		Note:         service.Put(&input.Note),
		Measurements: service.Put(&input.Measurements),
	}
	if !input.At.IsZero() {
		upd.At = service.Put(&input.At)
	}
	if _, err := ServicesFromContext(c).Activities.Update(c.Request.Context(), in.ID, upd); err != nil {
		apiServiceError(c, err, "api_failed_to_update_activity")
		return
	}
	apiOK(c, "api_activity_updated")
}

// DeleteActivity handles DELETE /plantActivity/delete/:id.
func DeleteActivity(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := ServicesFromContext(c).Activities.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_activity")
		return
	}
	apiOK(c, "api_activity_deleted")
}

// RecordMultiPlantActivity handles POST /record-multi-activity.
func RecordMultiPlantActivity(c *gin.Context) {
	var in struct {
		PlantIDs     []int              `json:"plant_ids"`
		ActivityID   int                `json:"activity_id"`
		Note         string             `json:"note"`
		Date         string             `json:"date"`
		Measurements []measurementInput `json:"measurements"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	if len(in.PlantIDs) == 0 {
		apiBadRequest(c, "api_no_plants_selected")
		return
	}
	input, ok := activityInput(c, in.ActivityID, in.Note, in.Date, in.Measurements)
	if !ok {
		return
	}
	if _, err := ServicesFromContext(c).Activities.CreateBulk(c.Request.Context(), in.PlantIDs, input); err != nil {
		apiServiceError(c, err, "api_failed_to_save_activity")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
