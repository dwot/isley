package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"isley/chrono"
	"isley/logger"
)

// TimeStorageSetting names the settings row that records how instants are
// stored. Its value is "utc" once the database has been converted (#141
// Phase 2). It lives in settings so it travels with backups and the
// SQLite→Postgres copy: data and its convention move together.
const TimeStorageSetting = "time_storage"

// systemZoneSetting, when present, says which zone the data's system
// stamps were written in, overriding the server's: the SQLite→Postgres
// copy sets it to UTC, since the stamps came from SQLite.
const systemZoneSetting = "time_storage_system_zone"

// column is a timestamp column.
type column struct{ table, name string }

// wallColumns hold instants people entered, stored before the conversion
// as the instance zone's wall time.
var wallColumns = []column{
	{"plant_status_log", "date"},
	{"plant_activity", "date"},
	{"plant_measurements", "date"},
}

// systemColumns are stamped by CURRENT_TIMESTAMP: UTC on SQLite, the
// session zone on Postgres. sensor_data_hourly is handled on its own: hours
// that still have readings are recomputed, older ones shifted.
var systemColumns = []column{
	{"settings", "create_dt"}, {"settings", "update_dt"},
	{"zones", "create_dt"}, {"zones", "update_dt"},
	{"sensors", "create_dt"}, {"sensors", "update_dt"},
	{"sensor_data", "create_dt"},
	{"rolling_averages", "create_dt"},
	{"plant_images", "created_at"}, {"plant_images", "updated_at"},
	{"api_keys", "create_dt"}, {"api_keys", "update_dt"}, {"api_keys", "last_used"},
	{"smoke_report", "created_at"}, {"smoke_report", "updated_at"},
	{"idempotency_key", "created_at"},
}

// triggeredTables have triggers that rewrite update_dt on UPDATE; they are
// disabled while their stamps are converted.
var triggeredTables = []string{"settings", "zones", "sensors"}

// StoresUTC reports whether the database has been converted.
func StoresUTC(ctx context.Context, q querier) (bool, error) {
	var v string
	err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = $1", TimeStorageSetting).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", TimeStorageSetting, err)
	}
	return v == "utc", nil
}

// EnsureUTCStorage converts the database to store every instant in UTC,
// once; it does nothing if the conversion has been recorded. wall is the
// zone wall times were written in (the instance zone); system is the zone
// CURRENT_TIMESTAMP stamped in (UTC for SQLite, the server's default for
// Postgres). Calendar days (plant start, image day, report day) are not
// touched. It all happens in one transaction with the flag.
func EnsureUTCStorage(ctx context.Context, db *sql.DB, dialect Dialect, wall, system *time.Location) (converted bool, err error) {
	done, err := StoresUTC(ctx, db)
	if err != nil || done {
		return false, err
	}
	var override string
	err = db.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = $1", systemZoneSetting).Scan(&override)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read %s: %w", systemZoneSetting, err)
	}
	if override != "" {
		loc, err := time.LoadLocation(override)
		if err != nil {
			return false, fmt.Errorf("%s %q: %w", systemZoneSetting, override, err)
		}
		system = loc
	}
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		if dialect.IsPostgres() {
			if err := convertPostgres(ctx, tx, wall, system); err != nil {
				return err
			}
		} else if err := convertSQLite(ctx, tx, wall); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO settings (name, value) VALUES ($1, 'utc')", TimeStorageSetting); err != nil {
			return fmt.Errorf("record %s: %w", TimeStorageSetting, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE name = $1", systemZoneSetting); err != nil {
			return fmt.Errorf("clear %s: %w", systemZoneSetting, err)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("convert stored times to UTC: %w", err)
	}
	logger.Log.WithFields(map[string]any{"wall_zone": wall.String(), "system_zone": system.String()}).
		Info("Converted stored times to UTC")
	return true, nil
}

// convertPostgres shifts timestamp columns with AT TIME ZONE, which follows
// each value's own DST offset.
func convertPostgres(ctx context.Context, tx *sql.Tx, wall, system *time.Location) error {
	shift := func(c column, from *time.Location) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"UPDATE %s SET %s = (%s AT TIME ZONE $1) AT TIME ZONE 'UTC' WHERE %s IS NOT NULL",
			c.table, c.name, c.name, c.name), from.String()); err != nil {
			return fmt.Errorf("convert %s.%s: %w", c.table, c.name, err)
		}
		return nil
	}
	if !isUTC(wall) {
		for _, c := range wallColumns {
			if err := shift(c, wall); err != nil {
				return err
			}
		}
	}
	if isUTC(system) {
		return nil
	}
	// The hourly rollup: hours after the earliest hour of readings are
	// recomputed from the converted readings below; that hour (pruning cut
	// it part way) and older ones are all that is left of their readings,
	// so they are shifted like any other stamp.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sensor_data_hourly
		WHERE bucket > (SELECT date_trunc('hour', MIN(create_dt)) FROM sensor_data)`); err != nil {
		return fmt.Errorf("clear recomputable rollup hours: %w", err)
	}
	if err := shift(column{"sensor_data_hourly", "bucket"}, system); err != nil {
		return err
	}
	for _, t := range triggeredTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DISABLE TRIGGER USER", t)); err != nil {
			return fmt.Errorf("pause %s triggers: %w", t, err)
		}
	}
	for _, c := range systemColumns {
		if err := shift(c, system); err != nil {
			return err
		}
	}
	for _, t := range triggeredTables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ENABLE TRIGGER USER", t)); err != nil {
			return fmt.Errorf("resume %s triggers: %w", t, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		SELECT sensor_id, date_trunc('hour', create_dt), MIN(value), MAX(value), AVG(value), COUNT(*)
		FROM sensor_data
		WHERE create_dt >= (SELECT date_trunc('hour', MIN(create_dt)) + INTERVAL '1 hour' FROM sensor_data)
		GROUP BY sensor_id, date_trunc('hour', create_dt)
		ON CONFLICT (sensor_id, bucket) DO UPDATE SET
			min_val = EXCLUDED.min_val, max_val = EXCLUDED.max_val,
			avg_val = EXCLUDED.avg_val, sample_count = EXCLUDED.sample_count`); err != nil {
		return fmt.Errorf("recompute hourly rollup: %w", err)
	}
	return nil
}

// convertSQLite rewrites wall times row by row: they are text in several
// layouts, and SQLite has no zone database. Every value comes out in one
// layout, which also makes them sort correctly as text. System stamps are
// already UTC on SQLite.
func convertSQLite(ctx context.Context, tx *sql.Tx, wall *time.Location) error {
	for _, c := range wallColumns {
		rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT id, %s FROM %s", c.name, c.table))
		if err != nil {
			return fmt.Errorf("read %s.%s: %w", c.table, c.name, err)
		}
		type change struct {
			id  int
			utc string
		}
		var changes []change
		skipped := 0
		for rows.Next() {
			var (
				id  int
				raw chrono.Stored
			)
			if err := rows.Scan(&id, &raw); err != nil {
				// An unreadable value stays as it is rather than stopping
				// the upgrade.
				skipped++
				continue
			}
			if !raw.Valid {
				continue
			}
			t := raw.Time
			local := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, wall)
			changes = append(changes, change{id, local.UTC().Format("2006-01-02 15:04:05")})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read %s.%s: %w", c.table, c.name, err)
		}
		if skipped > 0 {
			logger.Log.WithFields(map[string]any{"table": c.table, "rows": skipped}).
				Warn("Left unreadable stored times unconverted")
		}
		stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", c.table, c.name))
		if err != nil {
			return fmt.Errorf("convert %s.%s: %w", c.table, c.name, err)
		}
		for _, ch := range changes {
			if _, err := stmt.ExecContext(ctx, ch.utc, ch.id); err != nil {
				stmt.Close()
				return fmt.Errorf("convert %s.%s row %d: %w", c.table, c.name, ch.id, err)
			}
		}
		stmt.Close()
	}
	return nil
}

func isUTC(loc *time.Location) bool {
	if loc == nil || loc == time.UTC {
		return true
	}
	// A zone that is UTC year-round ("Etc/UTC", "UTC", "GMT").
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	_, a := jan.Zone()
	_, b := jul.Zone()
	return a == 0 && b == 0
}
