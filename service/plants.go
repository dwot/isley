package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"isley/chrono"
)

// Ref names a related record: enough to display it and to fetch it.
type Ref struct {
	ID   int
	Name string
}

// PlantStrain is the strain a plant was grown from.
type PlantStrain struct {
	Ref
	Breeder    Ref
	URL        string
	Autoflower bool
	// CycleDays is the strain's expected flowering time (photoperiod) or
	// seed-to-harvest time (autoflower), in days. 0 means unknown.
	CycleDays int
}

// StatusEntry is one row of a plant's status history: the plant entered
// Status at At.
type StatusEntry struct {
	ID       int
	StatusID int
	Status   StatusName
	At       time.Time
}

// FlowerPeriod is the plant's latest spell in flower.
type FlowerPeriod struct {
	Start time.Time
	// End is when the plant left flower (its next status), or nil if it
	// is still flowering.
	End *time.Time
	// Days is how long it flowered. While flowering it is the day of
	// flower, with the first day as Day 1; once ended it is the number of
	// days between Start and End.
	Days int
}

// Plant is a plant with the facts derived from its history: age, status,
// last watering and feeding, flowering and harvest.
type Plant struct {
	ID          int
	Name        string
	Description string
	Clone       bool
	Parent      *Ref
	Strain      *PlantStrain
	Zone        *Ref

	// StartDay is the day the plant was started, which plant age counts
	// from; StartedAt is that day's midnight in the instance zone.
	StartedAt time.Time
	StartDay  chrono.Date
	// Day and Week are the plant's age today: the start day is Day 1 and
	// week 1 is days 1-7.
	Day  int
	Week int

	// Status is the latest entry in the status history; nil if the plant
	// has none.
	Status *StatusEntry

	HarvestWeightG float64

	LastWatered *time.Time
	LastFed     *time.Time
	// DaysSinceWatered and DaysSinceFed count calendar days in the
	// instance zone (0 = today). Nil when it never happened.
	DaysSinceWatered *int
	DaysSinceFed     *int

	Flower *FlowerPeriod
	// HarvestedAt is when the plant first entered a status that ends
	// growth (Drying, Curing, Success or Dead).
	HarvestedAt *time.Time
	// EstimatedHarvest is the start day (autoflower) or the first day of
	// flower (photoperiod) plus the strain's cycle time. Nil when unknown.
	EstimatedHarvest *chrono.Date
}

// PlantDetail is a plant with its history and linked records.
type PlantDetail struct {
	Plant
	// StatusHistory is oldest first.
	StatusHistory []StatusEntry
	// Stages is how long the plant spent in each status it reached, in the
	// order first reached. See stageDays.
	Stages []StageDays
	// GrowDays is the plant's age (Day N) on the day growth ended, or today
	// while it is still growing. 0 without a start date.
	GrowDays int
	Sensors  []PlantSensor
	// Activities and Measurements are newest first, up to the limit asked
	// for.
	Activities   []PlantActivity
	Measurements []Measurement
	// Images are newest first.
	Images []PlantImage
}

// StageDays is the time a plant spent in one status.
type StageDays struct {
	StatusID int
	Status   StatusName
	Days     int
}

// PlantSensor is a sensor that reports on a plant: linked to it directly,
// or inherited from its zone.
type PlantSensor struct {
	ID        int
	Name      string
	Source    string
	Type      string
	Unit      string
	Inherited bool
	Latest    *Reading
	// VPD is set for the zone's derived VPD sensor when the plant's status
	// has a target range.
	VPD *VPDCheck
}

// VPDCheck compares a VPD reading with the target for the plant's status.
type VPDCheck struct {
	LowKPa  float64
	HighKPa float64
	// State is "low", "ideal" or "high"; "" without a reading.
	State string
}

// PlantActivity is something done to a plant (watering, feeding, a note).
type PlantActivity struct {
	ID           int
	Type         Ref
	Note         string
	At           time.Time
	IsWatering   bool
	IsFeeding    bool
	Measurements []Measurement
}

// Measurement is a value recorded for a plant.
type Measurement struct {
	ID     int
	Metric Ref
	Unit   string
	Value  float64
	At     time.Time
	// ActivityID is the activity it was recorded with, if any.
	ActivityID *int
}

// PlantImage is a photo of a plant.
type PlantImage struct {
	ID      int
	PlantID int
	// Path is where the file is stored, relative to the working
	// directory (normally uploads/plants/<file>), with forward slashes.
	Path        string
	Description string
	Order       int
	// TakenOn is the day the photo was taken.
	TakenOn chrono.Date
}

// PlantPhase filters plants by what their current status means.
type PlantPhase string

const (
	PhaseGrowing   PlantPhase = "growing"
	PhaseHarvested PlantPhase = "harvested"
	PhaseDead      PlantPhase = "dead"
)

// PlantFilter selects plants. Zero fields don't filter.
type PlantFilter struct {
	Phase    PlantPhase
	StatusID int
	ZoneID   int
	StrainID int
	// AfterID and Limit page through the results in id order. Limit 0
	// returns every match.
	AfterID int
	Limit   int
}

// Error codes for plants.
const CodePlantNotFound = "plant_not_found"

// PlantService reads plants.
type PlantService struct {
	deps Deps
}

// plantSelect yields one row per plant; filters go on the q.* columns.
const plantSelect = `
	SELECT q.* FROM (
		SELECT p.id AS id, p.name AS name, p.description AS description, p.clone AS clone,
		       p.start_dt AS start_dt, COALESCE(p.harvest_weight, 0) AS harvest_weight,
		       p.parent_plant_id AS parent_id, pp.name AS parent_name,
		       s.id AS strain_id, s.name AS strain_name, s.url AS strain_url,
		       s.autoflower AS autoflower, s.cycle_time AS cycle_time,
		       b.id AS breeder_id, b.name AS breeder_name,
		       z.id AS zone_id, z.name AS zone_name,
		       (SELECT psl.status_id FROM plant_status_log psl
		        WHERE psl.plant_id = p.id ORDER BY psl.date DESC, psl.id DESC LIMIT 1) AS current_status_id
		FROM plant p
		LEFT JOIN plant pp ON pp.id = p.parent_plant_id
		LEFT JOIN strain s ON s.id = p.strain_id
		LEFT JOIN breeder b ON b.id = s.breeder_id
		LEFT JOIN zones z ON z.id = p.zone_id
	) q`

func (s *PlantService) scanPlant(row interface{ Scan(...any) error }) (Plant, error) {
	var (
		p                       Plant
		start                   chrono.Date
		parentID                sql.NullInt64
		parentName              sql.NullString
		strainID, breederID     sql.NullInt64
		strainName, breederName sql.NullString
		strainURL               sql.NullString
		autoflower              sql.NullBool
		cycle                   sql.NullInt64
		zoneID                  sql.NullInt64
		zoneName                sql.NullString
		currentStatus           sql.NullInt64
	)
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Clone, &start, &p.HarvestWeightG,
		&parentID, &parentName,
		&strainID, &strainName, &strainURL, &autoflower, &cycle,
		&breederID, &breederName,
		&zoneID, &zoneName, &currentStatus)
	if err != nil {
		return Plant{}, err
	}
	// The start is a calendar day. Its stored digits are the day whether
	// the row holds a bare date or (from older versions) a wall time, so
	// it is never shifted by the UTC migration.
	if !start.IsZero() {
		p.StartDay = start
		p.StartedAt = time.Date(start.Year, start.Month, start.Day, 0, 0, 0, 0, s.deps.Zones.Instance())
	}
	if parentID.Valid && parentName.Valid {
		p.Parent = &Ref{ID: int(parentID.Int64), Name: parentName.String}
	}
	if strainID.Valid {
		p.Strain = &PlantStrain{
			Ref:        Ref{ID: int(strainID.Int64), Name: strainName.String},
			Breeder:    Ref{ID: int(breederID.Int64), Name: breederName.String},
			URL:        strainURL.String,
			Autoflower: autoflower.Bool,
			CycleDays:  int(cycle.Int64),
		}
	}
	if zoneID.Valid {
		p.Zone = &Ref{ID: int(zoneID.Int64), Name: zoneName.String}
	}
	return p, nil
}

// List returns the plants f selects, in id order, and whether more follow.
func (s *PlantService) List(ctx context.Context, f PlantFilter) ([]Plant, bool, error) {
	where := []string{"q.id > $1"}
	args := []any{f.AfterID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ZoneID > 0 {
		add("q.zone_id = $%d", f.ZoneID)
	}
	if f.StrainID > 0 {
		add("q.strain_id = $%d", f.StrainID)
	}
	if f.StatusID > 0 {
		add("q.current_status_id = $%d", f.StatusID)
	}
	if f.Phase != "" {
		ids, err := s.statusIDsIn(ctx, f.Phase)
		if err != nil {
			return nil, false, err
		}
		if len(ids) == 0 {
			return []Plant{}, false, nil
		}
		in := make([]string, len(ids))
		for i, id := range ids {
			args = append(args, id)
			in[i] = fmt.Sprintf("$%d", len(args))
		}
		where = append(where, "q.current_status_id IN ("+strings.Join(in, ", ")+")")
	}
	query := plantSelect + " WHERE " + strings.Join(where, " AND ") + " ORDER BY q.id"
	if f.Limit > 0 {
		args = append(args, f.Limit+1)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := s.deps.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list plants: %w", err)
	}
	defer rows.Close()
	plants := []Plant{}
	for rows.Next() {
		p, err := s.scanPlant(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan plant: %w", err)
		}
		plants = append(plants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list plants: %w", err)
	}
	more := f.Limit > 0 && len(plants) > f.Limit
	if more {
		plants = plants[:f.Limit]
	}
	if err := s.derive(ctx, plants); err != nil {
		return nil, false, err
	}
	return plants, more, nil
}

// Get returns one plant.
func (s *PlantService) Get(ctx context.Context, id int) (Plant, error) {
	p, err := s.scanPlant(s.deps.DB.QueryRowContext(ctx, plantSelect+" WHERE q.id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Plant{}, NotFound(CodePlantNotFound, fmt.Sprintf("plant %d does not exist", id))
	}
	if err != nil {
		return Plant{}, fmt.Errorf("get plant %d: %w", id, err)
	}
	plants := []Plant{p}
	if err := s.derive(ctx, plants); err != nil {
		return Plant{}, err
	}
	return plants[0], nil
}

// Detail returns a plant with its full status history, sensors, images,
// and its latest activities and measurements (up to limit each; 0 means
// all).
func (s *PlantService) Detail(ctx context.Context, id, limit int) (PlantDetail, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return PlantDetail{}, err
	}
	d := PlantDetail{Plant: p}
	histories, err := s.histories(ctx, []int{id})
	if err != nil {
		return PlantDetail{}, err
	}
	d.StatusHistory = histories[id]
	if d.StatusHistory == nil {
		d.StatusHistory = []StatusEntry{}
	}
	today := s.deps.Zones.Today(s.deps.Clock)
	d.Stages = stageDays(d.StatusHistory, today, s.deps.Zones)
	if !p.StartDay.IsZero() {
		end := today
		if p.HarvestedAt != nil {
			end = s.deps.Zones.DateOf(*p.HarvestedAt)
		}
		d.GrowDays, _ = chrono.PlantAge(p.StartDay, end)
	}
	if d.Sensors, err = s.sensors(ctx, p); err != nil {
		return PlantDetail{}, err
	}
	if d.Activities, err = s.activities(ctx, id, limit); err != nil {
		return PlantDetail{}, err
	}
	if d.Measurements, err = s.measurements(ctx, id, limit); err != nil {
		return PlantDetail{}, err
	}
	if d.Images, err = s.images(ctx, id); err != nil {
		return PlantDetail{}, err
	}
	return d, nil
}

// Adjacent returns the plants before and after id in the plant page's
// order (start, then name) among plants that are growing, or among those
// that are not. 0 means none.
func (s *PlantService) Adjacent(ctx context.Context, id int) (prev, next int, err error) {
	const query = `
		WITH current AS (
			SELECT p.id, p.start_dt, p.name,
			       (SELECT ps.active FROM plant_status_log psl
			        JOIN plant_status ps ON ps.id = psl.status_id
			        WHERE psl.plant_id = p.id ORDER BY psl.date DESC, psl.id DESC LIMIT 1) AS active
			FROM plant p
		), ranked AS (
			SELECT id,
			       LAG(id)  OVER (PARTITION BY active ORDER BY start_dt, name, id) AS prev_id,
			       LEAD(id) OVER (PARTITION BY active ORDER BY start_dt, name, id) AS next_id
			FROM current WHERE active IS NOT NULL
		)
		SELECT COALESCE(prev_id, 0), COALESCE(next_id, 0) FROM ranked WHERE id = $1`
	err = s.deps.DB.QueryRowContext(ctx, query, id).Scan(&prev, &next)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("adjacent plants of %d: %w", id, err)
	}
	return prev, next, nil
}

func (s *PlantService) statusIDsIn(ctx context.Context, phase PlantPhase) ([]int, error) {
	all, err := listStatuses(ctx, s.deps.DB)
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, st := range all {
		var in bool
		switch phase {
		case PhaseGrowing:
			in = st.Name.IsGrowing()
		case PhaseHarvested:
			in = st.Name.IsHarvested()
		case PhaseDead:
			in = st.Name.IsDead()
		default:
			return nil, Invalid(map[string]string{"phase": "must be growing, harvested or dead"})
		}
		if in {
			ids = append(ids, st.ID)
		}
	}
	return ids, nil
}

// derive fills in the facts that come from each plant's status history and
// activities.
func (s *PlantService) derive(ctx context.Context, plants []Plant) error {
	if len(plants) == 0 {
		return nil
	}
	ids := make([]int, len(plants))
	for i, p := range plants {
		ids[i] = p.ID
	}
	histories, err := s.histories(ctx, ids)
	if err != nil {
		return err
	}
	lastWater, lastFeed, err := s.lastCare(ctx, ids)
	if err != nil {
		return err
	}
	today := s.deps.Zones.Today(s.deps.Clock)
	for i := range plants {
		p := &plants[i]
		derivePlant(p, histories[p.ID], lastWater[p.ID], lastFeed[p.ID], today, s.deps.Zones)
	}
	return nil
}

// derivePlant computes a plant's derived facts. history is oldest first.
func derivePlant(p *Plant, history []StatusEntry, lastWater, lastFeed *time.Time, today chrono.Date, zones *chrono.Zones) {
	if !p.StartDay.IsZero() {
		p.Day, p.Week = chrono.PlantAge(p.StartDay, today)
	}
	if n := len(history); n > 0 {
		cur := history[n-1]
		p.Status = &cur
	}
	daysSince := func(t *time.Time) *int {
		if t == nil {
			return nil
		}
		d := today.DaysSince(zones.DateOf(*t))
		return &d
	}
	p.LastWatered, p.DaysSinceWatered = lastWater, daysSince(lastWater)
	p.LastFed, p.DaysSinceFed = lastFeed, daysSince(lastFeed)

	var firstFlower *time.Time
	for i, e := range history {
		if e.Status.IsFlowering() {
			if firstFlower == nil {
				at := e.At
				firstFlower = &at
			}
			fp := &FlowerPeriod{Start: e.At}
			if i+1 < len(history) {
				end := history[i+1].At
				fp.End = &end
				fp.Days = zones.DateOf(end).DaysSince(zones.DateOf(e.At))
			} else {
				fp.Days, _ = chrono.PlantAge(zones.DateOf(e.At), today)
			}
			p.Flower = fp
		}
		if p.HarvestedAt == nil && e.Status.EndsGrowth() {
			at := e.At
			p.HarvestedAt = &at
		}
	}

	if p.Strain != nil && p.Strain.CycleDays > 0 {
		var from chrono.Date
		switch {
		case p.Strain.Autoflower:
			from = p.StartDay
		case firstFlower != nil:
			from = zones.DateOf(*firstFlower)
		}
		if !from.IsZero() {
			est := from.AddDays(p.Strain.CycleDays)
			p.EstimatedHarvest = &est
		}
	}
}

// stageDays totals the days spent in each status. A stage that has ended
// counts the days from its start to the next stage's start. The current
// stage counts like plant age, with its first day as Day 1, except a
// final Success or Dead, which gets 0 (it is a date, not a stretch of
// time). A status reached more than once adds up.
func stageDays(history []StatusEntry, today chrono.Date, zones *chrono.Zones) []StageDays {
	out := []StageDays{}
	index := map[int]int{}
	for i, e := range history {
		start := zones.DateOf(e.At)
		var days int
		switch {
		case i+1 < len(history):
			days = zones.DateOf(history[i+1].At).DaysSince(start)
		case e.Status == StatusSuccess || e.Status.IsDead():
			days = 0
		default:
			days, _ = chrono.PlantAge(start, today)
		}
		if days < 0 {
			days = 0
		}
		j, seen := index[e.StatusID]
		if !seen {
			j = len(out)
			index[e.StatusID] = j
			out = append(out, StageDays{StatusID: e.StatusID, Status: e.Status})
		}
		out[j].Days += days
	}
	return out
}

// histories returns each plant's status history, oldest first.
func (s *PlantService) histories(ctx context.Context, ids []int) (map[int][]StatusEntry, error) {
	args, in := idArgs(ids)
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT psl.id, psl.plant_id, psl.status_id, ps.status, psl.date
		FROM plant_status_log psl
		JOIN plant_status ps ON ps.id = psl.status_id
		WHERE psl.plant_id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("status histories: %w", err)
	}
	defer rows.Close()
	out := map[int][]StatusEntry{}
	for rows.Next() {
		var (
			e       StatusEntry
			plantID int
			name    string
			at      chrono.Stored
		)
		if err := rows.Scan(&e.ID, &plantID, &e.StatusID, &name, &at); err != nil {
			return nil, fmt.Errorf("scan status entry: %w", err)
		}
		e.Status = StatusName(name)
		e.At = s.deps.Zones.FromWall(at.Time)
		out[plantID] = append(out[plantID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("status histories: %w", err)
	}
	// Sort in Go: stored dates mix text layouts on SQLite.
	for _, h := range out {
		sort.SliceStable(h, func(i, j int) bool {
			if !h[i].At.Equal(h[j].At) {
				return h[i].At.Before(h[j].At)
			}
			return h[i].ID < h[j].ID
		})
	}
	return out, nil
}

// lastCare returns each plant's latest watering and feeding.
func (s *PlantService) lastCare(ctx context.Context, ids []int) (water, feed map[int]*time.Time, err error) {
	args, in := idArgs(ids)
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT pa.plant_id, a.is_watering, a.is_feeding, pa.date
		FROM plant_activity pa
		JOIN activity a ON a.id = pa.activity_id
		WHERE pa.plant_id IN (`+in+`) AND (a.is_watering = TRUE OR a.is_feeding = TRUE)`, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("last watering and feeding: %w", err)
	}
	defer rows.Close()
	water, feed = map[int]*time.Time{}, map[int]*time.Time{}
	later := func(m map[int]*time.Time, id int, t time.Time) {
		if cur := m[id]; cur == nil || t.After(*cur) {
			m[id] = &t
		}
	}
	for rows.Next() {
		var (
			plantID           int
			watering, feeding bool
			at                chrono.Stored
		)
		if err := rows.Scan(&plantID, &watering, &feeding, &at); err != nil {
			return nil, nil, fmt.Errorf("scan activity date: %w", err)
		}
		t := s.deps.Zones.FromWall(at.Time)
		if watering {
			later(water, plantID, t)
		}
		if feeding {
			later(feed, plantID, t)
		}
	}
	return water, feed, rows.Err()
}

// LinkedSensors returns the sensors linked directly to each plant, in
// the order they were linked.
func (s *PlantService) LinkedSensors(ctx context.Context, plantIDs []int) (map[int][]int, error) {
	out := map[int][]int{}
	if len(plantIDs) == 0 {
		return out, nil
	}
	args, in := idArgs(plantIDs)
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT plant_id, sensor_id FROM plant_sensor
		WHERE plant_id IN (`+in+`) ORDER BY plant_id, position, sensor_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("plant sensor links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var plantID, sensorID int
		if err := rows.Scan(&plantID, &sensorID); err != nil {
			return nil, fmt.Errorf("scan plant sensor link: %w", err)
		}
		out[plantID] = append(out[plantID], sensorID)
	}
	return out, rows.Err()
}

// sensors returns the plant's direct sensors (in link order), then the
// zone's sensors it inherits: those shown on plants, except soil probes,
// which belong to the plant they're in.
func (s *PlantService) sensors(ctx context.Context, p Plant) ([]PlantSensor, error) {
	links, err := s.LinkedSensors(ctx, []int{p.ID})
	if err != nil {
		return nil, err
	}
	direct := links[p.ID]
	order := append([]int(nil), direct...)
	inherited := map[int]bool{}
	if p.Zone != nil {
		rows, err := s.deps.DB.QueryContext(ctx, `
			SELECT id FROM sensors
			WHERE zone_id = $1 AND visibility IN ('zone_plant', 'plant') AND type NOT LIKE 'Soil.%'
			ORDER BY id`, p.Zone.ID)
		if err != nil {
			return nil, fmt.Errorf("zone sensors: %w", err)
		}
		isDirect := map[int]bool{}
		for _, id := range direct {
			isDirect[id] = true
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan zone sensor: %w", err)
			}
			if !isDirect[id] {
				inherited[id] = true
				order = append(order, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("zone sensors: %w", err)
		}
	}
	if len(order) == 0 {
		return []PlantSensor{}, nil
	}

	args, in := idArgs(order)
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT s.id, s.name, s.source, s.type, s.unit, sd.value, sd.create_dt
		FROM sensors s
		LEFT JOIN sensor_data sd ON sd.id = (
			SELECT id FROM sensor_data WHERE sensor_id = s.id ORDER BY create_dt DESC LIMIT 1
		)
		WHERE s.id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("plant sensors: %w", err)
	}
	defer rows.Close()
	found := map[int]PlantSensor{}
	for rows.Next() {
		var (
			ps    PlantSensor
			value sql.NullFloat64
			at    chrono.Stored
		)
		if err := rows.Scan(&ps.ID, &ps.Name, &ps.Source, &ps.Type, &ps.Unit, &value, &at); err != nil {
			return nil, fmt.Errorf("scan plant sensor: %w", err)
		}
		if value.Valid && at.Valid {
			ps.Latest = &Reading{Value: value.Float64, At: s.deps.Zones.FromSystem(at.Time)}
		}
		found[ps.ID] = ps
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plant sensors: %w", err)
	}

	var low, high *float64
	if p.Status != nil {
		st, err := (&LookupService{deps: s.deps}).Status(ctx, p.Status.StatusID)
		if err != nil {
			return nil, err
		}
		low, high = st.VPDLowKPa, st.VPDHighKPa
	}

	out := []PlantSensor{}
	for _, id := range order {
		ps, ok := found[id]
		if !ok {
			continue // a link to a deleted sensor
		}
		ps.Inherited = inherited[id]
		if ps.Source == "derived" && ps.Type == "VPD" && low != nil && high != nil {
			ps.VPD = &VPDCheck{LowKPa: *low, HighKPa: *high}
			if ps.Latest != nil {
				switch v := ps.Latest.Value; {
				case v < *low:
					ps.VPD.State = "low"
				case v > *high:
					ps.VPD.State = "high"
				default:
					ps.VPD.State = "ideal"
				}
			}
		}
		out = append(out, ps)
	}
	return out, nil
}

// activities returns the plant's latest activities with the measurements
// recorded with them.
func (s *PlantService) activities(ctx context.Context, plantID, limit int) ([]PlantActivity, error) {
	query := `
		SELECT pa.id, pa.activity_id, COALESCE(a.name, ''), pa.note, pa.date,
		       COALESCE(a.is_watering, FALSE), COALESCE(a.is_feeding, FALSE)
		FROM plant_activity pa
		LEFT JOIN activity a ON a.id = pa.activity_id
		WHERE pa.plant_id = $1
		ORDER BY pa.date DESC, pa.id DESC`
	args := []any{plantID}
	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
	}
	rows, err := s.deps.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("plant activities: %w", err)
	}
	defer rows.Close()
	out := []PlantActivity{}
	index := map[int]int{}
	for rows.Next() {
		var (
			a  PlantActivity
			at chrono.Stored
		)
		if err := rows.Scan(&a.ID, &a.Type.ID, &a.Type.Name, &a.Note, &at, &a.IsWatering, &a.IsFeeding); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		a.At = s.deps.Zones.FromWall(at.Time)
		a.Measurements = []Measurement{}
		index[a.ID] = len(out)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plant activities: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]int, len(out))
	for i, a := range out {
		ids[i] = a.ID
	}
	ms, err := s.measurementsWhere(ctx, "pm.plant_activity_id IN ("+Placeholders(1, len(ids))+")", toAny(ids), 0)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		i := index[*m.ActivityID]
		out[i].Measurements = append(out[i].Measurements, m)
	}
	sortByTimeDesc(out, func(a PlantActivity) (time.Time, int) { return a.At, a.ID })
	return out, nil
}

// measurements returns the plant's latest measurements.
func (s *PlantService) measurements(ctx context.Context, plantID, limit int) ([]Measurement, error) {
	return s.measurementsWhere(ctx, "pm.plant_id = $1", []any{plantID}, limit)
}

func (s *PlantService) measurementsWhere(ctx context.Context, where string, args []any, limit int) ([]Measurement, error) {
	query := `
		SELECT pm.id, pm.metric_id, COALESCE(me.name, ''), COALESCE(me.unit, ''), pm.value, pm.date, pm.plant_activity_id
		FROM plant_measurements pm
		LEFT JOIN metric me ON me.id = pm.metric_id
		WHERE ` + where + `
		ORDER BY pm.date DESC, pm.id DESC`
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	rows, err := s.deps.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("plant measurements: %w", err)
	}
	defer rows.Close()
	out := []Measurement{}
	for rows.Next() {
		var (
			m          Measurement
			at         chrono.Stored
			activityID sql.NullInt64
		)
		if err := rows.Scan(&m.ID, &m.Metric.ID, &m.Metric.Name, &m.Unit, &m.Value, &at, &activityID); err != nil {
			return nil, fmt.Errorf("scan measurement: %w", err)
		}
		m.At = s.deps.Zones.FromWall(at.Time)
		m.ActivityID = nullInt(activityID)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plant measurements: %w", err)
	}
	sortByTimeDesc(out, func(m Measurement) (time.Time, int) { return m.At, m.ID })
	return out, nil
}

// images returns the plant's images, newest first.
func (s *PlantService) images(ctx context.Context, plantID int) ([]PlantImage, error) {
	return queryImages(ctx, s.deps, "plant_id = $1", plantID)
}

// sortByTimeDesc orders newest first, by id when times tie. SQL already
// orders, but stored text dates in mixed layouts don't sort exactly.
func sortByTimeDesc[T any](items []T, key func(T) (time.Time, int)) {
	sort.SliceStable(items, func(i, j int) bool {
		ti, ii := key(items[i])
		tj, ij := key(items[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return ii > ij
	})
}

func idArgs(ids []int) ([]any, string) {
	return toAny(ids), Placeholders(1, len(ids))
}

func toAny(ids []int) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}
