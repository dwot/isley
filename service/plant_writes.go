package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"isley/chrono"
)

// When is a time a user gave: a whole calendar day, or an instant. A plant
// started "on 1 November" is a day; the web form's date-time picker gives
// an instant. Exactly one of Day and At is set.
type When struct {
	Day chrono.Date
	At  *time.Time
}

// OnDay returns a When for a calendar day.
func OnDay(d chrono.Date) When { return When{Day: d} }

// AtTime returns a When for an instant.
func AtTime(t time.Time) When { return When{At: &t} }

// IsZero reports whether no time was given.
func (w When) IsZero() bool { return w.At == nil && w.Day.IsZero() }

// day is the calendar day: the day given, or the instant's day in the
// instance zone. A plant's start is stored as this day.
func (w When) day(z *chrono.Zones) chrono.Date {
	if w.At != nil {
		return z.DateOf(*w.At)
	}
	return w.Day
}

// instant is the start of the day for a day, or the instant itself.
func (w When) instant(z *chrono.Zones) time.Time {
	if w.At != nil {
		return *w.At
	}
	return time.Date(w.Day.Year, w.Day.Month, w.Day.Day, 0, 0, 0, 0, z.Instance())
}

// PlantInput is a plant to create.
type PlantInput struct {
	Name        string
	Description string
	StrainID    int
	// ZoneID is optional.
	ZoneID *int
	// StatusID is the status the plant starts in.
	StatusID int
	Start    When
	// StatusAt is when the first status began; it defaults to the start.
	StatusAt *time.Time
	Clone    bool
	ParentID *int
	// SensorIDs are sensors to link to the plant (a soil probe), in order.
	SensorIDs []int
	// DecrementSeedCount takes one seed off the strain's count (not below 0).
	DecrementSeedCount bool
}

// PlantUpdate changes the fields that are Set.
type PlantUpdate struct {
	Name           Patch[string]
	Description    Patch[string]
	StrainID       Patch[int]
	ZoneID         Patch[int]
	Start          Patch[When]
	HarvestWeightG Patch[float64]
	Clone          Patch[bool]
	ParentID       Patch[int]
}

// StatusEntryUpdate changes a status history entry.
type StatusEntryUpdate struct {
	StatusID Patch[int]
	At       Patch[time.Time]
}

// Error codes for plant writes.
const (
	CodeStatusEntryNotFound = "status_entry_not_found"
	CodeLastStatusEntry     = "last_status_entry"
)

const maxDescriptionLength = 2000

func (s *PlantService) exists(ctx context.Context, table string, id int) (bool, error) {
	known, err := existingIDs(ctx, s.deps.DB, table, []int{id})
	if err != nil {
		return false, err
	}
	return known[id], nil
}

// checkRef records a problem when id doesn't name a row of table.
func (s *PlantService) checkRef(ctx context.Context, v *Validation, field, table string, id int) error {
	if id <= 0 {
		v.Add(field, "is required")
		return nil
	}
	ok, err := s.exists(ctx, table, id)
	if err != nil {
		return err
	}
	if !ok {
		v.Add(field, fmt.Sprintf("%d does not exist", id))
	}
	return nil
}

// checkSensors validates and de-duplicates sensor ids, keeping order.
func (s *PlantService) checkSensors(ctx context.Context, v *Validation, ids []int) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	known, err := existingIDs(ctx, s.deps.DB, "sensors", out)
	if err != nil {
		return nil, err
	}
	for _, id := range out {
		if !known[id] {
			v.Add("sensor_ids", fmt.Sprintf("sensor %d does not exist", id))
			break
		}
	}
	return out, nil
}

func checkText(v *Validation, name, description string) {
	checkName(v, "name", name)
	if len(description) > maxDescriptionLength {
		v.Add("description", fmt.Sprintf("must be at most %d characters", maxDescriptionLength))
	}
}

// Create adds a plant with its first status entry.
func (s *PlantService) Create(ctx context.Context, in PlantInput) (Plant, error) {
	in.Name = strings.TrimSpace(in.Name)
	var v Validation
	checkText(&v, in.Name, in.Description)
	if err := s.checkRef(ctx, &v, "strain_id", "strain", in.StrainID); err != nil {
		return Plant{}, err
	}
	if in.ZoneID != nil {
		if err := s.checkRef(ctx, &v, "zone_id", "zones", *in.ZoneID); err != nil {
			return Plant{}, err
		}
	}
	if err := s.checkRef(ctx, &v, "status_id", "plant_status", in.StatusID); err != nil {
		return Plant{}, err
	}
	if in.ParentID != nil {
		if err := s.checkRef(ctx, &v, "parent_id", "plant", *in.ParentID); err != nil {
			return Plant{}, err
		}
	}
	if in.Start.IsZero() {
		v.Add("start", "is required")
	}
	sensors, err := s.checkSensors(ctx, &v, in.SensorIDs)
	if err != nil {
		return Plant{}, err
	}
	if err := v.Err(); err != nil {
		return Plant{}, err
	}

	statusAt := in.Start.instant(s.deps.Zones)
	if in.StatusAt != nil {
		statusAt = *in.StatusAt
	}
	var id int
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO plant (name, description, strain_id, zone_id, clone, parent_plant_id, start_dt)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			in.Name, in.Description, in.StrainID, in.ZoneID, boolInt(in.Clone), in.ParentID,
			in.Start.day(s.deps.Zones).String()).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert plant: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)",
			id, in.StatusID, s.deps.Zones.ToWall(statusAt)); err != nil {
			return fmt.Errorf("insert first status: %w", err)
		}
		if err := saveSensorLinks(ctx, tx, id, sensors); err != nil {
			return err
		}
		if in.DecrementSeedCount {
			_, err := tx.ExecContext(ctx, "UPDATE strain SET seed_count = "+
				s.deps.Dialect.Greatest("0", "seed_count - 1")+" WHERE id = $1", in.StrainID)
			if err != nil {
				return fmt.Errorf("decrement seed count: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Plant{}, err
	}
	if in.DecrementSeedCount {
		if err := refreshStrains(ctx, s.deps.DB, s.deps.Store); err != nil {
			return Plant{}, err
		}
	}
	return s.Get(ctx, id)
}

// Update changes the fields upd sets.
func (s *PlantService) Update(ctx context.Context, id int, upd PlantUpdate) (Plant, error) {
	var v Validation
	// Check the input first, so a bad request is reported as such whether
	// or not the plant exists.
	var name, description string
	if upd.Name.Set {
		if upd.Name.Value != nil {
			name = strings.TrimSpace(*upd.Name.Value)
		}
		checkName(&v, "name", name)
	}
	if upd.Description.Set && upd.Description.Value != nil {
		description = *upd.Description.Value
		if len(description) > maxDescriptionLength {
			v.Add("description", fmt.Sprintf("must be at most %d characters", maxDescriptionLength))
		}
	}
	if upd.StrainID.Set {
		if upd.StrainID.Value == nil {
			v.Add("strain_id", "is required")
		} else if err := s.checkRef(ctx, &v, "strain_id", "strain", *upd.StrainID.Value); err != nil {
			return Plant{}, err
		}
	}
	if upd.ZoneID.Set && upd.ZoneID.Value != nil {
		if err := s.checkRef(ctx, &v, "zone_id", "zones", *upd.ZoneID.Value); err != nil {
			return Plant{}, err
		}
	}
	if upd.ParentID.Set && upd.ParentID.Value != nil {
		if *upd.ParentID.Value == id {
			v.Add("parent_id", "can't be the plant itself")
		} else if err := s.checkRef(ctx, &v, "parent_id", "plant", *upd.ParentID.Value); err != nil {
			return Plant{}, err
		}
	}
	if upd.Start.Set && (upd.Start.Value == nil || upd.Start.Value.IsZero()) {
		v.Add("start", "is required")
	}
	if upd.HarvestWeightG.Set && upd.HarvestWeightG.Value != nil {
		if w := *upd.HarvestWeightG.Value; w < 0 || math.IsNaN(w) || math.IsInf(w, 0) {
			v.Add("harvest_weight_g", "must be a number, 0 or more")
		}
	}
	if err := v.Err(); err != nil {
		return Plant{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return Plant{}, err
	}

	var sets []string
	var args []any
	set := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if upd.Name.Set {
		set("name", name)
	}
	if upd.Description.Set {
		set("description", description)
	}
	if upd.StrainID.Set {
		set("strain_id", *upd.StrainID.Value)
	}
	if upd.ZoneID.Set {
		set("zone_id", upd.ZoneID.Value)
	}
	if upd.Start.Set {
		set("start_dt", upd.Start.Value.day(s.deps.Zones).String())
	}
	if upd.HarvestWeightG.Set {
		w := 0.0
		if upd.HarvestWeightG.Value != nil {
			w = *upd.HarvestWeightG.Value
		}
		set("harvest_weight", w)
	}
	if upd.Clone.Set {
		set("clone", boolInt(upd.Clone.Value != nil && *upd.Clone.Value))
	}
	if upd.ParentID.Set {
		set("parent_plant_id", upd.ParentID.Value)
	}
	if len(sets) > 0 {
		args = append(args, id)
		_, err := s.deps.DB.ExecContext(ctx,
			fmt.Sprintf("UPDATE plant SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)), args...)
		if err != nil {
			return Plant{}, fmt.Errorf("update plant %d: %w", id, err)
		}
	}
	return s.Get(ctx, id)
}

// SetSensors replaces the sensors linked directly to a plant.
func (s *PlantService) SetSensors(ctx context.Context, id int, sensorIDs []int) error {
	var v Validation
	sensors, err := s.checkSensors(ctx, &v, sensorIDs)
	if err != nil {
		return err
	}
	if err := v.Err(); err != nil {
		return err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM plant_sensor WHERE plant_id = $1", id); err != nil {
			return fmt.Errorf("clear plant %d sensors: %w", id, err)
		}
		return saveSensorLinks(ctx, tx, id, sensors)
	})
}

func saveSensorLinks(ctx context.Context, tx *sql.Tx, plantID int, sensorIDs []int) error {
	for i, sid := range sensorIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO plant_sensor (plant_id, sensor_id, position) VALUES ($1, $2, $3)",
			plantID, sid, i); err != nil {
			return fmt.Errorf("link sensor %d to plant %d: %w", sid, plantID, err)
		}
	}
	return nil
}

// Delete deletes a plant and everything recorded for it, image files
// included. Its clones stay, without a mother.
func (s *PlantService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	images, err := queryImages(ctx, s.deps, "plant_id = $1", id)
	if err != nil {
		return err
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM plant_images WHERE plant_id = $1",
			"DELETE FROM plant_measurements WHERE plant_id = $1",
			"DELETE FROM plant_activity WHERE plant_id = $1",
			"DELETE FROM plant_status_log WHERE plant_id = $1",
			"DELETE FROM smoke_report_score WHERE report_id IN (SELECT id FROM smoke_report WHERE plant_id = $1)",
			"DELETE FROM smoke_report WHERE plant_id = $1",
			"DELETE FROM plant_sensor WHERE plant_id = $1",
			"UPDATE plant SET parent_plant_id = NULL WHERE parent_plant_id = $1",
			"DELETE FROM plant WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete plant %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	paths := make([]string, len(images))
	for i, img := range images {
		paths[i] = img.Path
	}
	(&ImageService{deps: s.deps}).removeFiles(paths)
	return nil
}

// StatusHistory returns a plant's status history, oldest first.
func (s *PlantService) StatusHistory(ctx context.Context, id int) ([]StatusEntry, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	h, err := s.histories(ctx, []int{id})
	if err != nil {
		return nil, err
	}
	if h[id] == nil {
		return []StatusEntry{}, nil
	}
	return h[id], nil
}

// ChangeStatus moves a plant to statusID at at (now if nil). If the plant
// is already in that status nothing changes and changed is false; entry is
// then the current entry.
func (s *PlantService) ChangeStatus(ctx context.Context, plantID, statusID int, at *time.Time) (entry StatusEntry, changed bool, err error) {
	var v Validation
	if err := s.checkRef(ctx, &v, "status_id", "plant_status", statusID); err != nil {
		return StatusEntry{}, false, err
	}
	if err := v.Err(); err != nil {
		return StatusEntry{}, false, err
	}
	history, err := s.StatusHistory(ctx, plantID)
	if err != nil {
		return StatusEntry{}, false, err
	}
	if n := len(history); n > 0 && history[n-1].StatusID == statusID {
		return history[n-1], false, nil
	}
	when := s.deps.Zones.Now(s.deps.Clock)
	if at != nil {
		when = *at
	}
	var id int
	err = s.deps.DB.QueryRowContext(ctx,
		"INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3) RETURNING id",
		plantID, statusID, s.deps.Zones.ToWall(when)).Scan(&id)
	if err != nil {
		return StatusEntry{}, false, fmt.Errorf("change plant %d status: %w", plantID, err)
	}
	e, err := s.statusEntry(ctx, 0, id)
	return e, true, err
}

// statusEntry returns one history entry; plantID 0 matches any plant.
func (s *PlantService) statusEntry(ctx context.Context, plantID, entryID int) (StatusEntry, error) {
	var (
		e     StatusEntry
		owner int
		name  string
		at    chrono.Stored
	)
	err := s.deps.DB.QueryRowContext(ctx, `
		SELECT psl.id, psl.plant_id, psl.status_id, ps.status, psl.date
		FROM plant_status_log psl JOIN plant_status ps ON ps.id = psl.status_id
		WHERE psl.id = $1`, entryID).Scan(&e.ID, &owner, &e.StatusID, &name, &at)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && plantID != 0 && owner != plantID) {
		return StatusEntry{}, NotFound(CodeStatusEntryNotFound, fmt.Sprintf("status entry %d does not exist", entryID))
	}
	if err != nil {
		return StatusEntry{}, fmt.Errorf("get status entry %d: %w", entryID, err)
	}
	e.Status = StatusName(name)
	e.At = s.deps.Zones.FromWall(at.Time)
	return e, nil
}

// UpdateStatusEntry corrects a status history entry. plantID 0 matches
// the entry on any plant.
func (s *PlantService) UpdateStatusEntry(ctx context.Context, plantID, entryID int, upd StatusEntryUpdate) (StatusEntry, error) {
	var v Validation
	if upd.StatusID.Set {
		id := 0
		if upd.StatusID.Value != nil {
			id = *upd.StatusID.Value
		}
		if err := s.checkRef(ctx, &v, "status_id", "plant_status", id); err != nil {
			return StatusEntry{}, err
		}
	}
	if upd.At.Set && upd.At.Value == nil {
		v.Add("at", "is required")
	}
	if err := v.Err(); err != nil {
		return StatusEntry{}, err
	}
	if _, err := s.statusEntry(ctx, plantID, entryID); err != nil {
		return StatusEntry{}, err
	}
	if upd.StatusID.Set {
		if _, err := s.deps.DB.ExecContext(ctx, "UPDATE plant_status_log SET status_id = $1 WHERE id = $2",
			*upd.StatusID.Value, entryID); err != nil {
			return StatusEntry{}, fmt.Errorf("update status entry %d: %w", entryID, err)
		}
	}
	if upd.At.Set {
		if _, err := s.deps.DB.ExecContext(ctx, "UPDATE plant_status_log SET date = $1 WHERE id = $2",
			s.deps.Zones.ToWall(*upd.At.Value), entryID); err != nil {
			return StatusEntry{}, fmt.Errorf("update status entry %d: %w", entryID, err)
		}
	}
	return s.statusEntry(ctx, plantID, entryID)
}

// DeleteStatusEntry removes a status history entry. A plant's last entry
// can't be removed: every plant has a status. plantID 0 matches the entry
// on any plant.
func (s *PlantService) DeleteStatusEntry(ctx context.Context, plantID, entryID int) error {
	if _, err := s.statusEntry(ctx, plantID, entryID); err != nil {
		return err
	}
	var owner, count int
	err := s.deps.DB.QueryRowContext(ctx, `
		SELECT plant_id, (SELECT COUNT(*) FROM plant_status_log x WHERE x.plant_id = psl.plant_id)
		FROM plant_status_log psl WHERE psl.id = $1`, entryID).Scan(&owner, &count)
	if err != nil {
		return fmt.Errorf("count status entries: %w", err)
	}
	if count <= 1 {
		return Conflict(CodeLastStatusEntry, fmt.Sprintf("status entry %d is plant %d's only status", entryID, owner))
	}
	if _, err := s.deps.DB.ExecContext(ctx, "DELETE FROM plant_status_log WHERE id = $1", entryID); err != nil {
		return fmt.Errorf("delete status entry %d: %w", entryID, err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
