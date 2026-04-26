package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"isley/logger"
	"isley/model"
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
		{"zones", &payload.Zones},
		{"breeder", &payload.Breeders},
		{"sensors", &payload.Sensors},
		{"rolling_averages", &payload.RollingAvgs},
		{"plant_status", &payload.PlantStatuses},
		{"strain", &payload.Strains},
		{"strain_lineage", &payload.StrainLineage},
		{"metric", &payload.Metrics},
		{"activity", &payload.Activities},
		{"plant", &payload.Plants},
		{"plant_status_log", &payload.PlantStatusLog},
		{"plant_measurements", &payload.PlantMeasure},
		{"plant_activity", &payload.PlantActivity},
		{"plant_images", &payload.PlantImages},
		{"streams", &payload.Streams},
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
			sensorRows, err = dumpTableFiltered(db, "sensor_data", opts.SensorDays)
		}
		if err != nil {
			return nil, BackupManifest{}, fmt.Errorf("dump sensor_data: %w", err)
		}
		payload.SensorData = sensorRows
		if len(sensorRows) > 0 {
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

// ApplyBackupToDB applies a parsed backup payload to db. It mirrors the
// truncate-then-insert sequence used by runRestore but without the
// async goroutine, the global RestoreInProgress flag, the progress
// reporting, and the upload-files extraction. Suitable for tests and
// for any synchronous restore path that may emerge later.
//
// The function uses model.IsPostgres()/IsSQLite() to pick the right
// dialect; tests that go through tests/testutil will see SQLite.
func ApplyBackupToDB(ctx context.Context, db *sql.DB, payload BackupPayload) error {
	if db == nil {
		return fmt.Errorf("ApplyBackupToDB: db is required")
	}

	// Tables in deletion order (children first) to satisfy FK constraints.
	truncateOrder := []string{
		"plant_activity",
		"plant_measurements",
		"plant_status_log",
		"plant_images",
		"streams",
		"strain_lineage",
		"plant",
		"sensor_data",
		"rolling_averages",
		"sensors",
		"strain",
		"activity",
		"metric",
		"plant_status",
		"breeder",
		"zones",
		"settings",
	}

	// Tables in insertion order (parents first).
	insertOrder := []struct {
		name string
		rows []map[string]interface{}
	}{
		{"settings", payload.Settings},
		{"zones", payload.Zones},
		{"breeder", payload.Breeders},
		{"plant_status", payload.PlantStatuses},
		{"metric", payload.Metrics},
		{"activity", payload.Activities},
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
		{"strain", payload.Strains},
		{"strain_lineage", payload.StrainLineage},
		{"plant", payload.Plants},
		{"plant_status_log", payload.PlantStatusLog},
		{"plant_measurements", payload.PlantMeasure},
		{"plant_activity", payload.PlantActivity},
		{"plant_images", payload.PlantImages},
		{"streams", payload.Streams},
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if model.IsPostgres() {
		if _, err := tx.Exec("SET CONSTRAINTS ALL DEFERRED"); err != nil {
			logger.Log.WithError(err).Warn("Could not defer constraints (non-fatal)")
		}
	} else {
		if _, err := tx.Exec("PRAGMA foreign_keys = OFF"); err != nil {
			logger.Log.WithError(err).Warn("Could not disable FK checks")
		}
	}

	for _, tbl := range truncateOrder {
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

	for _, tbl := range insertOrder {
		if len(tbl.rows) == 0 {
			continue
		}
		if err := insertRows(tx, tbl.name, tbl.rows); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert into %s: %w", tbl.name, err)
		}
	}

	if model.IsSQLite() {
		if _, err := tx.Exec("PRAGMA foreign_keys = ON"); err != nil {
			logger.Log.WithError(err).Warn("Could not re-enable FK checks")
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Reset Postgres sequences so subsequent inserts don't collide with
	// imported ids. Mirrors the production runRestore behavior.
	if model.IsPostgres() {
		seqTables := []string{
			"settings", "zones", "breeder", "sensors", "sensor_data",
			"strain", "strain_lineage", "plant_status", "plant",
			"plant_status_log", "metric", "plant_measurements",
			"activity", "plant_activity", "plant_images", "streams",
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
