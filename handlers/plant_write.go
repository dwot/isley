package handlers

// Web adapters for plant writes. The request and response shapes are the
// ones the plant pages have always used; service.PlantService does the
// work.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
	"isley/utils"
)

// parseWhen reads a date the web forms send: a bare day ("2026-11-01")
// or a date-time, read in the instance zone when it has no offset.
func parseWhen(svc *service.Services, s string) (service.When, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return service.When{}, true
	}
	if d, err := chrono.ParseDate(s); err == nil {
		return service.OnDay(d), true
	}
	t, err := svc.TimeZones().ParseInput(s)
	if err != nil {
		return service.When{}, false
	}
	return service.AtTime(t), true
}

// parseFormTime reads an optional date-time from a web form; a bare day
// means its start.
func parseFormTime(svc *service.Services, s string) (*time.Time, bool) {
	w, ok := parseWhen(svc, s)
	if !ok || w.IsZero() {
		return nil, ok
	}
	if w.At != nil {
		return w.At, true
	}
	t := time.Date(w.Day.Year, w.Day.Month, w.Day.Day, 0, 0, 0, 0, svc.TimeZones().Instance())
	return &t, true
}

type newStrainInput struct {
	Name       string `json:"name"`
	BreederId  int    `json:"breeder_id"`
	NewBreeder string `json:"new_breeder"`
}

// resolveZoneAndStrain creates the zone and strain a form asked for by
// name. On failure it has answered the request and returns false.
func resolveZoneAndStrain(c *gin.Context, zoneID *int, newZone string, strainID *int, newStrain *newStrainInput) (*int, *int, bool) {
	svc := ServicesFromContext(c)
	if zoneID == nil && strings.TrimSpace(newZone) != "" {
		zone, err := svc.Zones.Create(c.Request.Context(), service.ZoneInput{Name: newZone})
		if err != nil {
			apiServiceError(c, err, "api_failed_to_create_new_zone")
			return nil, nil, false
		}
		zoneID = &zone.ID
	}
	if strainID == nil && newStrain != nil {
		// A strain added from the plant form gets placeholder genetics
		// (50/50, autoflower, no seeds) for the strain page to fill in.
		st, err := svc.Strains.Create(c.Request.Context(), service.StrainInput{
			Name: newStrain.Name, BreederID: newStrain.BreederId, BreederName: newStrain.NewBreeder,
			IndicaPct: 50, SativaPct: 50, Autoflower: true,
		})
		if err != nil {
			apiServiceError(c, err, "api_failed_to_create_new_strain")
			return nil, nil, false
		}
		strainID = &st.ID
	}
	return zoneID, strainID, true
}

// AddPlant handles POST /plant.
func AddPlant(c *gin.Context) {
	var in struct {
		Name               string          `json:"name"`
		ZoneID             *int            `json:"zone_id"`
		NewZone            string          `json:"new_zone"`
		StrainID           *int            `json:"strain_id"`
		NewStrain          *newStrainInput `json:"new_strain"`
		StatusID           int             `json:"status_id"`
		Date               string          `json:"date"`
		Clone              int             `json:"clone"`
		ParentID           int             `json:"parent_id"`
		DecrementSeedCount bool            `json:"decrement_seed_count"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	svc := ServicesFromContext(c)
	start, ok := parseWhen(svc, in.Date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	zoneID, strainID, ok := resolveZoneAndStrain(c, in.ZoneID, in.NewZone, in.StrainID, in.NewStrain)
	if !ok {
		return
	}
	input := service.PlantInput{
		Name: in.Name, ZoneID: zoneID, StatusID: in.StatusID, Start: start,
		Clone: in.Clone != 0, DecrementSeedCount: in.DecrementSeedCount,
	}
	if strainID != nil {
		input.StrainID = *strainID
	}
	if in.ParentID > 0 {
		input.ParentID = &in.ParentID
	}
	p, err := svc.Plants.Create(c.Request.Context(), input)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_create_plant")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": p.ID, "message": T(c, "api_plant_added")})
}

// UpdatePlant handles POST /plant (the edit form): every field is sent,
// and a status change when status_id is set.
func UpdatePlant(c *gin.Context) {
	var in struct {
		PlantID           int             `json:"plant_id"`
		PlantName         string          `json:"plant_name"`
		PlantDescription  string          `json:"plant_description"`
		StatusID          int             `json:"status_id"`
		Date              string          `json:"date"`
		ZoneID            *int            `json:"zone_id"`
		NewZone           string          `json:"new_zone"`
		StrainID          *int            `json:"strain_id"`
		NewStrain         *newStrainInput `json:"new_strain"`
		IsClone           bool            `json:"clone"`
		StartDT           string          `json:"start_date"`
		HarvestWeight     float64         `json:"harvest_weight"`
		HarvestWeightUnit string          `json:"harvest_weight_unit"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	svc := ServicesFromContext(c)
	start, ok := parseWhen(svc, in.StartDT)
	if !ok {
		apiBadRequest(c, "start_date is not a valid date")
		return
	}
	statusAt, ok := parseFormTime(svc, in.Date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	zoneID, strainID, ok := resolveZoneAndStrain(c, in.ZoneID, in.NewZone, in.StrainID, in.NewStrain)
	if !ok {
		return
	}
	// The forms let the user enter grams or ounces; the database only ever
	// stores grams. An absent unit means the historical gram behaviour.
	harvestWeightG := utils.WeightToGrams(in.HarvestWeight, in.HarvestWeightUnit)
	upd := service.PlantUpdate{
		Name:           service.Put(&in.PlantName),
		Description:    service.Put(&in.PlantDescription),
		StrainID:       service.Put(strainID),
		ZoneID:         service.Put(zoneID),
		HarvestWeightG: service.Put(&harvestWeightG),
		Clone:          service.Put(&in.IsClone),
	}
	if !start.IsZero() {
		upd.Start = service.Put(&start)
	}
	ctx := c.Request.Context()
	if _, err := svc.Plants.Update(ctx, in.PlantID, upd); err != nil {
		apiServiceError(c, err, "api_failed_to_update_plant")
		return
	}
	if in.StatusID != 0 {
		if _, _, err := svc.Plants.ChangeStatus(ctx, in.PlantID, in.StatusID, statusAt); err != nil {
			apiServiceError(c, err, "api_failed_to_update_plant_status")
			return
		}
	}
	c.JSON(http.StatusCreated, in)
}

// DeletePlant handles DELETE /plant/delete/:id.
func DeletePlant(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := DeletePlantById(c.Request.Context(), ServicesFromContext(c), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_plant")
		return
	}
	apiOK(c, "api_plant_deleted")
}

// DeletePlantById deletes a plant and everything recorded for it. The zone
// and strain deletes use it to take their plants with them.
func DeletePlantById(ctx context.Context, svc *service.Services, id int) error {
	return svc.Plants.Delete(ctx, id)
}

// LinkSensorsToPlant handles POST /plant/link-sensors.
func LinkSensorsToPlant(c *gin.Context) {
	var in struct {
		PlantID   string `json:"plant_id"`
		SensorIDs []int  `json:"sensor_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	plantID, err := strconv.Atoi(in.PlantID)
	if err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if err := ServicesFromContext(c).Plants.SetSensors(c.Request.Context(), plantID, in.SensorIDs); err != nil {
		if e, ok := service.AsError(err); ok && e.Fields["sensor_ids"] != "" {
			apiBadRequest(c, "api_invalid_sensor_ids")
			return
		}
		apiServiceError(c, err, "api_failed_to_update_sensors")
		return
	}
	apiOK(c, "api_sensors_linked")
}

// UpdatePlantStatus handles POST /plant/status.
func UpdatePlantStatus(c *gin.Context) {
	var in struct {
		PlantID  int    `json:"plant_id"`
		StatusID int    `json:"status_id"`
		Date     string `json:"date"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if in.PlantID == 0 || in.StatusID == 0 {
		apiBadRequest(c, "api_plant_id_status_id_required")
		return
	}
	svc := ServicesFromContext(c)
	at, ok := parseFormTime(svc, in.Date)
	if !ok {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	entry, changed, err := svc.Plants.ChangeStatus(c.Request.Context(), in.PlantID, in.StatusID, at)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_update_plant_status")
		return
	}
	id := 0
	if changed {
		id = entry.ID
	}
	c.JSON(http.StatusOK, gin.H{"updated": changed, "id": id})
}

// EditStatus handles POST /plant/status/edit: it moves a history entry to
// a new date.
func EditStatus(c *gin.Context) {
	var in struct {
		ID   int    `json:"id"`
		Date string `json:"date"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	svc := ServicesFromContext(c)
	at, ok := parseFormTime(svc, in.Date)
	if !ok || at == nil {
		apiBadRequest(c, "date is not a valid date")
		return
	}
	if _, err := svc.Plants.UpdateStatusEntry(c.Request.Context(), 0, in.ID, service.StatusEntryUpdate{At: service.Put(at)}); err != nil {
		apiServiceError(c, err, "api_failed_to_update_status")
		return
	}
	apiOK(c, "api_status_updated")
}

// DeleteStatus handles DELETE /plant/status/:id.
func DeleteStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := ServicesFromContext(c).Plants.DeleteStatusEntry(c.Request.Context(), 0, id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_status")
		return
	}
	apiOK(c, "api_status_deleted")
}
