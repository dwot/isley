package watcher

import (
	"database/sql"
	"fmt"
	"isley/logger"
	"isley/model"
)

// RefreshHourlyRollups aggregates raw sensor_data into the sensor_data_hourly
// rollup table. It processes only the last 25 hours of data on each run,
// using UPSERT to keep existing buckets current and add any new ones.
//
// On first run (empty rollup table) it backfills all available history.
func RefreshHourlyRollups() error {
	db, err := model.GetDB()
	if err != nil {
		return fmt.Errorf("rollup: failed to open database: %w", err)
	}

	// Check if the rollup table is empty (first run → full backfill)
	var rowCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sensor_data_hourly").Scan(&rowCount)
	if err != nil {
		return fmt.Errorf("rollup: failed to check rollup table: %w", err)
	}

	if rowCount == 0 {
		logger.Log.Info("Hourly rollup table is empty — running full backfill")
		return runRollup(db, true)
	}

	return runRollup(db, false)
}

// runRollup executes the actual aggregation query. When fullBackfill is false
// it only processes the last 25 hours (overlap by 1 hour to catch late data).
func runRollup(db *sql.DB, fullBackfill bool) error {
	var query string

	if model.IsPostgres() {
		query = buildPostgresRollupQuery(fullBackfill)
	} else {
		query = buildSQLiteRollupQuery(fullBackfill)
	}

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("rollup: aggregation query failed: %w", err)
	}

	logger.Log.Info("Hourly rollup refresh completed")
	return nil
}

func buildSQLiteRollupQuery(fullBackfill bool) string {
	whereClause := ""
	if !fullBackfill {
		whereClause = "WHERE sd.create_dt > datetime('now', '-25 hours')"
	}

	return fmt.Sprintf(`
		INSERT OR REPLACE INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		SELECT
			sd.sensor_id,
			strftime('%%Y-%%m-%%d %%H:00:00', sd.create_dt) AS bucket,
			MIN(sd.value),
			MAX(sd.value),
			AVG(sd.value),
			COUNT(*)
		FROM sensor_data sd
		%s
		GROUP BY sd.sensor_id, strftime('%%Y-%%m-%%d %%H:00:00', sd.create_dt)
	`, whereClause)
}

func buildPostgresRollupQuery(fullBackfill bool) string {
	whereClause := ""
	if !fullBackfill {
		whereClause = "WHERE sd.create_dt > NOW() - INTERVAL '25 hours'"
	}

	return fmt.Sprintf(`
		INSERT INTO sensor_data_hourly (sensor_id, bucket, min_val, max_val, avg_val, sample_count)
		SELECT
			sd.sensor_id,
			date_trunc('hour', sd.create_dt) AS bucket,
			MIN(sd.value),
			MAX(sd.value),
			AVG(sd.value),
			COUNT(*)
		FROM sensor_data sd
		%s
		GROUP BY sd.sensor_id, date_trunc('hour', sd.create_dt)
		ON CONFLICT (sensor_id, bucket) DO UPDATE SET
			min_val      = EXCLUDED.min_val,
			max_val      = EXCLUDED.max_val,
			avg_val      = EXCLUDED.avg_val,
			sample_count = EXCLUDED.sample_count
	`, whereClause)
}
