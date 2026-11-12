package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"isley/chrono"
	"isley/utils"
)

// CodeSensorNotFound is the code for a missing sensor.
const CodeSensorNotFound = "sensor_not_found"

// Where a sensor is shown.
const (
	VisibilityZonePlant = "zone_plant"
	VisibilityZone      = "zone"
	VisibilityPlant     = "plant"
	VisibilityHide      = "hide"
)

func validVisibility(v string) bool {
	switch v {
	case VisibilityZonePlant, VisibilityZone, VisibilityPlant, VisibilityHide:
		return true
	}
	return false
}

// Sensor is a source of readings. Source, Device and Type identify it to
// the integrations that feed it and never change.
type Sensor struct {
	ID     int
	Name   string
	Zone   *Ref
	Source string
	Device string
	Type   string
	Unit   string
	// Visibility is "zone_plant", "zone", "plant" or "hide".
	Visibility string
	CreatedAt  time.Time
	// Latest is the most recent reading, or nil.
	Latest *Reading
}

// SensorFilter selects sensors.
type SensorFilter struct {
	ZoneID int
	Source string
	Offset int
	// Limit 0 means all.
	Limit int
}

// SensorUpdate changes the fields that are set. A nil ZoneID takes the
// sensor out of any zone.
type SensorUpdate struct {
	Name       Patch[string]
	ZoneID     Patch[int]
	Unit       Patch[string]
	Visibility Patch[string]
}

// DashboardSensor is a zone sensor with its latest reading, as the
// dashboard shows it.
type DashboardSensor struct {
	ID     int
	Zone   string
	Device string
	Type   string
	Name   string
	Unit   string
	Value  float64
	// Trend compares the reading with the rolling average: "up", "down" or
	// "flat".
	Trend string
	// PlantName is the growing plant the sensor is linked to, if any.
	PlantName string
}

// SensorService owns sensors.
type SensorService struct {
	deps Deps
}

func sensorNotFound(id int) error {
	return NotFound(CodeSensorNotFound, fmt.Sprintf("sensor %d does not exist", id))
}

func (s *SensorService) changed() {
	if s.deps.SensorsChanged != nil {
		s.deps.SensorsChanged()
	}
}

// The latest reading walks the (sensor_id, create_dt) index; MAX(id) made
// Postgres scan the primary key backwards past every newer row from other
// sensors.
const sensorSelect = `
	SELECT s.id, s.name, s.zone_id, z.name, s.source, s.device, s.type, s.unit, s.visibility, s.create_dt,
	       sd.value, sd.create_dt
	FROM sensors s
	LEFT JOIN zones z ON z.id = s.zone_id
	LEFT JOIN sensor_data sd ON sd.id = (
		SELECT id FROM sensor_data WHERE sensor_id = s.id ORDER BY create_dt DESC LIMIT 1
	)`

// List returns sensors by source, device, type and name, with the total
// that match.
func (s *SensorService) List(ctx context.Context, f SensorFilter) ([]Sensor, int, error) {
	var where []string
	var args []any
	if f.ZoneID > 0 {
		args = append(args, f.ZoneID)
		where = append(where, fmt.Sprintf("s.zone_id = $%d", len(args)))
	}
	if f.Source != "" {
		args = append(args, f.Source)
		where = append(where, fmt.Sprintf("s.source = $%d", len(args)))
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sensors s"+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count sensors: %w", err)
	}
	q := sensorSelect + cond + " ORDER BY s.source, s.device, s.type, s.name, s.id"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	out, err := s.query(ctx, q, args...)
	return out, total, err
}

// Get returns one sensor.
func (s *SensorService) Get(ctx context.Context, id int) (Sensor, error) {
	out, err := s.query(ctx, sensorSelect+" WHERE s.id = $1", id)
	if err != nil {
		return Sensor{}, err
	}
	if len(out) == 0 {
		return Sensor{}, sensorNotFound(id)
	}
	return out[0], nil
}

func (s *SensorService) query(ctx context.Context, q string, args ...any) ([]Sensor, error) {
	rows, err := s.deps.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list sensors: %w", err)
	}
	defer rows.Close()
	out := []Sensor{}
	for rows.Next() {
		var (
			sn       Sensor
			zoneID   sql.NullInt64
			zoneName sql.NullString
			created  chrono.Stored
			value    sql.NullFloat64
			at       chrono.Stored
		)
		if err := rows.Scan(&sn.ID, &sn.Name, &zoneID, &zoneName, &sn.Source, &sn.Device, &sn.Type, &sn.Unit,
			&sn.Visibility, &created, &value, &at); err != nil {
			return nil, fmt.Errorf("scan sensor: %w", err)
		}
		if zoneID.Valid && zoneName.Valid {
			sn.Zone = &Ref{ID: int(zoneID.Int64), Name: zoneName.String}
		}
		if created.Valid {
			sn.CreatedAt = s.deps.Zones.FromSystem(created.Time)
		}
		if value.Valid && at.Valid {
			sn.Latest = &Reading{Value: value.Float64, At: s.deps.Zones.FromSystem(at.Time)}
		}
		out = append(out, sn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sensors: %w", err)
	}
	return out, nil
}

// Update changes a sensor's name, zone, unit or visibility.
func (s *SensorService) Update(ctx context.Context, id int, upd SensorUpdate) (Sensor, error) {
	// Check the input before looking the sensor up, so a bad request is
	// reported as such whether or not the sensor exists.
	var v Validation
	str := func(p Patch[string]) string {
		if p.Value == nil {
			return ""
		}
		return strings.TrimSpace(*p.Value)
	}
	if upd.Name.Set {
		checkName(&v, "name", str(upd.Name))
	}
	if upd.Unit.Set && len(str(upd.Unit)) > utils.MaxUnitLength {
		v.Add("unit", fmt.Sprintf("must be at most %d characters", utils.MaxUnitLength))
	}
	if upd.Visibility.Set && !validVisibility(str(upd.Visibility)) {
		v.Add("visibility", "must be zone_plant, zone, plant or hide")
	}
	if upd.ZoneID.Set && upd.ZoneID.Value != nil {
		found, err := existingIDs(ctx, s.deps.DB, "zones", []int{*upd.ZoneID.Value})
		if err != nil {
			return Sensor{}, err
		}
		if !found[*upd.ZoneID.Value] {
			v.Add("zone_id", "does not exist")
		}
	}
	if err := v.Err(); err != nil {
		return Sensor{}, err
	}
	sn, err := s.Get(ctx, id)
	if err != nil {
		return Sensor{}, err
	}
	if upd.Name.Set {
		sn.Name = str(upd.Name)
	}
	if upd.Unit.Set {
		sn.Unit = str(upd.Unit)
	}
	if upd.Visibility.Set {
		sn.Visibility = str(upd.Visibility)
	}
	var zoneID *int
	if sn.Zone != nil {
		zoneID = &sn.Zone.ID
	}
	if upd.ZoneID.Set {
		zoneID = upd.ZoneID.Value
	}
	if _, err := s.deps.DB.ExecContext(ctx,
		"UPDATE sensors SET name = $1, zone_id = $2, unit = $3, visibility = $4 WHERE id = $5",
		sn.Name, zoneID, sn.Unit, sn.Visibility, id); err != nil {
		return Sensor{}, fmt.Errorf("update sensor %d: %w", id, err)
	}
	s.changed()
	return s.Get(ctx, id)
}

// Delete deletes a sensor with its readings, rollups and plant links, and
// clears it from any zone's VPD inputs.
func (s *SensorService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM sensor_data WHERE sensor_id = $1",
			"DELETE FROM sensor_data_hourly WHERE sensor_id = $1",
			"DELETE FROM rolling_averages WHERE sensor_id = $1",
			"DELETE FROM plant_sensor WHERE sensor_id = $1",
			"UPDATE zones SET vpd_temp_sensor_id = NULL WHERE vpd_temp_sensor_id = $1",
			"UPDATE zones SET vpd_humidity_sensor_id = NULL WHERE vpd_humidity_sensor_id = $1",
			"DELETE FROM sensors WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete sensor %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.changed()
	return refreshZones(ctx, s.deps.DB, s.deps.Store)
}

// Register returns the id of the sensor r identifies, adding it if there
// is none.
func (s *SensorService) Register(ctx context.Context, r SensorRegistration) (int, error) {
	id, created, err := registerSensor(ctx, s.deps.DB, r)
	if created {
		s.changed()
	}
	return id, err
}

// Dashboard returns every sensor shown on zone pages that has a reading,
// by zone, device and type.
func (s *SensorService) Dashboard(ctx context.Context) ([]DashboardSensor, error) {
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT s.id, z.name, s.device, s.type, s.name, s.unit, sd.value,
		       CASE WHEN sd.value > ra.avg_value THEN 'up'
		            WHEN sd.value < ra.avg_value THEN 'down'
		            ELSE 'flat' END
		FROM sensors s
		JOIN zones z ON z.id = s.zone_id
		JOIN sensor_data sd ON sd.id = (
			SELECT id FROM sensor_data WHERE sensor_id = s.id ORDER BY create_dt DESC LIMIT 1
		)
		LEFT JOIN rolling_averages ra ON ra.sensor_id = s.id
		WHERE s.visibility IN ('zone_plant', 'zone')
		ORDER BY z.name, s.device, s.type`)
	if err != nil {
		return nil, fmt.Errorf("dashboard sensors: %w", err)
	}
	defer rows.Close()
	out := []DashboardSensor{}
	for rows.Next() {
		var d DashboardSensor
		if err := rows.Scan(&d.ID, &d.Zone, &d.Device, &d.Type, &d.Name, &d.Unit, &d.Value, &d.Trend); err != nil {
			return nil, fmt.Errorf("scan dashboard sensor: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard sensors: %w", err)
	}
	plants, err := s.activePlantNames(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].PlantName = plants[out[i].ID]
	}
	return out, nil
}

// activePlantNames maps each sensor linked to a plant whose current status
// is active to that plant's name. Soil probes are 1:1 with plants; if two
// active plants share a sensor, the later plant wins.
func (s *SensorService) activePlantNames(ctx context.Context) (map[int]string, error) {
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT ps.sensor_id, p.name
		FROM plant p
		JOIN plant_sensor ps ON ps.plant_id = p.id
		JOIN plant_status st ON st.id = (
			SELECT psl.status_id FROM plant_status_log psl
			WHERE psl.plant_id = p.id ORDER BY psl.date DESC, psl.id DESC LIMIT 1)
		WHERE st.active = 1
		ORDER BY p.id`)
	if err != nil {
		return nil, fmt.Errorf("active plant sensors: %w", err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan active plant sensor: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// SensorRegistration identifies a sensor by (Source, Device, Type) and
// gives the values a new row gets.
type SensorRegistration struct {
	Name   string
	Source string
	Device string
	Type   string
	ZoneID *int
	Unit   string
}

// sensorRegisterMu serialises registration across the process: the
// sensors table has no unique key on (source, device, type), so two
// concurrent first readings would otherwise insert the sensor twice.
var sensorRegisterMu sync.Mutex

func registerSensor(ctx context.Context, db *sql.DB, r SensorRegistration) (int, bool, error) {
	sensorRegisterMu.Lock()
	defer sensorRegisterMu.Unlock()

	var id int
	created := false
	err := db.QueryRowContext(ctx,
		"SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3",
		r.Source, r.Device, r.Type).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		created = true
		err = db.QueryRowContext(ctx, `
			INSERT INTO sensors (name, source, device, type, zone_id, unit, visibility)
			VALUES ($1, $2, $3, $4, $5, $6, 'zone_plant')
			RETURNING id`,
			r.Name, r.Source, r.Device, r.Type, r.ZoneID, r.Unit).Scan(&id)
	}
	if err != nil {
		return 0, false, fmt.Errorf("register sensor %s/%s/%s: %w", r.Source, r.Device, r.Type, err)
	}
	return id, created, nil
}
