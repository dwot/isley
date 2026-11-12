package mcpserver

import (
	"time"

	"isley/service"
)

// The views are what tools return: compact, with empty fields left out,
// times as RFC 3339 strings in the instance zone, and units in the field
// names, as in /api/v1.

type view struct {
	loc *time.Location
}

func (t *tools) view() view { return view{loc: t.svc.TimeZones().Instance()} }

func (v view) time(t time.Time) string {
	return t.In(v.loc).Truncate(time.Second).Format(time.RFC3339)
}

func (v view) timePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return v.time(*t)
}

type plantSummary struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Strain           string  `json:"strain,omitempty"`
	StrainID         int     `json:"strain_id,omitempty"`
	Breeder          string  `json:"breeder,omitempty"`
	Zone             string  `json:"zone,omitempty"`
	ZoneID           int     `json:"zone_id,omitempty"`
	Status           string  `json:"status,omitempty"`
	StatusSince      string  `json:"status_since,omitempty"`
	StartDate        string  `json:"start_date,omitempty"`
	Day              int     `json:"day,omitempty"`
	Week             int     `json:"week,omitempty"`
	FlowerDay        int     `json:"flower_day,omitempty"`
	DaysSinceWatered *int    `json:"days_since_watered,omitempty"`
	DaysSinceFed     *int    `json:"days_since_fed,omitempty"`
	LastWatered      string  `json:"last_watered,omitempty"`
	LastFed          string  `json:"last_fed,omitempty"`
	EstimatedHarvest string  `json:"estimated_harvest,omitempty"`
	Harvested        string  `json:"harvested,omitempty"`
	HarvestWeightG   float64 `json:"harvest_weight_g,omitempty"`
}

func (v view) plant(p service.Plant) plantSummary {
	out := plantSummary{
		ID: p.ID, Name: p.Name, StartDate: p.StartDay.String(), Day: p.Day, Week: p.Week,
		DaysSinceWatered: p.DaysSinceWatered, DaysSinceFed: p.DaysSinceFed,
		LastWatered: v.timePtr(p.LastWatered), LastFed: v.timePtr(p.LastFed),
		Harvested: v.timePtr(p.HarvestedAt), HarvestWeightG: p.HarvestWeightG,
	}
	if p.Strain != nil {
		out.Strain, out.StrainID, out.Breeder = p.Strain.Name, p.Strain.ID, p.Strain.Breeder.Name
	}
	if p.Zone != nil {
		out.Zone, out.ZoneID = p.Zone.Name, p.Zone.ID
	}
	if p.Status != nil {
		out.Status, out.StatusSince = string(p.Status.Status), v.time(p.Status.At)
	}
	if p.Flower != nil && p.Flower.End == nil {
		out.FlowerDay = p.Flower.Days
	}
	if p.EstimatedHarvest != nil {
		out.EstimatedHarvest = p.EstimatedHarvest.String()
	}
	if p.HarvestedAt != nil {
		// Age stops meaning much once it's cut.
		out.DaysSinceWatered, out.DaysSinceFed = nil, nil
	}
	return out
}

type statusView struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type stageView struct {
	Status string `json:"status"`
	Days   int    `json:"days"`
}

type measurementView struct {
	ID     int     `json:"id"`
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit,omitempty"`
	At     string  `json:"at,omitempty"`
}

func (v view) measurement(m service.Measurement, withTime bool) measurementView {
	out := measurementView{ID: m.ID, Metric: m.Metric.Name, Value: m.Value, Unit: m.Unit}
	if withTime {
		out.At = v.time(m.At)
	}
	return out
}

type activityView struct {
	ID           int               `json:"id"`
	Type         string            `json:"type"`
	At           string            `json:"at"`
	Note         string            `json:"note,omitempty"`
	Plant        string            `json:"plant,omitempty"`
	PlantID      int               `json:"plant_id,omitempty"`
	Measurements []measurementView `json:"measurements,omitempty"`
}

func (v view) activity(a service.PlantActivity) activityView {
	out := activityView{ID: a.ID, Type: a.Type.Name, At: v.time(a.At), Note: a.Note}
	for _, m := range a.Measurements {
		out.Measurements = append(out.Measurements, v.measurement(m, false))
	}
	return out
}

type readingView struct {
	Value float64 `json:"value"`
	At    string  `json:"at"`
}

func (v view) reading(r *service.Reading) *readingView {
	if r == nil {
		return nil
	}
	return &readingView{Value: round(r.Value), At: v.time(r.At)}
}

type sensorView struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	Type      string       `json:"type,omitempty"`
	Unit      string       `json:"unit,omitempty"`
	Source    string       `json:"source,omitempty"`
	Zone      string       `json:"zone,omitempty"`
	Inherited bool         `json:"from_zone,omitempty"`
	Latest    *readingView `json:"latest,omitempty"`
	VPDState  string       `json:"vpd_state,omitempty"`
	VPDTarget []float64    `json:"vpd_target_kpa,omitempty"`
}

type plantDetail struct {
	plantSummary
	Description   string            `json:"description,omitempty"`
	Autoflower    bool              `json:"autoflower,omitempty"`
	CycleDays     int               `json:"strain_cycle_days,omitempty"`
	Clone         bool              `json:"clone,omitempty"`
	Parent        string            `json:"parent,omitempty"`
	GrowDays      int               `json:"grow_days,omitempty"`
	StatusHistory []statusView      `json:"status_history,omitempty"`
	Stages        []stageView       `json:"stages,omitempty"`
	Sensors       []sensorView      `json:"sensors,omitempty"`
	Activities    []activityView    `json:"recent_activities,omitempty"`
	Measurements  []measurementView `json:"recent_measurements,omitempty"`
	Images        int               `json:"images,omitempty"`
}

func (v view) plantDetail(d service.PlantDetail) plantDetail {
	out := plantDetail{
		plantSummary: v.plant(d.Plant), Description: d.Description, Clone: d.Clone,
		GrowDays: d.GrowDays, Images: len(d.Images),
	}
	if d.Strain != nil {
		out.Autoflower, out.CycleDays = d.Strain.Autoflower, d.Strain.CycleDays
	}
	if d.Parent != nil {
		out.Parent = d.Parent.Name
	}
	for _, s := range d.StatusHistory {
		out.StatusHistory = append(out.StatusHistory, statusView{Status: string(s.Status), At: v.time(s.At)})
	}
	for _, s := range d.Stages {
		out.Stages = append(out.Stages, stageView{Status: string(s.Status), Days: s.Days})
	}
	for _, s := range d.Sensors {
		sv := sensorView{ID: s.ID, Name: s.Name, Type: s.Type, Unit: s.Unit, Source: s.Source,
			Inherited: s.Inherited, Latest: v.reading(s.Latest)}
		if s.VPD != nil {
			sv.VPDState, sv.VPDTarget = s.VPD.State, []float64{s.VPD.LowKPa, s.VPD.HighKPa}
		}
		out.Sensors = append(out.Sensors, sv)
	}
	for _, a := range d.Activities {
		out.Activities = append(out.Activities, v.activity(a))
	}
	for _, m := range d.Measurements {
		out.Measurements = append(out.Measurements, v.measurement(m, true))
	}
	return out
}

type zoneView struct {
	ID      int          `json:"id"`
	Name    string       `json:"name"`
	Sensors []sensorView `json:"sensors"`
}

// zone shows a zone's sensors, without the hidden ones.
func (v view) zone(d service.ZoneDetail, withSource bool) zoneView {
	out := zoneView{ID: d.ID, Name: d.Name, Sensors: []sensorView{}}
	for _, s := range d.Sensors {
		if s.Visibility == "hide" {
			continue
		}
		sv := sensorView{ID: s.ID, Name: s.Name, Type: s.Type, Unit: s.Unit, Latest: v.reading(s.Latest)}
		if withSource {
			sv.Source = s.Source
		}
		out.Sensors = append(out.Sensors, sv)
	}
	return out
}

type strainSummary struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Breeder       string `json:"breeder"`
	BreederID     int    `json:"breeder_id"`
	Autoflower    bool   `json:"autoflower"`
	Feminized     bool   `json:"feminized"`
	IndicaPct     int    `json:"indica_pct"`
	SativaPct     int    `json:"sativa_pct"`
	SeedCount     int    `json:"seed_count"`
	CycleTimeDays *int   `json:"cycle_time_days,omitempty"`
	Summary       string `json:"short_description,omitempty"`
}

func strain(s service.Strain) strainSummary {
	return strainSummary{
		ID: s.ID, Name: s.Name, Breeder: s.Breeder.Name, BreederID: s.Breeder.ID, Autoflower: s.Autoflower,
		Feminized: s.Feminized, IndicaPct: s.IndicaPct, SativaPct: s.SativaPct, SeedCount: s.SeedCount, CycleTimeDays: s.CycleTimeDays,
		Summary: s.ShortDescription,
	}
}

type ancestor struct {
	Name     string     `json:"name"`
	StrainID int        `json:"strain_id,omitempty"`
	Parents  []ancestor `json:"parents,omitempty"`
}

func ancestry(nodes []service.AncestryNode) []ancestor {
	var out []ancestor
	for _, n := range nodes {
		a := ancestor{Name: n.ParentName, Parents: ancestry(n.Parents)}
		if n.Parent != nil {
			a.StrainID = n.Parent.ID
			if a.Name == "" {
				a.Name = n.Parent.Name
			}
		}
		out = append(out, a)
	}
	return out
}

type descendant struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Breeder      string   `json:"breeder"`
	OtherParents []string `json:"other_parents,omitempty"`
}
