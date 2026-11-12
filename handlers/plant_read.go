package handlers

// Web adapters for plant reads. service.PlantService does the work; these
// shape its results into the model/types values the templates and the
// dashboard JavaScript have always used.

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/logger"
	"isley/model/types"
	"isley/service"
)

// GetLivingPlants returns the growing plants for page sidebars and the
// dashboard, ordered by start then name. It returns nil on error.
func GetLivingPlants(ctx context.Context, svc *service.Services) []types.PlantListResponse {
	plants, err := listPlantsForWeb(ctx, svc, service.PhaseGrowing)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to list living plants")
		return nil
	}
	return plants
}

func listPlantsForWeb(ctx context.Context, svc *service.Services, phase service.PlantPhase) ([]types.PlantListResponse, error) {
	plants, _, err := svc.Plants.List(ctx, service.PlantFilter{Phase: phase})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(plants, func(i, j int) bool {
		if !plants[i].StartedAt.Equal(plants[j].StartedAt) {
			return plants[i].StartedAt.Before(plants[j].StartedAt)
		}
		return plants[i].Name < plants[j].Name
	})
	// Plants not yet harvested have always shown today as their harvest date.
	today := svc.TimeZones().Today(svc.Clock())
	startOfToday := dayStart(&today, svc)
	out := make([]types.PlantListResponse, len(plants))
	for i, p := range plants {
		out[i] = toPlantListResponse(p, startOfToday, svc)
	}
	return out, nil
}

func toPlantListResponse(p service.Plant, notHarvested time.Time, svc *service.Services) types.PlantListResponse {
	r := types.PlantListResponse{
		ID:            p.ID,
		Name:          p.Name,
		Description:   p.Description,
		Clone:         p.Clone,
		CurrentDay:    p.Day,
		CurrentWeek:   p.Week,
		HarvestWeight: p.HarvestWeightG,
		HarvestDate:   notHarvested,
	}
	if !p.StartedAt.IsZero() {
		r.StartDT = p.StartedAt.Format(time.RFC3339)
	}
	if p.Strain != nil {
		r.StrainName, r.BreederName = p.Strain.Name, p.Strain.Breeder.Name
		r.CycleTime, r.StrainUrl, r.Autoflower = p.Strain.CycleDays, p.Strain.URL, p.Strain.Autoflower
	}
	if p.Zone != nil {
		r.ZoneName = p.Zone.Name
	}
	if p.Status != nil {
		r.Status, r.StatusDate = string(p.Status.Status), p.Status.At
	}
	// The dashboard has always read "never" as 0 here.
	if p.DaysSinceWatered != nil {
		r.DaysSinceLastWatering = *p.DaysSinceWatered
	}
	if p.DaysSinceFed != nil {
		r.DaysSinceLastFeeding = *p.DaysSinceFed
	}
	flowering := 0
	if p.Flower != nil {
		flowering = p.Flower.Days
	}
	r.FloweringDays = &flowering
	if p.HarvestedAt != nil {
		r.HarvestDate = *p.HarvestedAt
	}
	r.EstHarvestDate = dayStart(p.EstimatedHarvest, svc)
	return r
}

// dayStart is midnight of d in the instance zone, or the zero time.
func dayStart(d *chrono.Date, svc *service.Services) time.Time {
	if d == nil || d.IsZero() {
		return time.Time{}
	}
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, svc.TimeZones().Instance())
}

func plantListHandler(phase service.PlantPhase) gin.HandlerFunc {
	return func(c *gin.Context) {
		plants, err := listPlantsForWeb(c.Request.Context(), ServicesFromContext(c), phase)
		if err != nil {
			logger.Log.WithError(err).WithField("phase", phase).Error("Failed to list plants")
			apiInternalError(c, "api_failed_to_retrieve_plants")
			return
		}
		c.JSON(http.StatusOK, plants)
	}
}

// LivingPlantsHandler handles GET /plants/living.
var LivingPlantsHandler = plantListHandler(service.PhaseGrowing)

// HarvestedPlantsHandler handles GET /plants/harvested.
var HarvestedPlantsHandler = plantListHandler(service.PhaseHarvested)

// DeadPlantsHandler handles GET /plants/dead.
var DeadPlantsHandler = plantListHandler(service.PhaseDead)

// GetPlant returns the plant page's data, or a zero Plant (ID 0) if id
// doesn't name a plant.
func GetPlant(ctx context.Context, svc *service.Services, id string) types.Plant {
	plantID, err := strconv.Atoi(id)
	if err != nil {
		return types.Plant{}
	}
	d, err := svc.Plants.Detail(ctx, plantID, PlantHistoryLimit)
	if err != nil {
		if !service.IsKind(err, service.KindNotFound) {
			logger.Log.WithError(err).WithField("plant", plantID).Error("Failed to load plant")
		}
		return types.Plant{}
	}
	now := svc.TimeZones().Now(svc.Clock())

	p := types.Plant{
		ID:            uint(d.ID),
		Name:          d.Name,
		Description:   d.Description,
		CurrentDay:    d.Day,
		CurrentWeek:   d.Week,
		CurrentHeight: "0",
		IsClone:       d.Clone,
		StartDT:       d.StartedAt,
		HarvestWeight: d.HarvestWeightG,
		HarvestDate:   now,
	}
	if d.Status != nil {
		p.Status, p.StatusID = string(d.Status.Status), d.Status.StatusID
	}
	if d.Strain != nil {
		p.StrainID, p.StrainName, p.BreederName = d.Strain.ID, d.Strain.Name, d.Strain.Breeder.Name
		p.CycleTime, p.StrainUrl, p.Autoflower = d.Strain.CycleDays, d.Strain.URL, d.Strain.Autoflower
	}
	if d.Zone != nil {
		p.ZoneID, p.ZoneName = d.Zone.ID, d.Zone.Name
	}
	if d.Parent != nil {
		p.ParentID, p.ParentName = uint(d.Parent.ID), d.Parent.Name
	}
	if d.LastWatered != nil {
		p.LastWaterDate = *d.LastWatered
	}
	if d.LastFed != nil {
		p.LastFeedDate = *d.LastFed
	}
	if d.HarvestedAt != nil {
		p.HarvestDate = *d.HarvestedAt
	}
	p.EstHarvestDate = dayStart(d.EstimatedHarvest, svc)

	// The page lists history newest first.
	for i := len(d.StatusHistory) - 1; i >= 0; i-- {
		e := d.StatusHistory[i]
		p.StatusHistory = append(p.StatusHistory, types.Status{ID: uint(e.ID), Status: string(e.Status), Date: e.At, StatusID: e.StatusID})
	}
	p.GrowDays = d.GrowDays
	p.StageDays = map[string]int{}
	for _, st := range d.Stages {
		p.StageDays[string(st.Status)] = st.Days
	}
	for _, m := range d.Measurements {
		p.Measurements = append(p.Measurements, toTypesMeasurement(m))
	}
	for _, a := range d.Activities {
		pa := types.PlantActivity{
			ID: uint(a.ID), Name: a.Type.Name, Note: a.Note, Date: a.At,
			ActivityId: a.Type.ID, IsWatering: a.IsWatering, IsFeeding: a.IsFeeding,
		}
		for _, m := range a.Measurements {
			pa.Measurements = append(pa.Measurements, toTypesMeasurement(m))
		}
		p.Activities = append(p.Activities, pa)
	}
	for _, s := range d.Sensors {
		r := types.SensorDataResponse{ID: uint(s.ID), Name: s.Name, Unit: s.Unit, Inherited: s.Inherited}
		if s.Latest != nil {
			r.Value, r.Date = s.Latest.Value, s.Latest.At
		}
		if s.Source == "derived" && s.Type == "VPD" {
			r.IsVPD = true
		}
		if s.VPD != nil {
			low, high := s.VPD.LowKPa, s.VPD.HighKPa
			r.IdealLow, r.IdealHigh, r.RangeState = &low, &high, s.VPD.State
		}
		p.Sensors = append(p.Sensors, r)
	}
	for _, img := range d.Images {
		taken := dayStart(&img.TakenOn, svc)
		p.Images = append(p.Images, types.PlantImage{
			ID: uint(img.ID), PlantID: p.ID, ImagePath: "/" + img.Path,
			ImageDescription: img.Description, ImageOrder: img.Order,
			ImageDate: taken, CreatedAt: taken, UpdatedAt: taken,
		})
	}
	if len(p.Images) > 0 {
		p.LatestImage = p.Images[0]
	} else {
		p.LatestImage = types.PlantImage{
			PlantID: p.ID, ImagePath: "/static/img/winston.hat.jpg",
			ImageDescription: "Placeholder", ImageOrder: DefaultPlantImageOrder,
			ImageDate: now, CreatedAt: now, UpdatedAt: now,
		}
	}
	return p
}

func toTypesMeasurement(m service.Measurement) types.Measurement {
	return types.Measurement{ID: uint(m.ID), MetricID: m.Metric.ID, Name: m.Metric.Name, Value: m.Value, Date: m.At}
}

// GetAdjacentPlantIDs returns the previous and next plant for the plant
// page's arrows; 0 means none.
func GetAdjacentPlantIDs(ctx context.Context, svc *service.Services, id int) (prev, next int) {
	prev, next, err := svc.Plants.Adjacent(ctx, id)
	if err != nil {
		logger.Log.WithError(err).WithField("plant", id).Error("Failed to find adjacent plants")
		return 0, 0
	}
	return prev, next
}
