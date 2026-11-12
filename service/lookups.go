package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"isley/config"
	"isley/model/types"
)

// Status is a plant status (plant_status). The set is fixed by migrations;
// only the VPD range can be edited.
type Status struct {
	ID    int
	Name  StatusName
	Order int
	// VPDLowKPa and VPDHighKPa are the target VPD range for plants in this
	// status, in kPa. Nil means no target.
	VPDLowKPa  *float64
	VPDHighKPa *float64
}

// Metric is a kind of measurement (metric): Height, pH, ...
type Metric struct {
	ID   int
	Name string
	Unit string
	// BuiltIn metrics ship with Isley and can't be deleted.
	BuiltIn bool
}

// ActivityType is a kind of activity that can be logged against a plant
// (activity): Water, Feed, Note, and user-defined ones.
type ActivityType struct {
	ID         int
	Name       string
	IsWatering bool
	IsFeeding  bool
	// BuiltIn activity types ship with Isley (Water, Feed, Note).
	BuiltIn bool
	// Metrics are the measurements the activity form asks for.
	Metrics []ActivityMetric
}

// ActivityMetric is a measurement an activity type asks for.
type ActivityMetric struct {
	MetricID int
	Name     string
	Unit     string
	Required bool
}

// MetricInput is a metric to create or the new state of one.
type MetricInput struct {
	Name string
	Unit string
}

// ActivityTypeInput is an activity type to create or the new state of one.
type ActivityTypeInput struct {
	Name       string
	IsWatering bool
	IsFeeding  bool
	Metrics    []ActivityMetricInput
}

// ActivityMetricInput links a metric to an activity type.
type ActivityMetricInput struct {
	MetricID int
	Required bool
}

const (
	maxNameLength = 255
	maxUnitLength = 50
)

// reservedActivityNames can't be used for a user-defined activity type:
// the built-in ones already have them.
var reservedActivityNames = map[string]bool{"Water": true, "Feed": true, "Note": true}

// Error codes for lookups.
const (
	CodeStatusNotFound           = "status_not_found"
	CodeMetricNotFound           = "metric_not_found"
	CodeMetricBuiltIn            = "metric_built_in"
	CodeActivityTypeNotFound     = "activity_type_not_found"
	CodeActivityTypeNameReserved = "activity_type_name_reserved"
)

// LookupService reads and edits the small reference lists: statuses,
// metrics and activity types.
type LookupService struct {
	deps Deps
}

// Statuses returns every status in lifecycle order.
func (s *LookupService) Statuses(ctx context.Context) ([]Status, error) {
	return listStatuses(ctx, s.deps.DB)
}

// Status returns one status.
func (s *LookupService) Status(ctx context.Context, id int) (Status, error) {
	all, err := s.Statuses(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, st := range all {
		if st.ID == id {
			return st, nil
		}
	}
	return Status{}, NotFound(CodeStatusNotFound, fmt.Sprintf("status %d does not exist", id))
}

// SetStatusVPD sets the target VPD range of a status. Nil clears a bound.
func (s *LookupService) SetStatusVPD(ctx context.Context, id int, lowKPa, highKPa *float64) (Status, error) {
	var v Validation
	if lowKPa != nil && *lowKPa < 0 {
		v.Add("vpd_low_kpa", "must not be negative")
	}
	if highKPa != nil && *highKPa < 0 {
		v.Add("vpd_high_kpa", "must not be negative")
	}
	if lowKPa != nil && highKPa != nil && *lowKPa > *highKPa {
		v.Add("vpd_low_kpa", "must not be above vpd_high_kpa")
	}
	if err := v.Err(); err != nil {
		return Status{}, err
	}
	res, err := s.deps.DB.ExecContext(ctx,
		"UPDATE plant_status SET vpd_low = $1, vpd_high = $2 WHERE id = $3", lowKPa, highKPa, id)
	if err != nil {
		return Status{}, fmt.Errorf("update status %d VPD: %w", id, err)
	}
	if err := requireRow(res, NotFound(CodeStatusNotFound, fmt.Sprintf("status %d does not exist", id))); err != nil {
		return Status{}, err
	}
	if err := refreshStatuses(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, id)
}

// Metrics returns every metric, oldest first.
func (s *LookupService) Metrics(ctx context.Context) ([]Metric, error) {
	return listMetrics(ctx, s.deps.DB)
}

// Metric returns one metric.
func (s *LookupService) Metric(ctx context.Context, id int) (Metric, error) {
	var m Metric
	var locked sql.NullBool
	err := s.deps.DB.QueryRowContext(ctx, "SELECT id, name, unit, lock FROM metric WHERE id = $1", id).
		Scan(&m.ID, &m.Name, &m.Unit, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return Metric{}, NotFound(CodeMetricNotFound, fmt.Sprintf("metric %d does not exist", id))
	}
	if err != nil {
		return Metric{}, fmt.Errorf("get metric %d: %w", id, err)
	}
	m.BuiltIn = locked.Bool
	return m, nil
}

func (in *MetricInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Unit = strings.TrimSpace(in.Unit)
	var v Validation
	checkName(&v, "name", in.Name)
	if len(in.Unit) > maxUnitLength {
		v.Add("unit", fmt.Sprintf("must be at most %d characters", maxUnitLength))
	}
	return v.Err()
}

// CreateMetric adds a metric.
func (s *LookupService) CreateMetric(ctx context.Context, in MetricInput) (Metric, error) {
	if err := in.validate(); err != nil {
		return Metric{}, err
	}
	var id int
	err := s.deps.DB.QueryRowContext(ctx,
		"INSERT INTO metric (name, unit) VALUES ($1, $2) RETURNING id", in.Name, in.Unit).Scan(&id)
	if err != nil {
		return Metric{}, fmt.Errorf("insert metric: %w", err)
	}
	if err := refreshMetrics(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Metric{}, err
	}
	return s.Metric(ctx, id)
}

// UpdateMetric renames a metric or changes its unit. Built-in metrics can
// be edited too.
func (s *LookupService) UpdateMetric(ctx context.Context, id int, in MetricInput) (Metric, error) {
	if err := in.validate(); err != nil {
		return Metric{}, err
	}
	res, err := s.deps.DB.ExecContext(ctx,
		"UPDATE metric SET name = $1, unit = $2 WHERE id = $3", in.Name, in.Unit, id)
	if err != nil {
		return Metric{}, fmt.Errorf("update metric %d: %w", id, err)
	}
	if err := requireRow(res, NotFound(CodeMetricNotFound, fmt.Sprintf("metric %d does not exist", id))); err != nil {
		return Metric{}, err
	}
	// Activity types embed metric names, so both caches change.
	if err := refreshMetrics(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Metric{}, err
	}
	if err := refreshActivityTypes(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Metric{}, err
	}
	return s.Metric(ctx, id)
}

// DeleteMetric deletes a metric with its measurements and its links to
// activity types. Built-in metrics can't be deleted.
func (s *LookupService) DeleteMetric(ctx context.Context, id int) error {
	m, err := s.Metric(ctx, id)
	if err != nil {
		return err
	}
	if m.BuiltIn {
		return Conflict(CodeMetricBuiltIn, fmt.Sprintf("metric %q is built in and can't be deleted", m.Name))
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM activity_metric WHERE metric_id = $1",
			"DELETE FROM plant_measurements WHERE metric_id = $1",
			"DELETE FROM metric WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete metric %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := refreshMetrics(ctx, s.deps.DB, s.deps.Store); err != nil {
		return err
	}
	return refreshActivityTypes(ctx, s.deps.DB, s.deps.Store)
}

// ActivityTypes returns every activity type by name.
func (s *LookupService) ActivityTypes(ctx context.Context) ([]ActivityType, error) {
	return listActivityTypes(ctx, s.deps.DB)
}

// ActivityType returns one activity type.
func (s *LookupService) ActivityType(ctx context.Context, id int) (ActivityType, error) {
	all, err := s.ActivityTypes(ctx)
	if err != nil {
		return ActivityType{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return ActivityType{}, NotFound(CodeActivityTypeNotFound, fmt.Sprintf("activity type %d does not exist", id))
}

// validate checks in and normalises its metric list: ids of 0 or less are
// dropped (the settings form sends them for empty rows) and a metric listed
// twice is required if any listing says so.
func (in *ActivityTypeInput) validate(ctx context.Context, q querier) error {
	in.Name = strings.TrimSpace(in.Name)
	var v Validation
	checkName(&v, "name", in.Name)

	required := map[int]bool{}
	for _, m := range in.Metrics {
		if m.MetricID <= 0 {
			continue
		}
		required[m.MetricID] = required[m.MetricID] || m.Required
	}
	ids := make([]int, 0, len(required))
	for id := range required {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	in.Metrics = in.Metrics[:0]
	for _, id := range ids {
		in.Metrics = append(in.Metrics, ActivityMetricInput{MetricID: id, Required: required[id]})
	}

	if len(ids) > 0 {
		known, err := existingIDs(ctx, q, "metric", ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !known[id] {
				v.Add("metrics", fmt.Sprintf("metric %d does not exist", id))
				break
			}
		}
	}
	return v.Err()
}

// CreateActivityType adds an activity type.
func (s *LookupService) CreateActivityType(ctx context.Context, in ActivityTypeInput) (ActivityType, error) {
	if err := in.validate(ctx, s.deps.DB); err != nil {
		return ActivityType{}, err
	}
	if reservedActivityNames[in.Name] {
		return ActivityType{}, Conflict(CodeActivityTypeNameReserved,
			fmt.Sprintf("%q is the name of a built-in activity type", in.Name))
	}
	var id int
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			"INSERT INTO activity (name, is_watering, is_feeding) VALUES ($1, $2, $3) RETURNING id",
			in.Name, in.IsWatering, in.IsFeeding).Scan(&id)
		if err != nil {
			return fmt.Errorf("insert activity type: %w", err)
		}
		return saveActivityMetrics(ctx, tx, id, in.Metrics)
	})
	if err != nil {
		return ActivityType{}, err
	}
	if err := refreshActivityTypes(ctx, s.deps.DB, s.deps.Store); err != nil {
		return ActivityType{}, err
	}
	return s.ActivityType(ctx, id)
}

// UpdateActivityType replaces an activity type's name, flags and metrics.
// Built-in types can be edited too, but nothing can be renamed to a
// built-in name it doesn't already have.
func (s *LookupService) UpdateActivityType(ctx context.Context, id int, in ActivityTypeInput) (ActivityType, error) {
	if err := in.validate(ctx, s.deps.DB); err != nil {
		return ActivityType{}, err
	}
	current, err := s.ActivityType(ctx, id)
	if err != nil {
		return ActivityType{}, err
	}
	if reservedActivityNames[in.Name] && current.Name != in.Name {
		return ActivityType{}, Conflict(CodeActivityTypeNameReserved,
			fmt.Sprintf("%q is the name of a built-in activity type", in.Name))
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE activity SET name = $1, is_watering = $2, is_feeding = $3 WHERE id = $4",
			in.Name, in.IsWatering, in.IsFeeding, id)
		if err != nil {
			return fmt.Errorf("update activity type %d: %w", id, err)
		}
		return saveActivityMetrics(ctx, tx, id, in.Metrics)
	})
	if err != nil {
		return ActivityType{}, err
	}
	if err := refreshActivityTypes(ctx, s.deps.DB, s.deps.Store); err != nil {
		return ActivityType{}, err
	}
	return s.ActivityType(ctx, id)
}

// DeleteActivityType deletes an activity type and every logged activity of
// that type.
func (s *LookupService) DeleteActivityType(ctx context.Context, id int) error {
	if _, err := s.ActivityType(ctx, id); err != nil {
		return err
	}
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM plant_activity WHERE activity_id = $1",
			"DELETE FROM activity_metric WHERE activity_id = $1",
			"DELETE FROM activity WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete activity type %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return refreshActivityTypes(ctx, s.deps.DB, s.deps.Store)
}

func saveActivityMetrics(ctx context.Context, tx *sql.Tx, activityID int, metrics []ActivityMetricInput) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM activity_metric WHERE activity_id = $1", activityID); err != nil {
		return fmt.Errorf("clear activity type metrics: %w", err)
	}
	for _, m := range metrics {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO activity_metric (activity_id, metric_id, required) VALUES ($1, $2, $3)",
			activityID, m.MetricID, m.Required)
		if err != nil {
			return fmt.Errorf("link metric %d to activity type: %w", m.MetricID, err)
		}
	}
	return nil
}

// checkName records a problem when a required name is blank or too long.
func checkName(v *Validation, field, name string) {
	switch {
	case name == "":
		v.Add(field, "is required")
	case len(name) > maxNameLength:
		v.Add(field, fmt.Sprintf("must be at most %d characters", maxNameLength))
	}
}

// requireRow returns notFound when res affected no rows.
func requireRow(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

// existingIDs reports which of ids exist in table.
func existingIDs(ctx context.Context, q querier, table string, ids []int) (map[int]bool, error) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := q.QueryContext(ctx,
		fmt.Sprintf("SELECT id FROM %s WHERE id IN (%s)", table, Placeholders(1, len(ids))), args...)
	if err != nil {
		return nil, fmt.Errorf("look up %s ids: %w", table, err)
	}
	defer rows.Close()
	found := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s id: %w", table, err)
		}
		found[id] = true
	}
	return found, rows.Err()
}

func listStatuses(ctx context.Context, q querier) ([]Status, error) {
	rows, err := q.QueryContext(ctx,
		"SELECT id, status, status_order, vpd_low, vpd_high FROM plant_status ORDER BY status_order, id")
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	defer rows.Close()
	var out []Status
	for rows.Next() {
		var st Status
		var name string
		var low, high sql.NullFloat64
		if err := rows.Scan(&st.ID, &name, &st.Order, &low, &high); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		st.Name = StatusName(name)
		st.VPDLowKPa, st.VPDHighKPa = nullFloat(low), nullFloat(high)
		out = append(out, st)
	}
	return out, rows.Err()
}

func listMetrics(ctx context.Context, q querier) ([]Metric, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, name, unit, lock FROM metric ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	defer rows.Close()
	var out []Metric
	for rows.Next() {
		var m Metric
		var locked sql.NullBool
		if err := rows.Scan(&m.ID, &m.Name, &m.Unit, &locked); err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		m.BuiltIn = locked.Bool
		out = append(out, m)
	}
	return out, rows.Err()
}

func listActivityTypes(ctx context.Context, q querier) ([]ActivityType, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT a.id, a.name, a.is_watering, a.is_feeding, a.lock,
		       m.id, m.name, m.unit, am.required
		FROM activity a
		LEFT JOIN activity_metric am ON am.activity_id = a.id
		LEFT JOIN metric m ON m.id = am.metric_id
		ORDER BY a.name, a.id, m.id`)
	if err != nil {
		return nil, fmt.Errorf("list activity types: %w", err)
	}
	defer rows.Close()
	var out []ActivityType
	index := map[int]int{}
	for rows.Next() {
		var (
			a          ActivityType
			locked     sql.NullBool
			metricID   sql.NullInt64
			metricName sql.NullString
			metricUnit sql.NullString
			required   sql.NullBool
		)
		if err := rows.Scan(&a.ID, &a.Name, &a.IsWatering, &a.IsFeeding, &locked,
			&metricID, &metricName, &metricUnit, &required); err != nil {
			return nil, fmt.Errorf("scan activity type: %w", err)
		}
		i, seen := index[a.ID]
		if !seen {
			a.BuiltIn = locked.Bool
			a.Metrics = []ActivityMetric{}
			out = append(out, a)
			i = len(out) - 1
			index[a.ID] = i
		}
		if metricID.Valid {
			out[i].Metrics = append(out[i].Metrics, ActivityMetric{
				MetricID: int(metricID.Int64),
				Name:     metricName.String,
				Unit:     metricUnit.String,
				Required: required.Bool,
			})
		}
	}
	return out, rows.Err()
}

// The config store holds model/types values for the templates; these
// convert and refresh it.

func refreshStatuses(ctx context.Context, q querier, store *config.Store) error {
	all, err := listStatuses(ctx, q)
	if err != nil {
		return err
	}
	out := make([]types.Status, len(all))
	for i, st := range all {
		out[i] = types.Status{ID: uint(st.ID), Status: string(st.Name), VPDLow: st.VPDLowKPa, VPDHigh: st.VPDHighKPa}
	}
	store.SetStatuses(out)
	return nil
}

func refreshMetrics(ctx context.Context, q querier, store *config.Store) error {
	all, err := listMetrics(ctx, q)
	if err != nil {
		return err
	}
	out := make([]types.Metric, len(all))
	for i, m := range all {
		out[i] = types.Metric{ID: m.ID, Name: m.Name, Unit: m.Unit}
	}
	store.SetMetrics(out)
	return nil
}

func refreshActivityTypes(ctx context.Context, q querier, store *config.Store) error {
	all, err := listActivityTypes(ctx, q)
	if err != nil {
		return err
	}
	out := make([]types.Activity, len(all))
	for i, a := range all {
		links := make([]types.ActivityMetricLink, len(a.Metrics))
		for j, m := range a.Metrics {
			links[j] = types.ActivityMetricLink{MetricID: m.MetricID, Required: m.Required}
		}
		out[i] = types.Activity{ID: a.ID, Name: a.Name, IsWatering: a.IsWatering, IsFeeding: a.IsFeeding, Metrics: links}
	}
	store.SetActivities(out)
	return nil
}

// RefreshStore reloads every cache this package owns (statuses, metrics,
// activity types, zones, breeders, strains, streams, smoke criteria) from db. Startup and backup restore call it; the
// service's own writes refresh what they touch.
func RefreshStore(ctx context.Context, db *sql.DB, store *config.Store) error {
	for _, refresh := range []func(context.Context, querier, *config.Store) error{
		refreshStatuses, refreshMetrics, refreshActivityTypes, refreshZones, refreshBreeders, refreshStrains, refreshStreams, refreshSmokeCriteria,
	} {
		if err := refresh(ctx, db, store); err != nil {
			return err
		}
	}
	return nil
}
