package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"isley/chrono"
	"isley/config"
	"isley/model/types"
)

// Zone is a grow space (tent, room, outdoor bed). Plants, sensors and
// streams belong to a zone.
type Zone struct {
	ID   int
	Name string
	// LeafTempOffsetF is how much cooler leaves are than the air, in °F
	// (a temperature difference). Non-nil turns on the zone's derived VPD
	// sensor.
	LeafTempOffsetF *float64
	// VPDTempSensorID and VPDHumiditySensorID are the sensors VPD is
	// computed from. Nil means the zone's first matching sensor.
	VPDTempSensorID     *int
	VPDHumiditySensorID *int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ZoneDetail is a zone with its sensors.
type ZoneDetail struct {
	Zone
	Sensors []ZoneSensor
}

// ZoneSensor is a sensor in a zone with its latest reading.
type ZoneSensor struct {
	ID     int
	Name   string
	Source string
	Device string
	Type   string
	Unit   string
	// Visibility is where the sensor is shown: "zone_plant", "zone",
	// "plant" or "hide".
	Visibility string
	// Latest is the most recent reading, or nil if there is none.
	Latest *Reading
}

// Reading is one sensor value.
type Reading struct {
	Value float64
	At    time.Time
}

// ZoneInput is a zone to create.
type ZoneInput struct {
	Name                string
	LeafTempOffsetF     *float64
	VPDTempSensorID     *int
	VPDHumiditySensorID *int
}

// ZoneUpdate changes the fields that are Set.
type ZoneUpdate struct {
	Name                Patch[string]
	LeafTempOffsetF     Patch[float64]
	VPDTempSensorID     Patch[int]
	VPDHumiditySensorID Patch[int]
}

// Error codes for zones.
const (
	CodeZoneNotFound = "zone_not_found"
	CodeZoneNotEmpty = "zone_not_empty"
)

// ZoneService reads and edits zones.
type ZoneService struct {
	deps Deps
}

const zoneColumns = "id, name, leaf_temp_offset, vpd_temp_sensor_id, vpd_humidity_sensor_id, create_dt, update_dt"

func (s *ZoneService) scanZone(row interface{ Scan(...any) error }) (Zone, error) {
	var (
		z               Zone
		offset          sql.NullFloat64
		tempID, humidID sql.NullInt64
		created, update chrono.Stored
	)
	if err := row.Scan(&z.ID, &z.Name, &offset, &tempID, &humidID, &created, &update); err != nil {
		return Zone{}, err
	}
	z.LeafTempOffsetF = nullFloat(offset)
	z.VPDTempSensorID, z.VPDHumiditySensorID = nullInt(tempID), nullInt(humidID)
	z.CreatedAt = s.deps.Zones.FromSystem(created.Time)
	z.UpdatedAt = s.deps.Zones.FromSystem(update.Time)
	return z, nil
}

// All returns every zone, oldest first.
func (s *ZoneService) All(ctx context.Context) ([]Zone, error) {
	zones, _, err := s.Page(ctx, 0, 0)
	return zones, err
}

// Page returns up to limit zones with ids above afterID, oldest first, and
// whether more follow. A limit of 0 returns them all.
func (s *ZoneService) Page(ctx context.Context, afterID, limit int) ([]Zone, bool, error) {
	query := "SELECT " + zoneColumns + " FROM zones WHERE id > $1 ORDER BY id"
	args := []any{afterID}
	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit+1)
	}
	rows, err := s.deps.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	var out []Zone
	for rows.Next() {
		z, err := s.scanZone(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan zone: %w", err)
		}
		out = append(out, z)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list zones: %w", err)
	}
	more := limit > 0 && len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}

// Get returns one zone.
func (s *ZoneService) Get(ctx context.Context, id int) (Zone, error) {
	z, err := s.scanZone(s.deps.DB.QueryRowContext(ctx, "SELECT "+zoneColumns+" FROM zones WHERE id = $1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Zone{}, zoneNotFound(id)
	}
	if err != nil {
		return Zone{}, fmt.Errorf("get zone %d: %w", id, err)
	}
	return z, nil
}

// Detail returns a zone with its sensors and their latest readings.
func (s *ZoneService) Detail(ctx context.Context, id int) (ZoneDetail, error) {
	z, err := s.Get(ctx, id)
	if err != nil {
		return ZoneDetail{}, err
	}
	// The latest reading walks the (sensor_id, create_dt) index; see
	// GetGroupedSensorsWithLatestReading for why it isn't MAX(id).
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT s.id, s.name, s.source, s.device, s.type, s.unit, s.visibility, sd.value, sd.create_dt
		FROM sensors s
		LEFT JOIN sensor_data sd ON sd.id = (
			SELECT id FROM sensor_data WHERE sensor_id = s.id ORDER BY create_dt DESC LIMIT 1
		)
		WHERE s.zone_id = $1
		ORDER BY s.device, s.type, s.id`, id)
	if err != nil {
		return ZoneDetail{}, fmt.Errorf("list zone %d sensors: %w", id, err)
	}
	defer rows.Close()
	d := ZoneDetail{Zone: z, Sensors: []ZoneSensor{}}
	for rows.Next() {
		var (
			sn    ZoneSensor
			value sql.NullFloat64
			at    chrono.Stored
		)
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.Source, &sn.Device, &sn.Type, &sn.Unit, &sn.Visibility, &value, &at); err != nil {
			return ZoneDetail{}, fmt.Errorf("scan zone sensor: %w", err)
		}
		if value.Valid && at.Valid {
			sn.Latest = &Reading{Value: value.Float64, At: s.deps.Zones.FromSystem(at.Time)}
		}
		d.Sensors = append(d.Sensors, sn)
	}
	return d, rows.Err()
}

// ByName returns the oldest zone called name.
func (s *ZoneService) ByName(ctx context.Context, name string) (Zone, error) {
	z, err := s.scanZone(s.deps.DB.QueryRowContext(ctx,
		"SELECT "+zoneColumns+" FROM zones WHERE name = $1 ORDER BY id LIMIT 1", strings.TrimSpace(name)))
	if errors.Is(err, sql.ErrNoRows) {
		return Zone{}, NotFound(CodeZoneNotFound, fmt.Sprintf("no zone is called %q", name))
	}
	if err != nil {
		return Zone{}, fmt.Errorf("find zone %q: %w", name, err)
	}
	return z, nil
}

// FindOrCreate returns the zone called name, creating it if there is none.
// The web forms that offer "new zone" use it.
func (s *ZoneService) FindOrCreate(ctx context.Context, name string) (Zone, error) {
	z, err := s.ByName(ctx, name)
	if err == nil || !IsKind(err, KindNotFound) {
		return z, err
	}
	return s.Create(ctx, ZoneInput{Name: name})
}

func (s *ZoneService) validateVPD(ctx context.Context, v *Validation, offset *float64, tempID, humidID *int) error {
	if offset != nil && (math.IsNaN(*offset) || math.IsInf(*offset, 0)) {
		v.Add("leaf_temp_offset_f", "must be a number")
	}
	var ids []int
	for _, id := range []*int{tempID, humidID} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	known, err := existingIDs(ctx, s.deps.DB, "sensors", ids)
	if err != nil {
		return err
	}
	if tempID != nil && !known[*tempID] {
		v.Add("vpd_temp_sensor_id", fmt.Sprintf("sensor %d does not exist", *tempID))
	}
	if humidID != nil && !known[*humidID] {
		v.Add("vpd_humidity_sensor_id", fmt.Sprintf("sensor %d does not exist", *humidID))
	}
	return nil
}

// Create adds a zone.
func (s *ZoneService) Create(ctx context.Context, in ZoneInput) (Zone, error) {
	in.Name = strings.TrimSpace(in.Name)
	var v Validation
	checkName(&v, "name", in.Name)
	if err := s.validateVPD(ctx, &v, in.LeafTempOffsetF, in.VPDTempSensorID, in.VPDHumiditySensorID); err != nil {
		return Zone{}, err
	}
	if err := v.Err(); err != nil {
		return Zone{}, err
	}
	var id int
	err := s.deps.DB.QueryRowContext(ctx, `
		INSERT INTO zones (name, leaf_temp_offset, vpd_temp_sensor_id, vpd_humidity_sensor_id)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		in.Name, in.LeafTempOffsetF, in.VPDTempSensorID, in.VPDHumiditySensorID).Scan(&id)
	if err != nil {
		return Zone{}, fmt.Errorf("insert zone: %w", err)
	}
	return s.afterWrite(ctx, id, in.LeafTempOffsetF != nil)
}

// Update changes the fields of a zone that upd sets.
func (s *ZoneService) Update(ctx context.Context, id int, upd ZoneUpdate) (Zone, error) {
	// Check the input before looking the zone up, so a bad request is
	// reported as such whether or not the zone exists.
	var v Validation
	var name string
	if upd.Name.Set {
		if upd.Name.Value != nil {
			name = strings.TrimSpace(*upd.Name.Value)
		}
		checkName(&v, "name", name)
		if err := v.Err(); err != nil {
			return Zone{}, err
		}
	}
	z, err := s.Get(ctx, id)
	if err != nil {
		return Zone{}, err
	}
	if upd.Name.Set {
		z.Name = name
	}
	if upd.LeafTempOffsetF.Set {
		z.LeafTempOffsetF = upd.LeafTempOffsetF.Value
	}
	if upd.VPDTempSensorID.Set {
		z.VPDTempSensorID = upd.VPDTempSensorID.Value
	}
	if upd.VPDHumiditySensorID.Set {
		z.VPDHumiditySensorID = upd.VPDHumiditySensorID.Value
	}
	if err := s.validateVPD(ctx, &v, z.LeafTempOffsetF, z.VPDTempSensorID, z.VPDHumiditySensorID); err != nil {
		return Zone{}, err
	}
	if err := v.Err(); err != nil {
		return Zone{}, err
	}
	_, err = s.deps.DB.ExecContext(ctx, `
		UPDATE zones SET name = $1, leaf_temp_offset = $2, vpd_temp_sensor_id = $3, vpd_humidity_sensor_id = $4
		WHERE id = $5`,
		z.Name, z.LeafTempOffsetF, z.VPDTempSensorID, z.VPDHumiditySensorID, id)
	if err != nil {
		return Zone{}, fmt.Errorf("update zone %d: %w", id, err)
	}
	return s.afterWrite(ctx, id, z.LeafTempOffsetF != nil)
}

// afterWrite makes sure a VPD-enabled zone has its derived VPD sensor,
// refreshes the cache and returns the zone as stored.
func (s *ZoneService) afterWrite(ctx context.Context, id int, vpdEnabled bool) (Zone, error) {
	if vpdEnabled {
		zoneID := id
		_, err := (&SensorService{deps: s.deps}).Register(ctx, SensorRegistration{
			Name:   "VPD (Zone " + strconv.Itoa(id) + ")",
			Source: "derived",
			Device: strconv.Itoa(id),
			Type:   "VPD",
			ZoneID: &zoneID,
			Unit:   "kPa",
		})
		if err != nil {
			return Zone{}, fmt.Errorf("zone %d VPD sensor: %w", id, err)
		}
	}
	if err := refreshZones(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Zone{}, err
	}
	return s.Get(ctx, id)
}

// Delete deletes an empty zone. A zone that still has plants, sensors or
// streams is a Conflict: move or delete them first. (The settings page
// deletes them for the user before calling this; the API makes the client
// do it, so one call can't wipe out a grow.)
func (s *ZoneService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	var plants, sensors, streams int
	err := s.deps.DB.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM plant WHERE zone_id = $1),
		       (SELECT COUNT(*) FROM sensors WHERE zone_id = $1),
		       (SELECT COUNT(*) FROM streams WHERE zone_id = $1)`, id).Scan(&plants, &sensors, &streams)
	if err != nil {
		return fmt.Errorf("count zone %d contents: %w", id, err)
	}
	if plants+sensors+streams > 0 {
		return Conflict(CodeZoneNotEmpty, fmt.Sprintf(
			"zone %d is not empty (plants: %d, sensors: %d, streams: %d); move or delete them first",
			id, plants, sensors, streams))
	}
	if _, err := s.deps.DB.ExecContext(ctx, "DELETE FROM zones WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete zone %d: %w", id, err)
	}
	return refreshZones(ctx, s.deps.DB, s.deps.Store)
}

func zoneNotFound(id int) error {
	return NotFound(CodeZoneNotFound, fmt.Sprintf("zone %d does not exist", id))
}

func refreshZones(ctx context.Context, q querier, store *config.Store) error {
	rows, err := q.QueryContext(ctx,
		"SELECT id, name, leaf_temp_offset, vpd_temp_sensor_id, vpd_humidity_sensor_id FROM zones ORDER BY id")
	if err != nil {
		return fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	var out []types.Zone
	for rows.Next() {
		var (
			z               types.Zone
			offset          sql.NullFloat64
			tempID, humidID sql.NullInt64
		)
		if err := rows.Scan(&z.ID, &z.Name, &offset, &tempID, &humidID); err != nil {
			return fmt.Errorf("scan zone: %w", err)
		}
		z.LeafTempOffset = nullFloat(offset)
		if tempID.Valid {
			v := uint(tempID.Int64)
			z.VPDTempSensorID = &v
		}
		if humidID.Valid {
			v := uint(humidID.Int64)
			z.VPDHumiditySensorID = &v
		}
		out = append(out, z)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list zones: %w", err)
	}
	store.SetZones(out)
	return nil
}
