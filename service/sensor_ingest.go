package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"isley/chrono"
	"isley/utils"
)

// NewReading is a value for a sensor, stamped when it is stored.
type NewReading struct {
	SensorID int
	Value    float64
}

// Record stores readings in one transaction, so a batch lands whole or
// not at all. The database stamps them (in the system zone).
func (s *SensorService) Record(ctx context.Context, readings []NewReading) error {
	return inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, r := range readings {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO sensor_data (sensor_id, value) VALUES ($1, $2)", r.SensorID, r.Value); err != nil {
				return fmt.Errorf("record reading for sensor %d: %w", r.SensorID, err)
			}
		}
		return nil
	})
}

// RecordByIdentity stores a reading for the sensor (source, device, type)
// if that sensor is registered; tracked is false when it isn't. The
// integrations report every sensor a device has, and only the ones the
// user scanned in are kept.
func (s *SensorService) RecordByIdentity(ctx context.Context, source, device, sensorType string, value float64) (tracked bool, err error) {
	var id int
	err = s.deps.DB.QueryRowContext(ctx,
		"SELECT id FROM sensors WHERE source = $1 AND device = $2 AND type = $3", source, device, sensorType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find sensor %s/%s/%s: %w", source, device, sensorType, err)
	}
	return true, s.Record(ctx, []NewReading{{SensorID: id, Value: value}})
}

// Prune deletes readings older than retentionDays and returns how many it
// deleted. The hourly rollup is kept: it is the long-term history.
func (s *SensorService) Prune(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := s.deps.Clock.Now().AddDate(0, 0, -retentionDays)
	res, err := s.deps.DB.ExecContext(ctx,
		"DELETE FROM sensor_data WHERE create_dt < $1", s.deps.Zones.ToSystem(cutoff))
	if err != nil {
		return 0, fmt.Errorf("prune sensor data: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// Maintain reclaims space and refreshes planner statistics after a large
// delete. Only SQLite needs it; Postgres autovacuum handles its own.
func (s *SensorService) Maintain(ctx context.Context) error {
	if s.deps.Dialect.IsPostgres() {
		return nil
	}
	for _, q := range []string{"VACUUM", "ANALYZE", "PRAGMA optimize"} {
		if _, err := s.deps.DB.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlite %s: %w", q, err)
		}
	}
	return nil
}

// rollupWindow is how far back an incremental rollup re-summarises, an
// hour more than a day so late readings are caught.
const rollupWindow = 25 * time.Hour

// RefreshRollup summarises readings into sensor_data_hourly. With full
// set (or an empty rollup) it summarises every reading; otherwise the
// hours of the last 25 hours, replacing them.
func (s *SensorService) RefreshRollup(ctx context.Context, full bool) error {
	if !full {
		var one int
		err := s.deps.DB.QueryRowContext(ctx, "SELECT 1 FROM sensor_data_hourly LIMIT 1").Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			full = true
		} else if err != nil {
			return fmt.Errorf("check rollup: %w", err)
		}
	}
	where, args := "", []any{}
	if !full {
		// From the start of an hour: each hour it touches is summarised
		// whole. Starting mid-hour rewrote the oldest hour from its last
		// few minutes, and that partial row was then kept for good.
		since := s.deps.Clock.Now().Add(-rollupWindow).UTC().Truncate(time.Hour)
		where = "WHERE sd.create_dt >= $1"
		args = append(args, s.deps.Zones.ToSystem(since))
	}
	hour := "strftime('%Y-%m-%d %H:00:00', sd.create_dt)"
	insert, conflict := "INSERT OR REPLACE INTO", ""
	if s.deps.Dialect.IsPostgres() {
		hour = "date_trunc('hour', sd.create_dt)"
		insert = "INSERT INTO"
		conflict = `ON CONFLICT (sensor_id, bucket) DO UPDATE SET
			min_val = EXCLUDED.min_val, max_val = EXCLUDED.max_val,
			avg_val = EXCLUDED.avg_val, sample_count = EXCLUDED.sample_count`
	}
	q := fmt.Sprintf(`
		%s sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		SELECT sd.sensor_id, %s, MIN(sd.value), MAX(sd.value), AVG(sd.value), COUNT(*)
		FROM sensor_data sd
		%s
		GROUP BY sd.sensor_id, %s
		%s`, insert, hour, where, hour, conflict)
	if _, err := s.deps.DB.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("refresh rollup: %w", err)
	}
	return nil
}

// latestReading is a sensor's newest reading, or ok false.
func (s *SensorService) latestReading(ctx context.Context, id int) (v float64, unit string, at time.Time, ok bool, err error) {
	var stamp chrono.Stored
	err = s.deps.DB.QueryRowContext(ctx, `
		SELECT sd.value, s.unit, sd.create_dt
		FROM sensor_data sd JOIN sensors s ON s.id = sd.sensor_id
		WHERE sd.sensor_id = $1
		ORDER BY sd.create_dt DESC, sd.id DESC LIMIT 1`, id).Scan(&v, &unit, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", time.Time{}, false, nil
	}
	if err != nil {
		return 0, "", time.Time{}, false, fmt.Errorf("latest reading of sensor %d: %w", id, err)
	}
	return v, unit, s.deps.Zones.FromSystem(stamp.Time), true, nil
}

// VPDResult is what ComputeZoneVPD did for one zone.
type VPDResult struct {
	ZoneID int
	// VPD is the value recorded, or nil when the zone was skipped.
	VPD *float64
	// Skipped says why nothing was recorded.
	Skipped string
}

// ComputeZoneVPD records a VPD reading (kPa) for each zone with VPD set
// up (a leaf temperature offset and both input sensors), on the zone's
// derived "VPD" sensor. A zone is skipped when an input has no reading
// newer than maxAge, so a dead sensor doesn't keep producing fresh-looking
// VPD.
func (s *SensorService) ComputeZoneVPD(ctx context.Context, maxAge time.Duration) ([]VPDResult, error) {
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT id, leaf_temp_offset, vpd_temp_sensor_id, vpd_humidity_sensor_id
		FROM zones
		WHERE leaf_temp_offset IS NOT NULL
		  AND vpd_temp_sensor_id IS NOT NULL
		  AND vpd_humidity_sensor_id IS NOT NULL
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("VPD zones: %w", err)
	}
	type vpdZone struct {
		id            int
		offsetF       float64
		tempID, humID int
	}
	var zones []vpdZone
	for rows.Next() {
		var z vpdZone
		if err := rows.Scan(&z.id, &z.offsetF, &z.tempID, &z.humID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan VPD zone: %w", err)
		}
		zones = append(zones, z)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("VPD zones: %w", err)
	}

	now := s.deps.Clock.Now()
	var out []VPDResult
	for _, z := range zones {
		res := VPDResult{ZoneID: z.id}
		temp, tempUnit, tempAt, ok, err := s.latestReading(ctx, z.tempID)
		if err != nil {
			return nil, err
		}
		hum, _, humAt, ok2, err := s.latestReading(ctx, z.humID)
		if err != nil {
			return nil, err
		}
		switch {
		case !ok || !ok2:
			res.Skipped = "an input sensor has no readings"
		case now.Sub(tempAt) > maxAge || now.Sub(humAt) > maxAge:
			res.Skipped = "an input sensor's latest reading is too old"
		}
		if res.Skipped != "" {
			out = append(out, res)
			continue
		}
		// The offset is a temperature difference in °F, so it converts to
		// °C by 5/9 alone. The air temperature converts only when it isn't
		// already Celsius.
		airC := temp
		if !utils.IsCelsius(tempUnit) {
			airC = utils.FahrenheitToCelsius(temp)
		}
		vpd := utils.VPD(airC, hum, z.offsetF*5.0/9.0)

		zoneID := z.id
		device := strconv.Itoa(z.id)
		sensorID, err := s.Register(ctx, SensorRegistration{
			Name: "VPD (Zone " + device + ")", Source: "derived", Device: device, Type: "VPD",
			ZoneID: &zoneID, Unit: "kPa",
		})
		if err != nil {
			return nil, err
		}
		if err := s.Record(ctx, []NewReading{{SensorID: sensorID, Value: vpd}}); err != nil {
			return nil, err
		}
		res.VPD = &vpd
		out = append(out, res)
	}
	return out, nil
}
