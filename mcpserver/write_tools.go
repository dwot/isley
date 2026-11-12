package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isley/chrono"
	"isley/scope"
	"isley/service"
)

func (t *tools) addWriteTools(s *mcp.Server) {
	addTool(s, &mcp.Tool{
		Name: "create_plant",
		Description: "Start a plant from a strain. status is the status it starts in (by name, e.g. Seedling; see " +
			"list_reference_data). start_date (YYYY-MM-DD) is day 1 of its age and defaults to today. Set " +
			"decrement_seed_count to take a seed off the strain's stock.",
	}, scope.Write, t.createPlant)

	addTool(s, &mcp.Tool{
		Name: "log_activity",
		Description: "Log something done to one or more plants: watering, feeding, training, a note. activity is " +
			"an activity type by name (Water, Feed, Note, ...; see list_reference_data). at defaults to now; " +
			"without an offset it is in the instance zone. measurements records amounts with it, by metric " +
			"name, in the metric's unit (e.g. Water amount in its unit).",
	}, scope.Write, t.logActivity)

	addTool(s, &mcp.Tool{
		Name: "record_measurement",
		Description: "Record a measurement of a plant (height, pH, EC, ...) by metric name, in the metric's unit " +
			"(see list_reference_data). at defaults to now.",
	}, scope.Write, t.recordMeasurement)

	addTool(s, &mcp.Tool{
		Name: "set_plant_status",
		Description: "Move a plant to a new status (Veg, Flower, Drying, Curing, Success, Dead, ...). at is when " +
			"it changed and defaults to now. When harvesting, harvest_weight_g records the dry weight in grams. " +
			"Setting the status a plant already has changes nothing.",
	}, scope.Write, t.setPlantStatus)

	addTool(s, &mcp.Tool{
		Name: "update_plant",
		Description: "Change a plant's details. Only the fields given change. start_date (YYYY-MM-DD) moves day 1 " +
			"of its age; harvest_weight_g is in grams.",
	}, scope.Write, t.updatePlant)

	addTool(s, &mcp.Tool{
		Name: "adjust_seed_count",
		Description: "Add seeds to a strain's stock (positive delta, e.g. a new pack) or take them off (negative). " +
			"The count never goes below 0. Say why in reason; it goes in the log.",
	}, scope.Write, t.adjustSeedCount)

	addTool(s, &mcp.Tool{
		Name: "link_plant_sensors",
		Description: "Set the sensors linked directly to a plant (a soil probe, say), replacing the current set; " +
			"an empty list unlinks them all. Zone sensors apply to every plant in the zone without linking.",
	}, scope.Write, t.linkPlantSensors)
}

// ------------------------------------------------------------ name lookup

// status finds a status by name, ignoring case.
func (t *tools) status(ctx context.Context, name string) (service.Status, error) {
	all, err := t.svc.Lookups.Statuses(ctx)
	if err != nil {
		return service.Status{}, err
	}
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = string(s.Name)
		if strings.EqualFold(string(s.Name), strings.TrimSpace(name)) {
			return s, nil
		}
	}
	return service.Status{}, unknown("status", name, names)
}

func (t *tools) activityType(ctx context.Context, name string) (service.ActivityType, error) {
	all, err := t.svc.Lookups.ActivityTypes(ctx)
	if err != nil {
		return service.ActivityType{}, err
	}
	names := make([]string, len(all))
	for i, a := range all {
		names[i] = a.Name
		if strings.EqualFold(a.Name, strings.TrimSpace(name)) {
			return a, nil
		}
	}
	return service.ActivityType{}, unknown("activity", name, names)
}

func (t *tools) metric(ctx context.Context, name string) (service.Metric, error) {
	all, err := t.svc.Lookups.Metrics(ctx)
	if err != nil {
		return service.Metric{}, err
	}
	names := make([]string, len(all))
	for i, m := range all {
		names[i] = m.Name
		if strings.EqualFold(m.Name, strings.TrimSpace(name)) {
			return m, nil
		}
	}
	return service.Metric{}, unknown("metric", name, names)
}

func unknown(field, name string, names []string) error {
	if strings.TrimSpace(name) == "" {
		return service.Invalid(map[string]string{field: "is required; one of " + strings.Join(names, ", ")})
	}
	return service.Invalid(map[string]string{field: fmt.Sprintf("%q is not one of %s", name, strings.Join(names, ", "))})
}

// at reads an optional date-time.
func (t *tools) at(field, raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	v, err := t.svc.TimeZones().ParseInput(raw)
	if err != nil {
		return time.Time{}, service.Invalid(map[string]string{field: "must be a date-time such as 2026-11-11T09:30 or 2026-11-11T09:30:00-05:00"})
	}
	return v, nil
}

func date(field, raw string) (chrono.Date, error) {
	d, err := chrono.ParseDate(strings.TrimSpace(raw))
	if err != nil {
		return chrono.Date{}, service.Invalid(map[string]string{field: "must be a date such as 2026-11-11"})
	}
	return d, nil
}

// ------------------------------------------------------------------ plants

type createPlantInput struct {
	Name               string `json:"name"`
	StrainID           int    `json:"strain_id"`
	Status             string `json:"status" jsonschema:"the status it starts in, by name, e.g. Seedling"`
	StartDate          string `json:"start_date,omitempty" jsonschema:"YYYY-MM-DD; default today"`
	ZoneID             int    `json:"zone_id,omitempty"`
	Description        string `json:"description,omitempty"`
	Clone              bool   `json:"clone,omitempty"`
	ParentID           int    `json:"parent_id,omitempty" jsonschema:"the mother plant, for a clone"`
	DecrementSeedCount bool   `json:"decrement_seed_count,omitempty" jsonschema:"take one seed off the strain's stock"`
}

func (t *tools) createPlant(ctx context.Context, in createPlantInput) (result, error) {
	st, err := t.status(ctx, in.Status)
	if err != nil {
		return result{}, err
	}
	start := t.svc.TimeZones().Today(t.svc.Clock())
	if in.StartDate != "" {
		if start, err = date("start_date", in.StartDate); err != nil {
			return result{}, err
		}
	}
	pin := service.PlantInput{
		Name: in.Name, Description: in.Description, StrainID: in.StrainID, StatusID: st.ID,
		Start: service.OnDay(start), Clone: in.Clone, DecrementSeedCount: in.DecrementSeedCount,
	}
	if in.ZoneID != 0 {
		pin.ZoneID = &in.ZoneID
	}
	if in.ParentID != 0 {
		pin.ParentID = &in.ParentID
	}
	p, err := t.svc.Plants.Create(ctx, pin)
	if err != nil {
		return result{}, err
	}
	out := t.view().plant(p)
	return result{summary: "Created plant " + strconv.Itoa(p.ID) + ": " + plantLine(out), data: out}, nil
}

type updatePlantInput struct {
	PlantID        int      `json:"plant_id"`
	Name           *string  `json:"name,omitempty"`
	Description    *string  `json:"description,omitempty"`
	StrainID       *int     `json:"strain_id,omitempty"`
	ZoneID         *int     `json:"zone_id,omitempty"`
	StartDate      *string  `json:"start_date,omitempty" jsonschema:"YYYY-MM-DD"`
	HarvestWeightG *float64 `json:"harvest_weight_g,omitempty"`
	Clone          *bool    `json:"clone,omitempty"`
	ParentID       *int     `json:"parent_id,omitempty"`
}

func (t *tools) updatePlant(ctx context.Context, in updatePlantInput) (result, error) {
	upd := service.PlantUpdate{}
	if in.Name != nil {
		upd.Name = service.Put(in.Name)
	}
	if in.Description != nil {
		upd.Description = service.Put(in.Description)
	}
	if in.StrainID != nil {
		upd.StrainID = service.Put(in.StrainID)
	}
	if in.ZoneID != nil {
		upd.ZoneID = service.Put(in.ZoneID)
	}
	if in.HarvestWeightG != nil {
		upd.HarvestWeightG = service.Put(in.HarvestWeightG)
	}
	if in.Clone != nil {
		upd.Clone = service.Put(in.Clone)
	}
	if in.ParentID != nil {
		upd.ParentID = service.Put(in.ParentID)
	}
	if in.StartDate != nil {
		d, err := date("start_date", *in.StartDate)
		if err != nil {
			return result{}, err
		}
		w := service.OnDay(d)
		upd.Start = service.Put(&w)
	}
	p, err := t.svc.Plants.Update(ctx, in.PlantID, upd)
	if err != nil {
		return result{}, err
	}
	out := t.view().plant(p)
	return result{summary: "Updated: " + plantLine(out), data: out}, nil
}

type setStatusInput struct {
	PlantID        int      `json:"plant_id"`
	Status         string   `json:"status" jsonschema:"the new status, by name, e.g. Flower"`
	At             string   `json:"at,omitempty" jsonschema:"when it changed; default now"`
	HarvestWeightG *float64 `json:"harvest_weight_g,omitempty" jsonschema:"dry weight in grams, when harvesting"`
}

func (t *tools) setPlantStatus(ctx context.Context, in setStatusInput) (result, error) {
	st, err := t.status(ctx, in.Status)
	if err != nil {
		return result{}, err
	}
	at, err := t.at("at", in.At)
	if err != nil {
		return result{}, err
	}
	var atPtr *time.Time
	if !at.IsZero() {
		atPtr = &at
	}
	_, changed, err := t.svc.Plants.ChangeStatus(ctx, in.PlantID, st.ID, atPtr)
	if err != nil {
		return result{}, err
	}
	if in.HarvestWeightG != nil {
		if _, err := t.svc.Plants.Update(ctx, in.PlantID,
			service.PlantUpdate{HarvestWeightG: service.Put(in.HarvestWeightG)}); err != nil {
			return result{}, err
		}
	}
	p, err := t.svc.Plants.Get(ctx, in.PlantID)
	if err != nil {
		return result{}, err
	}
	out := t.view().plant(p)
	summary := "Status changed: " + plantLine(out)
	if !changed {
		summary = "Already " + string(st.Name) + "; nothing changed. " + plantLine(out)
	}
	return result{summary: summary, data: out}, nil
}

type linkSensorsInput struct {
	PlantID   int   `json:"plant_id"`
	SensorIDs []int `json:"sensor_ids" jsonschema:"the complete set of sensors to link; [] unlinks all"`
}

func (t *tools) linkPlantSensors(ctx context.Context, in linkSensorsInput) (result, error) {
	if err := t.svc.Plants.SetSensors(ctx, in.PlantID, in.SensorIDs); err != nil {
		return result{}, err
	}
	d, err := t.svc.Plants.Detail(ctx, in.PlantID, 0)
	if err != nil {
		return result{}, err
	}
	out := t.view().plantDetail(d).Sensors
	return result{
		summary: fmt.Sprintf("%s now has %s linked.", d.Name, count(len(in.SensorIDs), "sensor")),
		data:    map[string]any{"plant_id": d.ID, "sensors": out},
	}, nil
}

// -------------------------------------------------------------- activities

type measurementArg struct {
	Metric string  `json:"metric" jsonschema:"metric name, e.g. Water amount"`
	Value  float64 `json:"value" jsonschema:"in the metric's unit"`
}

type logActivityInput struct {
	PlantIDs     []int            `json:"plant_ids" jsonschema:"the plants it was done to (one or more)"`
	Activity     string           `json:"activity" jsonschema:"activity type by name, e.g. Water"`
	Note         string           `json:"note,omitempty"`
	At           string           `json:"at,omitempty" jsonschema:"when; default now"`
	Measurements []measurementArg `json:"measurements,omitempty"`
}

func (t *tools) logActivity(ctx context.Context, in logActivityInput) (result, error) {
	if len(in.PlantIDs) == 0 {
		return result{}, service.Invalid(map[string]string{"plant_ids": "give at least one plant"})
	}
	typ, err := t.activityType(ctx, in.Activity)
	if err != nil {
		return result{}, err
	}
	at, err := t.at("at", in.At)
	if err != nil {
		return result{}, err
	}
	ain := service.ActivityInput{TypeID: typ.ID, Note: in.Note, At: at}
	for _, m := range in.Measurements {
		met, err := t.metric(ctx, m.Metric)
		if err != nil {
			return result{}, err
		}
		ain.Measurements = append(ain.Measurements, service.MeasurementValue{MetricID: met.ID, Value: m.Value})
	}
	var entries []service.ActivityEntry
	if len(in.PlantIDs) == 1 {
		e, err := t.svc.Activities.Create(ctx, in.PlantIDs[0], ain)
		if err != nil {
			return result{}, err
		}
		entries = []service.ActivityEntry{e}
	} else if entries, err = t.svc.Activities.CreateBulk(ctx, in.PlantIDs, ain); err != nil {
		return result{}, err
	}
	v := t.view()
	out := make([]activityView, len(entries))
	names := make([]string, len(entries))
	for i, e := range entries {
		out[i] = v.activity(e.PlantActivity)
		out[i].Plant, out[i].PlantID = e.Plant.Name, e.Plant.ID
		names[i] = e.Plant.Name
	}
	when := ""
	if len(out) > 0 {
		when = " at " + out[0].At
	}
	return result{
		summary: fmt.Sprintf("Logged %s for %s%s.", typ.Name, strings.Join(names, ", "), when),
		data:    out,
	}, nil
}

type recordMeasurementInput struct {
	PlantID int     `json:"plant_id"`
	Metric  string  `json:"metric" jsonschema:"metric name, e.g. Height"`
	Value   float64 `json:"value" jsonschema:"in the metric's unit"`
	At      string  `json:"at,omitempty" jsonschema:"when; default now"`
}

func (t *tools) recordMeasurement(ctx context.Context, in recordMeasurementInput) (result, error) {
	met, err := t.metric(ctx, in.Metric)
	if err != nil {
		return result{}, err
	}
	at, err := t.at("at", in.At)
	if err != nil {
		return result{}, err
	}
	m, err := t.svc.Measurements.Create(ctx, in.PlantID, service.MeasurementInput{MetricID: met.ID, Value: in.Value, At: at})
	if err != nil {
		return result{}, err
	}
	out := t.view().measurement(m, true)
	return result{
		summary: fmt.Sprintf("Recorded %s %v%s for plant %d at %s.", out.Metric, out.Value, unitSuffix(out.Unit),
			in.PlantID, out.At),
		data: out,
	}, nil
}

func unitSuffix(u string) string {
	if u == "" {
		return ""
	}
	return " " + u
}

// ----------------------------------------------------------------- strains

type adjustSeedsInput struct {
	StrainID int    `json:"strain_id"`
	Delta    int    `json:"delta" jsonschema:"seeds to add (positive) or take off (negative)"`
	Reason   string `json:"reason,omitempty" jsonschema:"why, for the log"`
}

func (t *tools) adjustSeedCount(ctx context.Context, in adjustSeedsInput) (result, error) {
	if in.Delta == 0 {
		return result{}, service.Invalid(map[string]string{"delta": "must not be 0"})
	}
	s, err := t.svc.Strains.AdjustSeeds(ctx, in.StrainID, in.Delta)
	if err != nil {
		return result{}, err
	}
	return result{
		summary: fmt.Sprintf("%s now has %s.", s.Name, count(s.SeedCount, "seed")),
		data:    strain(s),
	}, nil
}
