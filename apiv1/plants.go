package apiv1

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
)

// RecentLimit is how many activities and measurements a plant's detail
// includes.
const RecentLimit = 10

type refJSON struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func toRef(r *service.Ref) *refJSON {
	if r == nil {
		return nil
	}
	return &refJSON{ID: r.ID, Name: r.Name}
}

type plantStrainJSON struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Breeder    refJSON `json:"breeder"`
	URL        string  `json:"url"`
	Autoflower bool    `json:"autoflower"`
	CycleDays  *int    `json:"cycle_days"`
}

type plantStatusJSON struct {
	ID    int       `json:"id"`
	Name  string    `json:"name"`
	Phase string    `json:"phase"`
	Since time.Time `json:"since"`
}

type ageJSON struct {
	Day  int `json:"day"`
	Week int `json:"week"`
}

type flowerJSON struct {
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Days      int        `json:"days"`
}

type plantJSON struct {
	ID                   int              `json:"id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	Clone                bool             `json:"clone"`
	Parent               *refJSON         `json:"parent"`
	Strain               *plantStrainJSON `json:"strain"`
	Zone                 *refJSON         `json:"zone"`
	StartDate            *chrono.Date     `json:"start_date"`
	Age                  *ageJSON         `json:"age"`
	Status               *plantStatusJSON `json:"status"`
	HarvestWeightG       float64          `json:"harvest_weight_g"`
	LastWateredAt        *time.Time       `json:"last_watered_at"`
	DaysSinceWatered     *int             `json:"days_since_watered"`
	LastFedAt            *time.Time       `json:"last_fed_at"`
	DaysSinceFed         *int             `json:"days_since_fed"`
	Flower               *flowerJSON      `json:"flower"`
	HarvestedAt          *time.Time       `json:"harvested_at"`
	EstimatedHarvestDate *chrono.Date     `json:"estimated_harvest_date"`
}

func optTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := apiTime(*t)
	return &v
}

func toPlantJSON(p service.Plant) plantJSON {
	out := plantJSON{
		ID: p.ID, Name: p.Name, Description: p.Description, Clone: p.Clone,
		Parent: toRef(p.Parent), Zone: toRef(p.Zone),
		HarvestWeightG:       p.HarvestWeightG,
		LastWateredAt:        optTime(p.LastWatered),
		DaysSinceWatered:     p.DaysSinceWatered,
		LastFedAt:            optTime(p.LastFed),
		DaysSinceFed:         p.DaysSinceFed,
		HarvestedAt:          optTime(p.HarvestedAt),
		EstimatedHarvestDate: p.EstimatedHarvest,
	}
	if p.Strain != nil {
		out.Strain = &plantStrainJSON{
			ID: p.Strain.ID, Name: p.Strain.Name,
			Breeder: refJSON{ID: p.Strain.Breeder.ID, Name: p.Strain.Breeder.Name},
			URL:     p.Strain.URL, Autoflower: p.Strain.Autoflower,
		}
		if p.Strain.CycleDays > 0 {
			days := p.Strain.CycleDays
			out.Strain.CycleDays = &days
		}
	}
	if !p.StartDay.IsZero() {
		d := p.StartDay
		out.StartDate = &d
		out.Age = &ageJSON{Day: p.Day, Week: p.Week}
	}
	if p.Status != nil {
		out.Status = &plantStatusJSON{
			ID: p.Status.StatusID, Name: string(p.Status.Status),
			Phase: statusPhase(p.Status.Status), Since: apiTime(p.Status.At),
		}
	}
	if p.Flower != nil {
		out.Flower = &flowerJSON{StartedAt: apiTime(p.Flower.Start), EndedAt: optTime(p.Flower.End), Days: p.Flower.Days}
	}
	return out
}

type plantCursor struct {
	AfterID int `json:"after_id"`
}

// queryID reads an optional positive id from the query string.
func queryID(c *gin.Context, name string, problems map[string]string) int {
	raw := c.Query(name)
	if raw == "" {
		return 0
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		problems[name] = "must be a positive whole number"
		return 0
	}
	return id
}

func (a *api) listPlants(c *gin.Context) {
	page, ok := parsePage(c)
	if !ok {
		return
	}
	var cur plantCursor
	if !decodeCursor(c, page, &cur) {
		return
	}
	problems := map[string]string{}
	f := service.PlantFilter{
		ZoneID:   queryID(c, "zone_id", problems),
		StrainID: queryID(c, "strain_id", problems),
		StatusID: queryID(c, "status_id", problems),
		AfterID:  cur.AfterID,
		Limit:    page.Limit,
	}
	switch phase := service.PlantPhase(c.Query("phase")); phase {
	case "", service.PhaseGrowing, service.PhaseHarvested, service.PhaseDead:
		f.Phase = phase
	default:
		problems["phase"] = "must be growing, harvested or dead"
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	plants, more, err := a.svc.Plants.List(c.Request.Context(), f)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]plantJSON, len(plants))
	for i, p := range plants {
		out[i] = toPlantJSON(p)
	}
	var next *string
	if more {
		if next, err = encodeCursor(plantCursor{AfterID: plants[len(plants)-1].ID}); err != nil {
			writeError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

type statusEntryJSON struct {
	ID       int       `json:"id"`
	StatusID int       `json:"status_id"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
}

type plantSensorJSON struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Source    string       `json:"source"`
	Type      string       `json:"type"`
	Unit      string       `json:"unit"`
	Inherited bool         `json:"inherited"`
	Latest    *readingJSON `json:"latest"`
	VPDTarget *vpdJSON     `json:"vpd_target"`
}

type vpdJSON struct {
	LowKPa  float64 `json:"low_kpa"`
	HighKPa float64 `json:"high_kpa"`
	State   *string `json:"state"`
}

type measurementJSON struct {
	ID         int       `json:"id"`
	Metric     refJSON   `json:"metric"`
	Unit       string    `json:"unit"`
	Value      float64   `json:"value"`
	At         time.Time `json:"at"`
	ActivityID *int      `json:"activity_id"`
}

type activityJSON struct {
	ID           int               `json:"id"`
	Plant        refJSON           `json:"plant"`
	ActivityType refJSON           `json:"activity_type"`
	Note         string            `json:"note"`
	At           time.Time         `json:"at"`
	IsWatering   bool              `json:"is_watering"`
	IsFeeding    bool              `json:"is_feeding"`
	Measurements []measurementJSON `json:"measurements"`
}

type stageJSON struct {
	StatusID int    `json:"status_id"`
	Name     string `json:"name"`
	Days     int    `json:"days"`
}

type plantDetailJSON struct {
	plantJSON
	StatusHistory      []statusEntryJSON `json:"status_history"`
	Stages             []stageJSON       `json:"stages"`
	GrowDays           *int              `json:"grow_days"`
	Sensors            []plantSensorJSON `json:"sensors"`
	RecentActivities   []activityJSON    `json:"recent_activities"`
	RecentMeasurements []measurementJSON `json:"recent_measurements"`
}

func toMeasurementJSON(m service.Measurement) measurementJSON {
	return measurementJSON{
		ID: m.ID, Metric: refJSON{ID: m.Metric.ID, Name: m.Metric.Name}, Unit: m.Unit,
		Value: m.Value, At: apiTime(m.At), ActivityID: m.ActivityID,
	}
}

func (a *api) getPlant(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	a.writePlantDetail(c, http.StatusOK, id)
}

func toPlantDetailJSON(d service.PlantDetail) plantDetailJSON {
	out := plantDetailJSON{
		plantJSON:          toPlantJSON(d.Plant),
		StatusHistory:      make([]statusEntryJSON, len(d.StatusHistory)),
		Sensors:            make([]plantSensorJSON, len(d.Sensors)),
		RecentActivities:   make([]activityJSON, len(d.Activities)),
		RecentMeasurements: make([]measurementJSON, len(d.Measurements)),
	}
	out.Stages = make([]stageJSON, len(d.Stages))
	for i, st := range d.Stages {
		out.Stages[i] = stageJSON{StatusID: st.StatusID, Name: string(st.Status), Days: st.Days}
	}
	if !d.StartDay.IsZero() {
		grow := d.GrowDays
		out.GrowDays = &grow
	}
	for i, e := range d.StatusHistory {
		out.StatusHistory[i] = statusEntryJSON{ID: e.ID, StatusID: e.StatusID, Name: string(e.Status), At: apiTime(e.At)}
	}
	for i, s := range d.Sensors {
		js := plantSensorJSON{ID: s.ID, Name: s.Name, Source: s.Source, Type: s.Type, Unit: s.Unit, Inherited: s.Inherited}
		if s.Latest != nil {
			js.Latest = &readingJSON{Value: s.Latest.Value, At: apiTime(s.Latest.At)}
		}
		if s.VPD != nil {
			js.VPDTarget = &vpdJSON{LowKPa: s.VPD.LowKPa, HighKPa: s.VPD.HighKPa}
			if s.VPD.State != "" {
				state := s.VPD.State
				js.VPDTarget.State = &state
			}
		}
		out.Sensors[i] = js
	}
	for i, act := range d.Activities {
		out.RecentActivities[i] = toActivityJSON(act, refJSON{ID: d.ID, Name: d.Name})
	}
	for i, m := range d.Measurements {
		out.RecentMeasurements[i] = toMeasurementJSON(m)
	}
	return out
}
