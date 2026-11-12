package service

import (
	"context"
	"fmt"
	"math"
	"time"
)

// MeasurementInput is a measurement to record on a plant.
type MeasurementInput struct {
	MetricID int
	Value    float64
	// At is when it was taken; zero means now.
	At time.Time
}

// MeasurementUpdate changes the fields that are Set.
type MeasurementUpdate struct {
	Value Patch[float64]
	At    Patch[time.Time]
}

// Error codes for measurements.
const CodeMeasurementNotFound = "measurement_not_found"

// MeasurementService records and reads plant measurements.
type MeasurementService struct {
	deps Deps
}

func (s *MeasurementService) plants() *PlantService { return &PlantService{deps: s.deps} }

func checkValue(v *Validation, value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		v.Add("value", "must be a number")
	}
}

// Create records a measurement on its own (not with an activity).
func (s *MeasurementService) Create(ctx context.Context, plantID int, in MeasurementInput) (Measurement, error) {
	var v Validation
	checkValue(&v, in.Value)
	if err := s.plants().checkRef(ctx, &v, "metric_id", "metric", in.MetricID); err != nil {
		return Measurement{}, err
	}
	if err := v.Err(); err != nil {
		return Measurement{}, err
	}
	if _, err := s.plants().Get(ctx, plantID); err != nil {
		return Measurement{}, err
	}
	at := in.At
	if at.IsZero() {
		at = s.deps.Zones.Now(s.deps.Clock)
	}
	var id int
	if err := s.deps.DB.QueryRowContext(ctx,
		"INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, $3, $4) RETURNING id",
		plantID, in.MetricID, in.Value, s.deps.Zones.ToWall(at)).Scan(&id); err != nil {
		return Measurement{}, fmt.Errorf("insert measurement: %w", err)
	}
	return s.Get(ctx, id)
}

// Get returns one measurement.
func (s *MeasurementService) Get(ctx context.Context, id int) (Measurement, error) {
	ms, err := s.plants().measurementsWhere(ctx, "pm.id = $1", []any{id}, 0)
	if err != nil {
		return Measurement{}, err
	}
	if len(ms) == 0 {
		return Measurement{}, NotFound(CodeMeasurementNotFound, fmt.Sprintf("measurement %d does not exist", id))
	}
	return ms[0], nil
}

// ForPlant returns a plant's measurements newest first, optionally of one
// metric, with the total count.
func (s *MeasurementService) ForPlant(ctx context.Context, plantID, metricID, offset, limit int) ([]Measurement, int, error) {
	if _, err := s.plants().Get(ctx, plantID); err != nil {
		return nil, 0, err
	}
	where, args := "pm.plant_id = $1", []any{plantID}
	if metricID > 0 {
		where += " AND pm.metric_id = $2"
		args = append(args, metricID)
	}
	var total int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM plant_measurements pm WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count measurements: %w", err)
	}
	if limit > 0 {
		where += fmt.Sprintf(" ORDER BY pm.date DESC, pm.id DESC LIMIT %d OFFSET %d", limit, offset)
		// measurementsWhere appends its own ORDER BY; wrap the page in a subquery instead.
		where = "pm.id IN (SELECT pm.id FROM plant_measurements pm WHERE " + where + ")"
	}
	ms, err := s.plants().measurementsWhere(ctx, where, args, 0)
	return ms, total, err
}

// Update changes a measurement's value or time. (Changing an activity's
// time moves the measurements recorded with it; this moves one.)
func (s *MeasurementService) Update(ctx context.Context, id int, upd MeasurementUpdate) (Measurement, error) {
	var v Validation
	if upd.Value.Set {
		if upd.Value.Value == nil {
			v.Add("value", "is required")
		} else {
			checkValue(&v, *upd.Value.Value)
		}
	}
	if upd.At.Set && upd.At.Value == nil {
		v.Add("at", "is required")
	}
	if err := v.Err(); err != nil {
		return Measurement{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return Measurement{}, err
	}
	if upd.Value.Set {
		if _, err := s.deps.DB.ExecContext(ctx, "UPDATE plant_measurements SET value = $1 WHERE id = $2", *upd.Value.Value, id); err != nil {
			return Measurement{}, fmt.Errorf("update measurement %d: %w", id, err)
		}
	}
	if upd.At.Set {
		if _, err := s.deps.DB.ExecContext(ctx, "UPDATE plant_measurements SET date = $1 WHERE id = $2",
			s.deps.Zones.ToWall(*upd.At.Value), id); err != nil {
			return Measurement{}, fmt.Errorf("update measurement %d: %w", id, err)
		}
	}
	return s.Get(ctx, id)
}

// Delete removes a measurement.
func (s *MeasurementService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	if _, err := s.deps.DB.ExecContext(ctx, "DELETE FROM plant_measurements WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete measurement %d: %w", id, err)
	}
	return nil
}
