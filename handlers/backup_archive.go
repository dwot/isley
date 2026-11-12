package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"isley/logger"
	"isley/model"
	"isley/scope"
)

// BuildArchiveOptions tunes BuildBackupArchive. Production code sets
// these from runtime config; tests construct them by hand.
type BuildArchiveOptions struct {
	// IncludeImages walks UploadsDir and embeds every file under uploads/.
	IncludeImages bool

	// SensorDays controls sensor_data filtering:
	//   0  → include all rows (default)
	//   N  → include only the last N days
	//   -1 → skip the table entirely
	// The hourly rollup is included whole unless SensorDays is -1.
	SensorDays int

	// Version is baked into the manifest. Defaults to "unknown" if empty.
	Version string

	// UploadsDir is the directory walked when IncludeImages is true.
	// Defaults to "uploads" relative to the working directory.
	UploadsDir string

	// Now feeds the manifest CreatedAt timestamp. Zero values default
	// to time.Now() so production callers don't have to pass it.
	Now time.Time
}

// BuildBackupArchive dumps the database into a zip archive and returns
// the raw bytes plus the manifest written into it. No files are
// created or modified on disk — callers persist the archive themselves.
//
// This is the unit-testable core of runBackup: it has no goroutines, no
// status mutex, no backupsDir side effect, and it does not depend on
// the model.GetDB() global (db is passed in).
func BuildBackupArchive(db *sql.DB, opts BuildArchiveOptions) ([]byte, BackupManifest, error) {
	fieldLogger := logger.Log.WithField("func", "BuildBackupArchive")

	if db == nil {
		return nil, BackupManifest{}, fmt.Errorf("BuildBackupArchive: db is required")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	uploadsDir := opts.UploadsDir
	if uploadsDir == "" {
		uploadsDir = "uploads"
	}
	version := opts.Version
	if version == "" {
		version = "unknown"
	}

	payload := BackupPayload{}

	tableQueries := []struct {
		name string
		dest *[]map[string]interface{}
	}{
		{"settings", &payload.Settings},
		{"api_keys", &payload.APIKeys},
		{"zones", &payload.Zones},
		{"breeder", &payload.Breeders},
		{"sensors", &payload.Sensors},
		{"rolling_averages", &payload.RollingAvgs},
		{"plant_status", &payload.PlantStatuses},
		{"strain", &payload.Strains},
		{"strain_lineage", &payload.StrainLineage},
		{"metric", &payload.Metrics},
		{"activity", &payload.Activities},
		{"activity_metric", &payload.ActivityMetric},
		{"plant", &payload.Plants},
		{"plant_sensor", &payload.PlantSensors},
		{"plant_status_log", &payload.PlantStatusLog},
		{"plant_measurements", &payload.PlantMeasure},
		{"plant_activity", &payload.PlantActivity},
		{"plant_images", &payload.PlantImages},
		{"streams", &payload.Streams},
		{"smoke_report_criterion", &payload.SmokeCriteria},
		{"smoke_report", &payload.SmokeReports},
		{"smoke_report_score", &payload.SmokeScores},
	}

	tableCount := 0
	for _, tq := range tableQueries {
		rows, err := dumpTable(db, tq.name)
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump %s: %w", tq.name, err)
		}
		*tq.dest = rows
		if len(rows) > 0 {
			tableCount++
		}
	}

	if opts.SensorDays != -1 {
		var sensorRows []map[string]interface{}
		var err error
		if opts.SensorDays == 0 {
			sensorRows, err = dumpTable(db, "sensor_data")
		} else {
			sensorRows, err = dumpTableFiltered(db, "sensor_data", now.AddDate(0, 0, -opts.SensorDays))
		}
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump sensor_data: %w", err)
		}
		payload.SensorData = sensorRows
		if len(sensorRows) > 0 {
			tableCount++
		}
		// The whole hourly rollup, whatever SensorDays says: it is small
		// (one row per sensor-hour) and holds the history retention has
		// already pruned from sensor_data.
		hourly, err := dumpTable(db, "sensor_data_hourly")
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump sensor_data_hourly: %w", err)
		}
		payload.SensorDataHourly = hourly
		if len(hourly) > 0 {
			tableCount++
		}
	}

	fileCount := 0
	if opts.IncludeImages {
		_ = filepath.Walk(uploadsDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileCount++
			}
			return nil
		})
	}

	payload.Manifest = BackupManifest{
		Version:       version,
		Driver:        model.GetDriver(),
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Tables:        tableCount,
		Files:         fileCount,
		IncludeImages: opts.IncludeImages,
		SensorDays:    opts.SensorDays,
	}

	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, BackupManifest{}, fmt.Errorf("marshal JSON: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	jw, err := zw.Create("backup.json")
	if err != nil {
		return nil, BackupManifest{}, fmt.Errorf("create backup.json: %w", err)
	}
	if _, err := jw.Write(jsonData); err != nil {
		return nil, BackupManifest{}, fmt.Errorf("write backup.json: %w", err)
	}

	if opts.IncludeImages {
		err := filepath.Walk(uploadsDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return walkErr
			}
			fw, err := zw.Create(path)
			if err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(fw, f)
			return err
		})
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("add uploads: %w", err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, BackupManifest{}, fmt.Errorf("finalize zip: %w", err)
	}

	fieldLogger.Infof("Built backup archive: %d bytes, %d tables, %d files", buf.Len(), tableCount, fileCount)
	return buf.Bytes(), payload.Manifest, nil
}

// ParseBackupArchive reads a zip archive produced by BuildBackupArchive
// and returns the parsed BackupPayload. Returns a wrapped error if the
// bytes are not a valid zip, lack a backup.json entry, or contain
// malformed JSON.
func ParseBackupArchive(archive []byte) (BackupPayload, error) {
	if len(archive) == 0 {
		return BackupPayload{}, fmt.Errorf("ParseBackupArchive: archive is empty")
	}

	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return BackupPayload{}, fmt.Errorf("not a valid zip: %w", err)
	}

	for _, zf := range zr.File {
		if zf.Name != "backup.json" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return BackupPayload{}, fmt.Errorf("open backup.json: %w", err)
		}
		defer rc.Close()

		var payload BackupPayload
		dec := json.NewDecoder(rc)
		dec.UseNumber() // preserve int vs float fidelity
		if err := dec.Decode(&payload); err != nil {
			return BackupPayload{}, fmt.Errorf("decode backup.json: %w", err)
		}
		return payload, nil
	}
	return BackupPayload{}, fmt.Errorf("backup.json not found in archive")
}

// upgradeLegacyPayload fills in columns that archives from older Isley
// versions lack, where the column's default would give the wrong answer on
// restore. Both restore paths call it before inserting rows.
//
// api_keys.scopes (migration 024): an archive written before scoped keys has
// no scopes, and the column default (read) would quietly narrow every
// restored key. Those keys could do everything when the archive was made, so
// they come back as admin, the same as migration 024 does for live keys.
func upgradeLegacyPayload(p *BackupPayload) {
	for _, row := range p.APIKeys {
		if _, ok := row["scopes"]; !ok {
			row["scopes"] = scope.Full.String()
		}
	}
	upgradePlantSensorLinks(p)
}

// upgradePlantSensorLinks converts a backup made before migration 025,
// whose plant rows carry their linked sensors as a JSON array in
// "sensors", into plant_sensor rows, as the migration does: in order,
// without duplicates or links to sensors the backup doesn't have.
func upgradePlantSensorLinks(p *BackupPayload) {
	known := map[int64]bool{}
	for _, s := range p.Sensors {
		if id, ok := rowInt(s["id"]); ok {
			known[id] = true
		}
	}
	legacy := p.PlantSensors == nil
	for _, plant := range p.Plants {
		raw, had := plant["sensors"]
		delete(plant, "sensors")
		if !had || !legacy {
			continue
		}
		plantID, ok := rowInt(plant["id"])
		if !ok {
			continue
		}
		text, _ := raw.(string)
		var ids []int64
		if err := json.Unmarshal([]byte(text), &ids); err != nil {
			continue
		}
		seen := map[int64]bool{}
		for _, sid := range ids {
			if seen[sid] || !known[sid] {
				continue
			}
			seen[sid] = true
			p.PlantSensors = append(p.PlantSensors, map[string]interface{}{
				"plant_id": plantID, "sensor_id": sid, "position": len(seen) - 1,
			})
		}
	}
}

// rowInt reads an integer column from a decoded backup row (JSON numbers
// arrive as float64; rows built in Go may hold ints).
func rowInt(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

// ApplyBackupToDB applies a parsed backup payload to db. It mirrors the
// truncate-then-insert sequence used by runRestore but without the
// async goroutine, the global RestoreInProgress flag, the progress
// reporting, and the upload-files extraction. Suitable for tests and
// for any synchronous restore path that may emerge later.
//
// The function uses model.IsPostgres()/IsSQLite() to pick the right
// dialect; tests that go through tests/testutil will see SQLite.
// zoneVPDColumns point a zone at the sensors its VPD is worked out from,
// and sensors point at their zone. Neither table can go in first with its
// links, and Postgres checks foreign keys as each row goes in, so a
// restore inserts zones without these links and sets them once the
// sensors are in.
var zoneVPDColumns = []string{"vpd_temp_sensor_id", "vpd_humidity_sensor_id"}

// zoneVPDLink is one zone's VPD sensor links, to set after the sensors.
type zoneVPDLink struct {
	id        any
	temp, hum any
}

// splitZoneVPDLinks returns the zone rows with their VPD sensor links
// cleared, and the links to set later. Backups from before zone VPD have
// no such columns and pass through unchanged.
func splitZoneVPDLinks(zones []map[string]interface{}) ([]map[string]interface{}, []zoneVPDLink) {
	rows := make([]map[string]interface{}, len(zones))
	var links []zoneVPDLink
	for i, z := range zones {
		row := make(map[string]interface{}, len(z))
		for k, v := range z {
			row[k] = v
		}
		temp, hasTemp := z[zoneVPDColumns[0]]
		hum, hasHum := z[zoneVPDColumns[1]]
		if hasTemp || hasHum {
			if temp != nil || hum != nil {
				links = append(links, zoneVPDLink{id: z["id"], temp: temp, hum: hum})
			}
			if hasTemp {
				row[zoneVPDColumns[0]] = nil
			}
			if hasHum {
				row[zoneVPDColumns[1]] = nil
			}
		}
		rows[i] = row
	}
	return rows, links
}

// restoreZoneVPDLinks sets the links splitZoneVPDLinks held back.
func restoreZoneVPDLinks(tx *sql.Tx, links []zoneVPDLink) error {
	for _, l := range links {
		if _, err := tx.Exec("UPDATE zones SET vpd_temp_sensor_id = $1, vpd_humidity_sensor_id = $2 WHERE id = $3",
			l.temp, l.hum, l.id); err != nil {
			return fmt.Errorf("link zone %v to its VPD sensors: %w", l.id, err)
		}
	}
	return nil
}

// The update_rolling_avg trigger looks up a sensor's latest 16 readings for
// every row inserted into sensor_data. A restore loads millions of rows, and
// the lookups get slower as the table fills: on Postgres the planner still
// thinks the just-truncated table is empty (a prod-sized restore, 4.6M
// readings, was on course to take more than half a day); on SQLite the
// restore has dropped the sensor_data indexes. Bulk loads pause the trigger
// and work the averages out once at the end.
//
// pauseRollingAvgTrigger pauses it inside tx and returns the function that
// resumes it, which must run in the same transaction. Both dialects' DDL is
// transactional, so a failed load rolls the pause back with everything else.
func pauseRollingAvgTrigger(tx *sql.Tx) (resume func() error, err error) {
	if model.IsPostgres() {
		if _, err := tx.Exec("ALTER TABLE sensor_data DISABLE TRIGGER update_rolling_avg"); err != nil {
			return nil, fmt.Errorf("pause rolling average trigger: %w", err)
		}
		return func() error {
			return execAll(tx, "resume rolling average trigger",
				"ALTER TABLE sensor_data ENABLE TRIGGER update_rolling_avg",
				// So the planner sees the rows this transaction has just loaded.
				"ANALYZE sensor_data",
				recomputeRollingAvgPostgres)
		}, nil
	}
	// SQLite can't disable a trigger: drop it and put back the definition
	// the database has, whichever migration wrote it.
	var ddl string
	if err := tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'update_rolling_avg'").Scan(&ddl); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("pause rolling average trigger: %w", err)
	}
	if _, err := tx.Exec("DROP TRIGGER IF EXISTS update_rolling_avg"); err != nil {
		return nil, fmt.Errorf("pause rolling average trigger: %w", err)
	}
	return func() error {
		stmts := []string{recomputeRollingAvgSQLite}
		if ddl != "" {
			stmts = append([]string{ddl}, stmts...)
		}
		return execAll(tx, "resume rolling average trigger", stmts...)
	}, nil
}

// Each sensor's rolling average as the trigger leaves it: the mean of the 16
// readings before its newest, stamped with the newest. Postgres reads each
// sensor's latest readings off the index.
const recomputeRollingAvgPostgres = `
	INSERT INTO rolling_averages (sensor_id, avg_value, create_dt)
	SELECT l.sensor_id,
	       (SELECT AVG(value) FROM (
	            SELECT value FROM sensor_data
	            WHERE sensor_id = l.sensor_id
	            ORDER BY create_dt DESC
	            LIMIT 16 OFFSET 1) sub),
	       l.latest
	FROM (SELECT sensor_id, MAX(create_dt) AS latest FROM sensor_data GROUP BY sensor_id) l
	ON CONFLICT (sensor_id) DO UPDATE
	    SET avg_value = EXCLUDED.avg_value, create_dt = EXCLUDED.create_dt`

// SQLite's restore loads without the sensor_data indexes, so it ranks every
// reading in one pass instead.
const recomputeRollingAvgSQLite = `
	INSERT OR REPLACE INTO rolling_averages (sensor_id, avg_value, create_dt)
	SELECT l.sensor_id, w.avg_value, l.latest
	FROM (SELECT sensor_id, MAX(create_dt) AS latest FROM sensor_data GROUP BY sensor_id) l
	LEFT JOIN (
	    SELECT sensor_id, AVG(value) AS avg_value
	    FROM (SELECT sensor_id, value,
	                 ROW_NUMBER() OVER (PARTITION BY sensor_id ORDER BY create_dt DESC) AS rn
	          FROM sensor_data)
	    WHERE rn BETWEEN 2 AND 17
	    GROUP BY sensor_id) w ON w.sensor_id = l.sensor_id`

// sqliteForeignKeysOff turns SQLite's foreign key checks off on conn and
// returns the function that puts back the connection's previous setting
// (the app's own connections run with them off; the tests' with them on).
// conn must not be inside a transaction: SQLite ignores the pragma there.
func sqliteForeignKeysOff(ctx context.Context, conn *sql.Conn) (restore func(), err error) {
	var prev int
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&prev); err != nil {
		return nil, fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return nil, fmt.Errorf("disable foreign keys: %w", err)
	}
	return func() {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA foreign_keys = %d", prev)); err != nil {
			logger.Log.WithError(err).Warn("Could not restore the foreign_keys pragma")
		}
	}, nil
}

// execAll runs each statement in tx, stopping at the first error.
func execAll(tx *sql.Tx, what string, stmts ...string) error {
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	return nil
}

func ApplyBackupToDB(ctx context.Context, db *sql.DB, payload BackupPayload) error {
	if db == nil {
		return fmt.Errorf("ApplyBackupToDB: db is required")
	}
	upgradeLegacyPayload(&payload)
	zones, zoneLinks := splitZoneVPDLinks(payload.Zones)
	payload.Zones = zones

	// Tables in deletion order (children first) to satisfy FK constraints.
	truncateOrder := []string{
		"smoke_report_score",
		"smoke_report",
		"smoke_report_criterion",
		"plant_activity",
		"plant_measurements",
		"plant_status_log",
		"plant_images",
		"streams",
		"strain_lineage",
		"plant_sensor",
		"plant",
		"sensor_data_hourly",
		"sensor_data",
		"rolling_averages",
		"sensors",
		"strain",
		"activity_metric",
		"activity",
		"metric",
		"plant_status",
		"breeder",
		"zones",
		"api_keys",
		"settings",
	}

	// Tables in insertion order (parents first).
	insertOrder := []struct {
		name string
		rows []map[string]interface{}
	}{
		{"settings", payload.Settings},
		{"api_keys", payload.APIKeys},
		{"zones", payload.Zones},
		{"breeder", payload.Breeders},
		{"plant_status", payload.PlantStatuses},
		{"metric", payload.Metrics},
		{"activity", payload.Activities},
		{"activity_metric", payload.ActivityMetric},
		{"sensors", payload.Sensors},
		// rolling_averages BEFORE sensor_data: there's an AFTER INSERT
		// trigger on sensor_data that does INSERT OR REPLACE INTO
		// rolling_averages, so it would clobber a regular INSERT into
		// rolling_averages performed afterward (and worse, in a single
		// transaction the trigger pre-populates the rows and the
		// payload INSERT hits a UNIQUE conflict). Production splits
		// sensor_data into a second transaction which has the same
		// effective ordering.
		{"rolling_averages", payload.RollingAvgs},
		{"sensor_data", payload.SensorData},
		{"sensor_data_hourly", payload.SensorDataHourly},
		{"strain", payload.Strains},
		{"strain_lineage", payload.StrainLineage},
		{"plant", payload.Plants},
		{"plant_sensor", payload.PlantSensors},
		{"plant_status_log", payload.PlantStatusLog},
		{"plant_measurements", payload.PlantMeasure},
		{"plant_activity", payload.PlantActivity},
		{"plant_images", payload.PlantImages},
		{"streams", payload.Streams},
		{"smoke_report_criterion", payload.SmokeCriteria},
		{"smoke_report", payload.SmokeReports},
		{"smoke_report_score", payload.SmokeScores},
	}

	// SQLite: foreign key checks off for the whole load, on one pinned
	// connection, since SQLite ignores the pragma inside a transaction.
	var begin interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	} = db
	if model.IsSQLite() {
		conn, err := db.Conn(ctx)
		if err != nil {
			return fmt.Errorf("pin connection: %w", err)
		}
		defer conn.Close()
		restoreFKs, err := sqliteForeignKeysOff(ctx, conn)
		if err != nil {
			return err
		}
		defer restoreFKs()
		begin = conn
	}

	tx, err := begin.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if model.IsPostgres() {
		if _, err := tx.Exec("SET CONSTRAINTS ALL DEFERRED"); err != nil {
			logger.Log.WithError(err).Warn("Could not defer constraints (non-fatal)")
		}
	}

	for _, tbl := range tablesToTruncate(truncateOrder, insertOrder) {
		var stmt string
		if model.IsPostgres() {
			stmt = fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tbl)
		} else {
			stmt = fmt.Sprintf("DELETE FROM %s", tbl)
		}
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("truncate %s: %w", tbl, err)
		}
	}

	resumeRollingAvg, err := pauseRollingAvgTrigger(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, tbl := range insertOrder {
		if len(tbl.rows) == 0 {
			continue
		}
		if err := insertRows(tx, tbl.name, tbl.rows); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert into %s: %w", tbl.name, err)
		}
	}
	if err := resumeRollingAvg(); err != nil {
		tx.Rollback()
		return err
	}
	if err := restoreZoneVPDLinks(tx, zoneLinks); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Reset Postgres sequences so subsequent inserts don't collide with
	// imported ids. Mirrors the production runRestore behavior.
	if model.IsPostgres() {
		seqTables := []string{
			"settings", "api_keys", "zones", "breeder", "sensors", "sensor_data",
			"strain", "strain_lineage", "plant_status", "plant",
			"plant_status_log", "metric", "plant_measurements",
			"activity", "activity_metric", "plant_activity", "plant_images", "streams",
			"smoke_report_criterion", "smoke_report", "smoke_report_score",
		}
		for _, tbl := range seqTables {
			q := fmt.Sprintf(
				"SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1)) FROM %s",
				tbl, tbl,
			)
			if _, err := db.Exec(q); err != nil {
				logger.Log.WithError(err).Debugf("Could not reset sequence for %s", tbl)
			}
		}
	}
	return nil
}
