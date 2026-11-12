package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"isley/chrono"
)

// MeasurementValue is a value recorded with an activity.
type MeasurementValue struct {
	MetricID int
	Value    float64
}

// ActivityInput is an activity to log.
type ActivityInput struct {
	TypeID int
	Note   string
	// At is when it happened; zero means now.
	At           time.Time
	Measurements []MeasurementValue
}

// ActivityUpdate changes the fields that are Set. Setting Measurements
// replaces all of the activity's measurements.
type ActivityUpdate struct {
	TypeID       Patch[int]
	Note         Patch[string]
	At           Patch[time.Time]
	Measurements Patch[[]MeasurementValue]
}

// ActivityEntry is an activity with the plant it was logged against.
type ActivityEntry struct {
	PlantActivity
	Plant  Ref
	Zone   *Ref
	Strain *Ref
}

// ActivityOrder sorts the activity log.
type ActivityOrder string

const (
	NewestFirst ActivityOrder = ""
	OldestFirst ActivityOrder = "oldest"
	ByPlant     ActivityOrder = "plant"
	ByType      ActivityOrder = "type"
)

// ActivityFilter selects activities for the log. Zero fields don't filter.
type ActivityFilter struct {
	PlantID int
	ZoneID  int
	TypeIDs []int
	// From and To are calendar days in the stored (instance-zone) time,
	// both inclusive.
	From, To *chrono.Date
	// Query matches notes, ignoring case.
	Query  string
	Order  ActivityOrder
	Offset int
	// Limit 0 returns every match.
	Limit int
}

// Error codes for activities.
const (
	CodeActivityNotFound = "activity_not_found"
	maxNoteLength        = 5000
)

// ActivityService logs and reads activities.
type ActivityService struct {
	deps Deps
}

func (s *ActivityService) plants() *PlantService { return &PlantService{deps: s.deps} }

// validate checks in for logging against an activity type. required says
// whether the type's required metrics must be present (new activities).
// It drops measurement rows with no metric (the web forms send them for
// empty rows).
func (s *ActivityService) validate(ctx context.Context, v *Validation, in *ActivityInput, required bool) error {
	if len(in.Note) > maxNoteLength {
		v.Add("note", fmt.Sprintf("must be at most %d characters", maxNoteLength))
	}
	kept := in.Measurements[:0:0]
	var ids []int
	for _, m := range in.Measurements {
		if m.MetricID <= 0 {
			continue
		}
		if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
			v.Add("measurements", "values must be numbers")
		}
		kept = append(kept, m)
		ids = append(ids, m.MetricID)
	}
	in.Measurements = kept
	if len(ids) > 0 {
		known, err := existingIDs(ctx, s.deps.DB, "metric", ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !known[id] {
				v.Add("measurements", fmt.Sprintf("metric %d does not exist", id))
				break
			}
		}
	}
	if in.TypeID <= 0 {
		v.Add("activity_type_id", "is required")
		return nil
	}
	t, err := (&LookupService{deps: s.deps}).ActivityType(ctx, in.TypeID)
	if IsKind(err, KindNotFound) {
		v.Add("activity_type_id", fmt.Sprintf("%d does not exist", in.TypeID))
		return nil
	}
	if err != nil {
		return err
	}
	if required {
		have := map[int]bool{}
		for _, m := range in.Measurements {
			have[m.MetricID] = true
		}
		for _, m := range t.Metrics {
			if m.Required && !have[m.MetricID] {
				v.Add("measurements", fmt.Sprintf("%s needs a %s measurement", t.Name, m.Name))
				break
			}
		}
	}
	return nil
}

// Create logs an activity against a plant.
func (s *ActivityService) Create(ctx context.Context, plantID int, in ActivityInput) (ActivityEntry, error) {
	out, err := s.CreateBulk(ctx, []int{plantID}, in)
	if err != nil {
		if e, ok := AsError(err); ok && e.Fields["plant_ids"] != "" {
			return ActivityEntry{}, NotFound(CodePlantNotFound, fmt.Sprintf("plant %d does not exist", plantID))
		}
		return ActivityEntry{}, err
	}
	return out[0], nil
}

// CreateBulk logs the same activity against several plants, all or none.
func (s *ActivityService) CreateBulk(ctx context.Context, plantIDs []int, in ActivityInput) ([]ActivityEntry, error) {
	var v Validation
	seen := map[int]bool{}
	var plants []int
	for _, id := range plantIDs {
		if !seen[id] {
			seen[id] = true
			plants = append(plants, id)
		}
	}
	if len(plants) == 0 {
		v.Add("plant_ids", "must name at least one plant")
	} else {
		known, err := existingIDs(ctx, s.deps.DB, "plant", plants)
		if err != nil {
			return nil, err
		}
		for _, id := range plants {
			if !known[id] {
				v.Add("plant_ids", fmt.Sprintf("plant %d does not exist", id))
				break
			}
		}
	}
	if err := s.validate(ctx, &v, &in, true); err != nil {
		return nil, err
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	at := in.At
	if at.IsZero() {
		at = s.deps.Zones.Now(s.deps.Clock)
	}
	stored := s.deps.Zones.ToWall(at)

	ids := make([]int, 0, len(plants))
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, plantID := range plants {
			var id int
			if err := tx.QueryRowContext(ctx,
				"INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4) RETURNING id",
				plantID, in.TypeID, in.Note, stored).Scan(&id); err != nil {
				return fmt.Errorf("insert activity: %w", err)
			}
			if err := saveActivityMeasurements(ctx, tx, plantID, id, stored, in.Measurements); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]ActivityEntry, len(ids))
	for i, id := range ids {
		if out[i], err = s.Get(ctx, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func saveActivityMeasurements(ctx context.Context, tx *sql.Tx, plantID, activityID int, stored string, ms []MeasurementValue) error {
	for _, m := range ms {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id)
			VALUES ($1, $2, $3, $4, $5)`, plantID, m.MetricID, m.Value, stored, activityID); err != nil {
			return fmt.Errorf("insert activity measurement: %w", err)
		}
	}
	return nil
}

// Get returns one activity.
func (s *ActivityService) Get(ctx context.Context, id int) (ActivityEntry, error) {
	entries, _, err := s.query(ctx, "pa.id = $1", []any{id}, "", 0, 0)
	if err != nil {
		return ActivityEntry{}, err
	}
	if len(entries) == 0 {
		return ActivityEntry{}, NotFound(CodeActivityNotFound, fmt.Sprintf("activity %d does not exist", id))
	}
	return entries[0], nil
}

// Update changes an activity. Its measurements move with its date.
func (s *ActivityService) Update(ctx context.Context, id int, upd ActivityUpdate) (ActivityEntry, error) {
	var v Validation
	if upd.At.Set && upd.At.Value == nil {
		v.Add("at", "is required")
	}
	if upd.TypeID.Set && upd.TypeID.Value == nil {
		v.Add("activity_type_id", "is required")
	}
	if err := v.Err(); err != nil {
		return ActivityEntry{}, err
	}
	cur, err := s.Get(ctx, id)
	if err != nil {
		return ActivityEntry{}, err
	}
	in := ActivityInput{TypeID: cur.Type.ID, Note: cur.Note, At: cur.At}
	if upd.TypeID.Set {
		in.TypeID = *upd.TypeID.Value
	}
	if upd.Note.Set && upd.Note.Value != nil {
		in.Note = *upd.Note.Value
	} else if upd.Note.Set {
		in.Note = ""
	}
	if upd.At.Set {
		in.At = *upd.At.Value
	}
	if upd.Measurements.Set && upd.Measurements.Value != nil {
		in.Measurements = *upd.Measurements.Value
	}
	if err := s.validate(ctx, &v, &in, false); err != nil {
		return ActivityEntry{}, err
	}
	if err := v.Err(); err != nil {
		return ActivityEntry{}, err
	}
	stored := s.deps.Zones.ToWall(in.At)
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE plant_activity SET activity_id = $1, note = $2, date = $3 WHERE id = $4",
			in.TypeID, in.Note, stored, id); err != nil {
			return fmt.Errorf("update activity %d: %w", id, err)
		}
		if upd.Measurements.Set {
			if _, err := tx.ExecContext(ctx, "DELETE FROM plant_measurements WHERE plant_activity_id = $1", id); err != nil {
				return fmt.Errorf("clear activity %d measurements: %w", id, err)
			}
			return saveActivityMeasurements(ctx, tx, cur.Plant.ID, id, stored, in.Measurements)
		}
		_, err := tx.ExecContext(ctx, "UPDATE plant_measurements SET date = $1 WHERE plant_activity_id = $2", stored, id)
		return err
	})
	if err != nil {
		return ActivityEntry{}, err
	}
	return s.Get(ctx, id)
}

// Delete removes an activity and the measurements recorded with it.
func (s *ActivityService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM plant_measurements WHERE plant_activity_id = $1",
			"DELETE FROM plant_activity WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete activity %d: %w", id, err)
			}
		}
		return nil
	})
}

// Log returns the activities f selects and how many match in all.
func (s *ActivityService) Log(ctx context.Context, f ActivityFilter) ([]ActivityEntry, int, error) {
	var where []string
	var args []any
	add := func(cond string, vals ...any) {
		for _, v := range vals {
			args = append(args, v)
			cond = strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), 1)
		}
		where = append(where, cond)
	}
	if f.PlantID > 0 {
		add("pa.plant_id = ?", f.PlantID)
	}
	if f.ZoneID > 0 {
		add("p.zone_id = ?", f.ZoneID)
	}
	if len(f.TypeIDs) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(f.TypeIDs)), ", ")
		add("pa.activity_id IN ("+marks+")", toAny(f.TypeIDs)...)
	}
	// Days are instance-zone days: midnight to midnight, as stored instants.
	z := s.deps.Zones
	midnight := func(d chrono.Date) time.Time {
		return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, z.Instance())
	}
	if f.From != nil {
		add("pa.date >= ?", z.ToWall(midnight(*f.From)))
	}
	if f.To != nil {
		add("pa.date < ?", z.ToWall(midnight(f.To.AddDays(1))))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("LOWER(pa.note) LIKE ?", "%"+strings.ToLower(q)+"%")
	}
	cond := "1 = 1"
	if len(where) > 0 {
		cond = strings.Join(where, " AND ")
	}
	return s.query(ctx, cond, args, f.Order, f.Offset, f.Limit)
}

// ForPlant returns a plant's activities, newest first.
func (s *ActivityService) ForPlant(ctx context.Context, plantID, offset, limit int) ([]ActivityEntry, int, error) {
	if _, err := s.plants().Get(ctx, plantID); err != nil {
		return nil, 0, err
	}
	return s.Log(ctx, ActivityFilter{PlantID: plantID, Offset: offset, Limit: limit})
}

const activityFrom = `
	FROM plant_activity pa
	LEFT JOIN activity a ON a.id = pa.activity_id
	LEFT JOIN plant p ON p.id = pa.plant_id
	LEFT JOIN zones z ON z.id = p.zone_id
	LEFT JOIN strain st ON st.id = p.strain_id`

// query runs the activity log query: where is a condition over the
// activityFrom tables.
func (s *ActivityService) query(ctx context.Context, where string, args []any, order ActivityOrder, offset, limit int) ([]ActivityEntry, int, error) {
	var total int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+activityFrom+" WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activities: %w", err)
	}
	orderBy := "pa.date DESC, pa.id DESC"
	switch order {
	case OldestFirst:
		orderBy = "pa.date ASC, pa.id ASC"
	case ByPlant:
		orderBy = "COALESCE(p.name, '') ASC, pa.date DESC, pa.id DESC"
	case ByType:
		orderBy = "COALESCE(a.name, '') ASC, pa.date DESC, pa.id DESC"
	}
	q := `SELECT pa.id, pa.activity_id, COALESCE(a.name, ''), pa.note, pa.date,
	             COALESCE(a.is_watering, FALSE), COALESCE(a.is_feeding, FALSE),
	             pa.plant_id, COALESCE(p.name, ''), z.id, z.name, st.id, st.name` +
		activityFrom + " WHERE " + where + " ORDER BY " + orderBy
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	rows, err := s.deps.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()
	out := []ActivityEntry{}
	index := map[int]int{}
	for rows.Next() {
		var (
			e                    ActivityEntry
			at                   chrono.Stored
			zoneID, strainID     sql.NullInt64
			zoneName, strainName sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.Type.ID, &e.Type.Name, &e.Note, &at, &e.IsWatering, &e.IsFeeding,
			&e.Plant.ID, &e.Plant.Name, &zoneID, &zoneName, &strainID, &strainName); err != nil {
			return nil, 0, fmt.Errorf("scan activity: %w", err)
		}
		e.At = s.deps.Zones.FromWall(at.Time)
		if zoneID.Valid {
			e.Zone = &Ref{ID: int(zoneID.Int64), Name: zoneName.String}
		}
		if strainID.Valid {
			e.Strain = &Ref{ID: int(strainID.Int64), Name: strainName.String}
		}
		e.Measurements = []Measurement{}
		index[e.ID] = len(out)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list activities: %w", err)
	}
	if len(out) > 0 {
		ids := make([]int, len(out))
		for i, e := range out {
			ids[i] = e.ID
		}
		ms, err := s.plants().measurementsWhere(ctx,
			"pm.plant_activity_id IN ("+Placeholders(1, len(ids))+")", toAny(ids), 0)
		if err != nil {
			return nil, 0, err
		}
		for _, m := range ms {
			i := index[*m.ActivityID]
			out[i].Measurements = append(out[i].Measurements, m)
		}
	}
	return out, total, nil
}
