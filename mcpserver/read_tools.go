package mcpserver

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"isley/scope"
	"isley/service"
)

func (t *tools) addReadTools(s *mcp.Server) {
	addTool(s, &mcp.Tool{
		Name: "get_overview",
		Description: "What is going on now: every growing plant (strain, zone, status, age as day/week, " +
			"day of flower, days since last watered and fed) and every zone with the latest reading of each " +
			"sensor. Also the current time and the instance's time zone. Start here.",
	}, scope.Read, t.getOverview)

	addTool(s, &mcp.Tool{
		Name: "list_plants",
		Description: "List plants. phase is growing (default), harvested, dead or all. Narrow by status name, " +
			"zone_id or strain_id.",
	}, scope.Read, t.listPlants)

	addTool(s, &mcp.Tool{
		Name: "get_plant",
		Description: "One plant in full: strain, zone, status history, days spent in each stage, linked and " +
			"zone sensors with latest readings, and its most recent activities and measurements.",
	}, scope.Read, t.getPlant)

	addTool(s, &mcp.Tool{
		Name: "find_strains",
		Description: "Find strains by part of the name and/or breeder, optionally only those with seeds in stock. " +
			"Returns seed counts, indica/sativa split, autoflower, feminized or regular, and cycle time (days of flower for a " +
			"photoperiod strain, seed to harvest for an autoflower).",
	}, scope.Read, t.findStrains)

	addTool(s, &mcp.Tool{
		Name:        "get_strain",
		Description: "One strain in full: description, its ancestry as far as it is known, and strains bred from it.",
	}, scope.Read, t.getStrain)

	addTool(s, &mcp.Tool{
		Name: "list_zones",
		Description: "Zones (tents, rooms) with their sensors and each sensor's latest reading. Use the sensor ids " +
			"with get_sensor_history.",
	}, scope.Read, t.listZones)

	addTool(s, &mcp.Tool{
		Name: "get_sensor_history",
		Description: "How sensors did over a time range, summarised: per sensor the overall min/avg/max, the " +
			"first and last values, and a series of buckets (min/avg/max per bucket). On/off sensors (fans, " +
			"lights, pumps) also give the share of time on and the number of starts. Pick sensors by id, or " +
			"all of a zone's sensors with zone_id. The range defaults to the last 24 hours; give from/to " +
			"(date-times; without an offset they are in the instance zone) or hours. The bucket size is " +
			"chosen for about 24 buckets unless given (e.g. 15m, 1h, 1d).",
	}, scope.Read, t.getSensorHistory)

	addTool(s, &mcp.Tool{
		Name: "list_reference_data",
		Description: "The names the other tools take: plant statuses in lifecycle order, activity types (with " +
			"the measurements each one records), and measurement metrics with their units.",
	}, scope.Read, t.listReferenceData)
}

type noInput struct{}

// ---------------------------------------------------------------- overview

type overview struct {
	Now      string         `json:"now"`
	TimeZone string         `json:"time_zone"`
	Plants   []plantSummary `json:"growing_plants"`
	Zones    []zoneView     `json:"zones"`
}

func (t *tools) getOverview(ctx context.Context, _ noInput) (result, error) {
	v := t.view()
	plants, _, err := t.svc.Plants.List(ctx, service.PlantFilter{Phase: service.PhaseGrowing})
	if err != nil {
		return result{}, err
	}
	zones, err := t.zoneDetails(ctx, v, false)
	if err != nil {
		return result{}, err
	}
	out := overview{
		Now:      v.time(t.svc.TimeZones().Now(t.svc.Clock())),
		TimeZone: t.svc.TimeZones().Instance().String(),
		Plants:   make([]plantSummary, len(plants)),
		Zones:    zones,
	}
	for i, p := range plants {
		out.Plants[i] = v.plant(p)
	}
	return result{
		summary: fmt.Sprintf("%s growing, %s. Now %s (%s).", count(len(plants), "plant"), count(len(zones), "zone"),
			out.Now, out.TimeZone),
		data: out,
	}, nil
}

func (t *tools) zoneDetails(ctx context.Context, v view, withSource bool) ([]zoneView, error) {
	zones, err := t.svc.Zones.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]zoneView, 0, len(zones))
	for _, z := range zones {
		d, err := t.svc.Zones.Detail(ctx, z.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, v.zone(d, withSource))
	}
	return out, nil
}

// ------------------------------------------------------------------ plants

type listPlantsInput struct {
	Phase    string `json:"phase,omitempty" jsonschema:"growing (default), harvested, dead or all"`
	Status   string `json:"status,omitempty" jsonschema:"only plants whose current status has this name, e.g. Flower"`
	ZoneID   int    `json:"zone_id,omitempty"`
	StrainID int    `json:"strain_id,omitempty"`
}

// maxListedPlants keeps a list call to a sensible size for an agent.
const maxListedPlants = 200

func (t *tools) listPlants(ctx context.Context, in listPlantsInput) (result, error) {
	f := service.PlantFilter{ZoneID: in.ZoneID, StrainID: in.StrainID, Limit: maxListedPlants}
	switch strings.ToLower(strings.TrimSpace(in.Phase)) {
	case "", "growing":
		f.Phase = service.PhaseGrowing
	case "harvested":
		f.Phase = service.PhaseHarvested
	case "dead":
		f.Phase = service.PhaseDead
	case "all":
	default:
		return result{}, service.Invalid(map[string]string{"phase": "must be growing, harvested, dead or all"})
	}
	if in.Status != "" {
		st, err := t.status(ctx, in.Status)
		if err != nil {
			return result{}, err
		}
		f.StatusID = st.ID
	}
	plants, more, err := t.svc.Plants.List(ctx, f)
	if err != nil {
		return result{}, err
	}
	v := t.view()
	out := make([]plantSummary, len(plants))
	for i, p := range plants {
		out[i] = v.plant(p)
	}
	summary := count(len(plants), "plant") + "."
	if more {
		summary = fmt.Sprintf("The first %d plants; narrow the filter to see the rest.", len(plants))
	}
	return result{summary: summary, data: out}, nil
}

type getPlantInput struct {
	PlantID int `json:"plant_id"`
	Recent  int `json:"recent,omitempty" jsonschema:"how many recent activities and measurements to include (default 10, at most 100)"`
}

func (t *tools) getPlant(ctx context.Context, in getPlantInput) (result, error) {
	n := in.Recent
	if n <= 0 {
		n = 10
	}
	n = min(n, 100)
	d, err := t.svc.Plants.Detail(ctx, in.PlantID, n)
	if err != nil {
		return result{}, err
	}
	out := t.view().plantDetail(d)
	return result{summary: plantLine(out.plantSummary), data: out}, nil
}

// plantLine describes a plant in a sentence.
func plantLine(p plantSummary) string {
	var b strings.Builder
	b.WriteString(p.Name)
	if p.Strain != "" {
		b.WriteString(" (" + p.Strain + ")")
	}
	if p.Status != "" {
		b.WriteString(", " + p.Status)
	}
	if p.Day > 0 && p.Harvested == "" {
		fmt.Fprintf(&b, ", day %d (week %d)", p.Day, p.Week)
	}
	if p.FlowerDay > 0 {
		fmt.Fprintf(&b, ", day %d of flower", p.FlowerDay)
	}
	if p.Zone != "" {
		b.WriteString(", in " + p.Zone)
	}
	b.WriteString(".")
	return b.String()
}

// ----------------------------------------------------------------- strains

type findStrainsInput struct {
	Name    string `json:"name,omitempty" jsonschema:"part of the strain name, any case"`
	Breeder string `json:"breeder,omitempty" jsonschema:"part of the breeder name, any case"`
	InStock bool   `json:"in_stock,omitempty" jsonschema:"only strains with seeds left"`
}

func (t *tools) findStrains(ctx context.Context, in findStrainsInput) (result, error) {
	f := service.StrainFilter{Query: strings.TrimSpace(in.Name)}
	if in.InStock {
		yes := true
		f.InStock = &yes
	}
	strains, _, err := t.svc.Strains.List(ctx, f)
	if err != nil {
		return result{}, err
	}
	breeder := strings.ToLower(strings.TrimSpace(in.Breeder))
	out := []strainSummary{}
	seeds := 0
	for _, s := range strains {
		if breeder != "" && !strings.Contains(strings.ToLower(s.Breeder.Name), breeder) {
			continue
		}
		out = append(out, strain(s))
		seeds += s.SeedCount
	}
	return result{
		summary: fmt.Sprintf("%s, %s in stock.", count(len(out), "strain"), count(seeds, "seed")),
		data:    out,
	}, nil
}

type getStrainInput struct {
	StrainID int `json:"strain_id"`
}

type strainDetail struct {
	strainSummary
	Description string       `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	Ancestry    []ancestor   `json:"parents,omitempty"`
	Descendants []descendant `json:"descendants,omitempty"`
}

func (t *tools) getStrain(ctx context.Context, in getStrainInput) (result, error) {
	s, err := t.svc.Strains.Get(ctx, in.StrainID)
	if err != nil {
		return result{}, err
	}
	nodes, err := t.svc.Strains.Ancestry(ctx, in.StrainID)
	if err != nil {
		return result{}, err
	}
	kids, err := t.svc.Strains.Descendants(ctx, in.StrainID)
	if err != nil {
		return result{}, err
	}
	out := strainDetail{strainSummary: strain(s), Description: s.Description, URL: s.URL, Ancestry: ancestry(nodes)}
	for _, k := range kids {
		out.Descendants = append(out.Descendants, descendant{ID: k.ID, Name: k.Name, Breeder: k.Breeder.Name,
			OtherParents: k.OtherParents})
	}
	kind := "photoperiod"
	if s.Autoflower {
		kind = "autoflower"
	}
	if s.Feminized {
		kind = "feminized " + kind
	} else {
		kind = "regular " + kind
	}
	return result{
		summary: fmt.Sprintf("%s by %s: %s, %s left.", s.Name, s.Breeder.Name, kind, count(s.SeedCount, "seed")),
		data:    out,
	}, nil
}

// ------------------------------------------------------------------- zones

func (t *tools) listZones(ctx context.Context, _ noInput) (result, error) {
	zones, err := t.zoneDetails(ctx, t.view(), true)
	if err != nil {
		return result{}, err
	}
	sensors := 0
	for _, z := range zones {
		sensors += len(z.Sensors)
	}
	return result{summary: fmt.Sprintf("%s, %s.", count(len(zones), "zone"), count(sensors, "sensor")), data: zones}, nil
}

// ----------------------------------------------------------------- sensors

type sensorHistoryInput struct {
	SensorIDs []int  `json:"sensor_ids,omitempty" jsonschema:"the sensors to report on (up to 20)"`
	ZoneID    int    `json:"zone_id,omitempty" jsonschema:"report on every sensor in this zone instead"`
	From      string `json:"from,omitempty" jsonschema:"start of the range, e.g. 2026-11-10T20:00 (instance zone) or with an offset"`
	To        string `json:"to,omitempty" jsonschema:"end of the range; default now"`
	Hours     int    `json:"hours,omitempty" jsonschema:"the range is the last N hours before to (when from is not given; default 24)"`
	Bucket    string `json:"bucket,omitempty" jsonschema:"bucket size such as 15m, 1h, 6h or 1d; default about 24 buckets over the range"`
}

const maxHistorySensors = 20

type historyBucket struct {
	Start      string   `json:"start"`
	Min        float64  `json:"min"`
	Avg        float64  `json:"avg"`
	Max        float64  `json:"max"`
	OnFraction *float64 `json:"on_fraction,omitempty"`
	Starts     *int     `json:"starts,omitempty"`
}

type sensorHistory struct {
	Sensor     sensorView      `json:"sensor"`
	Readings   int             `json:"readings"`
	Min        *float64        `json:"min,omitempty"`
	Avg        *float64        `json:"avg,omitempty"`
	Max        *float64        `json:"max,omitempty"`
	First      *float64        `json:"first,omitempty"`
	Last       *float64        `json:"last,omitempty"`
	OnFraction *float64        `json:"on_fraction,omitempty"`
	Starts     *int            `json:"starts,omitempty"`
	Buckets    []historyBucket `json:"buckets"`
}

type historyResult struct {
	From    string          `json:"from"`
	To      string          `json:"to"`
	Bucket  string          `json:"bucket"`
	Sensors []sensorHistory `json:"sensors"`
}

func (t *tools) getSensorHistory(ctx context.Context, in sensorHistoryInput) (result, error) {
	z := t.svc.TimeZones()
	problems := map[string]string{}
	to := z.Now(t.svc.Clock())
	if in.To != "" {
		if v, err := z.ParseInput(in.To); err != nil {
			problems["to"] = "must be a date-time such as 2026-11-11T09:30"
		} else {
			to = v
		}
	}
	hours := in.Hours
	if hours <= 0 {
		hours = 24
	}
	from := to.Add(-time.Duration(hours) * time.Hour)
	if in.From != "" {
		if v, err := z.ParseInput(in.From); err != nil {
			problems["from"] = "must be a date-time such as 2026-11-10T20:00"
		} else {
			from = v
		}
	}
	size := agentBucket(from, to)
	if in.Bucket != "" {
		var err error
		if size, err = service.ParseBucket(in.Bucket); err != nil {
			problems["bucket"] = "must be a size such as 15m, 1h or 1d"
		}
	}
	ids := in.SensorIDs
	if in.ZoneID == 0 && len(ids) == 0 {
		problems["sensor_ids"] = "give sensor_ids or zone_id"
	}
	if len(ids) > maxHistorySensors {
		problems["sensor_ids"] = fmt.Sprintf("at most %d sensors at a time", maxHistorySensors)
	}
	if len(problems) > 0 {
		return result{}, service.Invalid(problems)
	}
	if in.ZoneID != 0 {
		d, err := t.svc.Zones.Detail(ctx, in.ZoneID)
		if err != nil {
			return result{}, err
		}
		for _, s := range d.Sensors {
			if s.Visibility != "hide" && len(ids) < maxHistorySensors {
				ids = append(ids, s.ID)
			}
		}
	}

	v := t.view()
	out := historyResult{From: v.time(from), To: v.time(to), Bucket: service.FormatBucket(size),
		Sensors: []sensorHistory{}}
	var lines []string
	for _, id := range ids {
		sn, err := t.svc.Sensors.Get(ctx, id)
		if err != nil {
			return result{}, err
		}
		buckets, err := t.svc.Sensors.Buckets(ctx, id, from, to, size)
		if err != nil {
			return result{}, err
		}
		h := summarise(v, buckets)
		h.Sensor = sensorView{ID: sn.ID, Name: sn.Name, Type: sn.Type, Unit: sn.Unit}
		if sn.Zone != nil {
			h.Sensor.Zone = sn.Zone.Name
		}
		out.Sensors = append(out.Sensors, h)
		lines = append(lines, historyLine(h))
	}
	summary := fmt.Sprintf("%s to %s, %s buckets.", out.From, out.To, out.Bucket)
	if len(lines) > 0 {
		summary += " " + strings.Join(lines, "; ") + "."
	}
	return result{summary: summary, data: out}, nil
}

// agentBucket picks a standard bucket size giving about 24 buckets: enough
// to see a night's shape without flooding the agent.
func agentBucket(from, to time.Time) time.Duration {
	span := to.Sub(from)
	for _, d := range []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour,
		2 * time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour} {
		if span/d <= 36 {
			return d
		}
	}
	return 7 * 24 * time.Hour
}

func summarise(v view, buckets []service.Bucket) sensorHistory {
	h := sensorHistory{Buckets: make([]historyBucket, 0, len(buckets))}
	var sum, on float64
	starts, onOff := 0, false
	for _, b := range buckets {
		h.Buckets = append(h.Buckets, historyBucket{Start: v.time(b.Start), Min: round(b.Min), Avg: round(b.Avg),
			Max: round(b.Max), OnFraction: roundPtr(b.OnFraction), Starts: b.Starts})
		if b.Count == 0 {
			continue
		}
		if h.Readings == 0 {
			first := round(b.Avg)
			h.First, h.Min, h.Max = &first, ptr(b.Min), ptr(b.Max)
		}
		last := round(b.Avg)
		h.Last = &last
		*h.Min = math.Min(*h.Min, b.Min)
		*h.Max = math.Max(*h.Max, b.Max)
		h.Readings += b.Count
		sum += b.Avg * float64(b.Count)
		if b.OnFraction != nil {
			onOff = true
			on += *b.OnFraction * float64(b.Count)
		}
		if b.Starts != nil {
			starts += *b.Starts
		}
	}
	if h.Readings > 0 {
		h.Avg = ptr(sum / float64(h.Readings))
		h.Min, h.Max = ptr(*h.Min), ptr(*h.Max)
		if onOff {
			h.OnFraction = ptr(on / float64(h.Readings))
			h.Starts = &starts
		}
	}
	return h
}

func historyLine(h sensorHistory) string {
	name := h.Sensor.Name
	if h.Readings == 0 {
		return name + ": no readings"
	}
	if h.OnFraction != nil {
		return fmt.Sprintf("%s: on %.0f%% of the time, %s", name, *h.OnFraction*100, count(*h.Starts, "start"))
	}
	unit := h.Sensor.Unit
	return fmt.Sprintf("%s: min %v%s, avg %v%s, max %v%s", name, *h.Min, unit, *h.Avg, unit, *h.Max, unit)
}

// ---------------------------------------------------------------- lookups

type referenceData struct {
	Statuses      []statusRef       `json:"statuses"`
	ActivityTypes []activityTypeRef `json:"activity_types"`
	Metrics       []metricRef       `json:"metrics"`
}

type statusRef struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Growing bool   `json:"growing,omitempty"`
}

type activityTypeRef struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Watering bool     `json:"watering,omitempty"`
	Feeding  bool     `json:"feeding,omitempty"`
	Records  []string `json:"records,omitempty"`
}

type metricRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit,omitempty"`
}

func (t *tools) listReferenceData(ctx context.Context, _ noInput) (result, error) {
	statuses, err := t.svc.Lookups.Statuses(ctx)
	if err != nil {
		return result{}, err
	}
	types, err := t.svc.Lookups.ActivityTypes(ctx)
	if err != nil {
		return result{}, err
	}
	metrics, err := t.svc.Lookups.Metrics(ctx)
	if err != nil {
		return result{}, err
	}
	var out referenceData
	for _, s := range statuses {
		out.Statuses = append(out.Statuses, statusRef{ID: s.ID, Name: string(s.Name), Growing: s.Name.IsGrowing()})
	}
	for _, a := range types {
		r := activityTypeRef{ID: a.ID, Name: a.Name, Watering: a.IsWatering, Feeding: a.IsFeeding}
		for _, m := range a.Metrics {
			label := m.Name
			if m.Required {
				label += " (required)"
			}
			r.Records = append(r.Records, label)
		}
		out.ActivityTypes = append(out.ActivityTypes, r)
	}
	for _, m := range metrics {
		out.Metrics = append(out.Metrics, metricRef{ID: m.ID, Name: m.Name, Unit: m.Unit})
	}
	return result{
		summary: fmt.Sprintf("%s, %s, %s.", count(len(out.Statuses), "status"),
			count(len(out.ActivityTypes), "activity type"), count(len(out.Metrics), "metric")),
		data: out,
	}, nil
}

// ----------------------------------------------------------------- helpers

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "s") {
		return fmt.Sprintf("%d %ses", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func round(f float64) float64 { return math.Round(f*100) / 100 }

func ptr(f float64) *float64 {
	r := round(f)
	return &r
}

func roundPtr(f *float64) *float64 {
	if f == nil {
		return nil
	}
	return ptr(*f)
}
