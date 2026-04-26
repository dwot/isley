package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"isley/config"
	"isley/logger"
	"isley/model"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// BackupManifest contains metadata about the backup archive.
type BackupManifest struct {
	Version       string `json:"version"`
	Driver        string `json:"driver"`
	CreatedAt     string `json:"created_at"`
	Tables        int    `json:"tables"`
	Files         int    `json:"files"`
	IncludeImages bool   `json:"include_images"`
	SensorDays    int    `json:"sensor_days"` // 0 = all history
}

// BackupPayload is the top-level JSON structure written to backup.json
// inside the archive. Each key is a table name mapped to a slice of
// row-maps so the format is driver-agnostic.
type BackupPayload struct {
	Manifest       BackupManifest           `json:"manifest"`
	Settings       []map[string]interface{} `json:"settings"`
	Zones          []map[string]interface{} `json:"zones"`
	Breeders       []map[string]interface{} `json:"breeder"`
	Sensors        []map[string]interface{} `json:"sensors"`
	SensorData     []map[string]interface{} `json:"sensor_data"`
	RollingAvgs    []map[string]interface{} `json:"rolling_averages"`
	Strains        []map[string]interface{} `json:"strain"`
	StrainLineage  []map[string]interface{} `json:"strain_lineage"`
	PlantStatuses  []map[string]interface{} `json:"plant_status"`
	Plants         []map[string]interface{} `json:"plant"`
	PlantStatusLog []map[string]interface{} `json:"plant_status_log"`
	Metrics        []map[string]interface{} `json:"metric"`
	PlantMeasure   []map[string]interface{} `json:"plant_measurements"`
	Activities     []map[string]interface{} `json:"activity"`
	PlantActivity  []map[string]interface{} `json:"plant_activity"`
	PlantImages    []map[string]interface{} `json:"plant_images"`
	Streams        []map[string]interface{} `json:"streams"`
}

// BackupFileInfo is returned by the list endpoint.
type BackupFileInfo struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SizeMB    string `json:"size_mb"`
	CreatedAt string `json:"created_at"`
}

// BackupStatus tracks the state of an in-progress async backup.
type BackupStatus struct {
	InProgress bool   `json:"in_progress"`
	Filename   string `json:"filename,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RestoreStatus tracks the state of an in-progress async restore.
type RestoreStatus struct {
	InProgress   bool   `json:"in_progress"`
	Phase        string `json:"phase,omitempty"`         // "uploading", "truncating", "restoring", "sequences", "extracting", "complete"
	CurrentTable string `json:"current_table,omitempty"` // table being restored
	BatchNum     int    `json:"batch_num,omitempty"`     // current batch within large table
	TotalBatches int    `json:"total_batches,omitempty"` // total batches for current table
	TablesLeft   int    `json:"tables_left,omitempty"`   // remaining tables to restore
	TotalTables  int    `json:"total_tables,omitempty"`  // total tables with data
	Error        string `json:"error,omitempty"`
	Tables       int    `json:"tables,omitempty"` // final count on completion
	Files        int    `json:"files,omitempty"`  // final count on completion
}

// insertBatchSize controls how many rows are packed into each multi-row
// INSERT during chunked restore of large tables (sensor_data).
const insertBatchSize = 5000

// backupsDir is the directory where backup archives are stored.
const backupsDir = "data/backups"

// backupMu protects concurrent backup creation.
var backupMu sync.Mutex

// currentBackup tracks the status of any in-flight async backup.
var currentBackup = BackupStatus{}

// restoreMu protects concurrent restore status access.
var restoreMu sync.Mutex

// currentRestore tracks the status of any in-flight async restore.
var currentRestore = RestoreStatus{}

// ---------------------------------------------------------------------------
// Export (async — saves to data/backups/)
// ---------------------------------------------------------------------------

// ResetBackupStatusForTesting clears the in-memory backup-status singleton.
// Lets tests start from a clean slate without depending on goroutine timing.
// Production code does not call this; the helper exists for handler tests
// that share this package's globals (Phase 7 of docs/TEST_PLAN.md will
// migrate these to instance-scoped state).
func ResetBackupStatusForTesting() {
	backupMu.Lock()
	currentBackup = BackupStatus{}
	backupMu.Unlock()
}

// SetBackupInProgressForTesting marks a backup as in-flight so the next
// CreateBackup call deterministically returns 409 without racing a real
// goroutine. Pair with ResetBackupStatusForTesting in t.Cleanup.
func SetBackupInProgressForTesting() {
	backupMu.Lock()
	currentBackup = BackupStatus{InProgress: true}
	backupMu.Unlock()
}

// ResetRestoreStatusForTesting / SetRestoreInProgressForTesting are the
// restore-side equivalents of the helpers above.
func ResetRestoreStatusForTesting() {
	restoreMu.Lock()
	currentRestore = RestoreStatus{}
	restoreMu.Unlock()
}

func SetRestoreInProgressForTesting() {
	restoreMu.Lock()
	currentRestore = RestoreStatus{InProgress: true}
	restoreMu.Unlock()
}

// CreateBackup kicks off an async backup job and returns immediately.
// Query params:
//
//	?images=true|false  — include uploaded images (default false)
//	?sensor_days=N      — include only last N days of sensor_data (0 = all, -1 = none)
func CreateBackup(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "CreateBackup")

	backupMu.Lock()
	if currentBackup.InProgress {
		backupMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": T(c, "api_backup_in_progress")})
		return
	}
	currentBackup = BackupStatus{InProgress: true}
	backupMu.Unlock()

	includeImages := c.DefaultQuery("images", "false") == "true"
	sensorDays := 0 // default: all
	if sd := c.Query("sensor_days"); sd != "" {
		fmt.Sscanf(sd, "%d", &sensorDays)
	}

	fieldLogger.Infof("Starting async backup: images=%v sensor_days=%d", includeImages, sensorDays)

	go func() {
		defer func() {
			backupMu.Lock()
			currentBackup.InProgress = false
			backupMu.Unlock()
		}()

		if err := runBackup(includeImages, sensorDays); err != nil {
			backupMu.Lock()
			currentBackup.Error = err.Error()
			backupMu.Unlock()
			fieldLogger.WithError(err).Error("Async backup failed")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": T(c, "api_backup_started")})
}

// GetBackupStatus returns the state of any in-progress backup.
func GetBackupStatus(c *gin.Context) {
	backupMu.Lock()
	defer backupMu.Unlock()
	c.JSON(http.StatusOK, currentBackup)
}

// GetRestoreStatus returns the state of any in-progress restore.
func GetRestoreStatus(c *gin.Context) {
	restoreMu.Lock()
	defer restoreMu.Unlock()
	c.JSON(http.StatusOK, currentRestore)
}

// runBackup does the actual work of dumping the DB and writing the zip.
// The archive contents are produced by BuildBackupArchive (which is
// unit-tested directly); runBackup adds the production-only concerns:
// resolving the DB, reading the VERSION file, naming the output, and
// writing it under data/backups/.
func runBackup(includeImages bool, sensorDays int) error {
	fieldLogger := logger.Log.WithField("handler", "runBackup")

	db, err := model.GetDB()
	if err != nil {
		return fmt.Errorf("get DB: %w", err)
	}

	version := "unknown"
	if v, err := os.ReadFile("VERSION"); err == nil {
		version = strings.TrimSpace(string(v))
	}

	archive, manifest, err := BuildBackupArchive(db, BuildArchiveOptions{
		IncludeImages: includeImages,
		SensorDays:    sensorDays,
		Version:       version,
		UploadsDir:    "uploads",
	})
	if err != nil {
		return err
	}

	if err := os.MkdirAll(backupsDir, os.ModePerm); err != nil {
		return fmt.Errorf("create backups dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	tag := "db"
	if includeImages {
		tag = "full"
	}
	if sensorDays == -1 {
		tag += "-nosensor"
	} else if sensorDays > 0 {
		tag += fmt.Sprintf("-%dd", sensorDays)
	}
	filename := fmt.Sprintf("isley-backup-%s-%s.zip", tag, ts)
	destPath := filepath.Join(backupsDir, filename)

	if err := os.WriteFile(destPath, archive, 0644); err != nil {
		return fmt.Errorf("write %s: %w", destPath, err)
	}

	backupMu.Lock()
	currentBackup.Filename = filename
	backupMu.Unlock()

	fieldLogger.Infof("Backup saved: %s (%d bytes, %d tables, %d files)",
		filename, len(archive), manifest.Tables, manifest.Files)
	return nil
}

// dumpTableFiltered dumps sensor_data rows from the last N days only.
func dumpTableFiltered(db *sql.DB, table string, days int) ([]map[string]interface{}, error) {
	var query string
	if model.IsPostgres() {
		query = fmt.Sprintf("SELECT * FROM %s WHERE create_dt >= NOW() - INTERVAL '%d days'", table, days)
	} else {
		query = fmt.Sprintf("SELECT * FROM %s WHERE create_dt >= datetime('now', 'localtime', '-%d days')", table, days)
	}

	rows, err := db.Query(query) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = normaliseValue(vals[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Backup management (list, download, delete)
// ---------------------------------------------------------------------------

// ListBackups returns a JSON array of available backup files.
func ListBackups(c *gin.Context) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, []BackupFileInfo{})
			return
		}
		apiInternalError(c, "api_database_error")
		return
	}

	backups := make([]BackupFileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupFileInfo{
			Name:      e.Name(),
			Size:      info.Size(),
			SizeMB:    fmt.Sprintf("%.1f", float64(info.Size())/1024/1024),
			CreatedAt: info.ModTime().Format(time.RFC3339),
		})
	}

	// Most recent first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})

	c.JSON(http.StatusOK, backups)
}

// DownloadBackup serves a specific backup file for download.
func DownloadBackup(c *gin.Context) {
	name := c.Param("name")
	// Sanitize: no path traversal
	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".zip") {
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}

	path := filepath.Join(backupsDir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		apiNotFound(c, "api_invalid_backup_file")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.File(path)
}

// DeleteBackup removes a specific backup file.
func DeleteBackup(c *gin.Context) {
	name := c.Param("name")
	name = filepath.Base(name)
	if !strings.HasSuffix(name, ".zip") {
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}

	path := filepath.Join(backupsDir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		apiNotFound(c, "api_invalid_backup_file")
		return
	}

	if err := os.Remove(path); err != nil {
		logger.Log.WithError(err).Errorf("Failed to delete backup %s", name)
		apiInternalError(c, "api_database_error")
		return
	}

	apiOK(c, "api_backup_deleted")
}

// ---------------------------------------------------------------------------
// SQLite file download / upload
// ---------------------------------------------------------------------------

// DownloadSQLiteDB serves the raw SQLite database file for download.
// Only available when the active driver is SQLite.
func DownloadSQLiteDB(c *gin.Context) {
	if !model.IsSQLite() {
		c.JSON(http.StatusBadRequest, gin.H{"error": T(c, "api_sqlite_download_only")})
		return
	}

	dbFile := os.Getenv("ISLEY_DB_FILE")
	if dbFile == "" {
		dbFile = "data/isley.db"
	}

	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		apiNotFound(c, "api_invalid_backup_file")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="isley-%s.db"`, time.Now().Format("20060102-150405")))
	c.File(dbFile)
}

// UploadSQLiteDB accepts a raw .db file upload, pauses watchers, replaces
// the current SQLite database file, and triggers a reload. This is the
// fastest way to clone a SQLite instance since it avoids row-by-row import.
func UploadSQLiteDB(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "UploadSQLiteDB")

	if !model.IsSQLite() {
		c.JSON(http.StatusBadRequest, gin.H{"error": T(c, "api_sqlite_upload_only")})
		return
	}

	restoreMu.Lock()
	if currentRestore.InProgress {
		restoreMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": T(c, "api_restore_in_progress")})
		return
	}
	currentRestore = RestoreStatus{InProgress: true, Phase: "uploading"}
	restoreMu.Unlock()

	file, header, err := c.Request.FormFile("database")
	if err != nil {
		fieldLogger.WithError(err).Error("No database file in request")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}
	defer file.Close()
	fieldLogger.Infof("Received SQLite file: %s (%d bytes)", header.Filename, header.Size)

	if header.Size > config.MaxBackupSize {
		fieldLogger.Errorf("SQLite file too large: %d bytes (max %d)", header.Size, config.MaxBackupSize)
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_backup_file_too_large")
		return
	}

	body, err := io.ReadAll(file)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read uploaded file")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiInternalError(c, "api_database_error")
		return
	}

	// Basic SQLite file validation — check the magic header bytes
	if len(body) < 16 || string(body[:16]) != "SQLite format 3\x00" {
		fieldLogger.Error("Uploaded file is not a valid SQLite database")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}

	go runSQLiteFileRestore(body)

	c.JSON(http.StatusAccepted, gin.H{"message": T(c, "api_restore_started")})
}

// runSQLiteFileRestore replaces the current SQLite file with the uploaded one.
func runSQLiteFileRestore(data []byte) {
	fieldLogger := logger.Log.WithField("handler", "runSQLiteFileRestore")

	defer func() {
		restoreMu.Lock()
		currentRestore.InProgress = false
		restoreMu.Unlock()
	}()

	config.RestoreInProgress.Store(true)
	defer config.RestoreInProgress.Store(false)
	fieldLogger.Info("Paused background watchers for SQLite file restore")

	updateRestoreStatus("restoring", "database file", 0, 0, 0, 0)

	dbFile := os.Getenv("ISLEY_DB_FILE")
	if dbFile == "" {
		dbFile = "data/isley.db"
	}

	// Close the current database connection so we can replace the file
	if err := model.CloseDB(); err != nil {
		fieldLogger.WithError(err).Error("Failed to close current database")
		restoreMu.Lock()
		currentRestore.Error = "Failed to close current database"
		restoreMu.Unlock()
		return
	}

	// Remove WAL and SHM files if they exist (stale after file swap)
	os.Remove(dbFile + "-wal")
	os.Remove(dbFile + "-shm")

	// Write the new database file
	if err := os.WriteFile(dbFile, data, 0644); err != nil {
		fieldLogger.WithError(err).Error("Failed to write new database file")
		restoreMu.Lock()
		currentRestore.Error = "Failed to write database file"
		restoreMu.Unlock()
		// Try to reopen the old DB (may fail if we partially wrote)
		model.InitDB()
		return
	}

	// Reopen the database
	model.InitDB()
	fieldLogger.Info("SQLite file replaced and database reopened")

	// Reload in-memory config from the newly reopened DB.
	if reopenedDB, dbErr := model.GetDB(); dbErr == nil {
		LoadSettings(reopenedDB)
	} else {
		fieldLogger.WithError(dbErr).Error("Failed to obtain DB after reopen for LoadSettings")
	}

	restoreMu.Lock()
	currentRestore.Phase = "complete"
	currentRestore.Tables = 0
	currentRestore.Files = 0
	restoreMu.Unlock()

	fieldLogger.Info("SQLite file restore complete")
}

// ---------------------------------------------------------------------------
// Import
// ---------------------------------------------------------------------------

// largeTables lists tables that may contain hundreds of thousands of rows.
// These are inserted in batches (separate transactions) rather than in one
// giant transaction, so the SQLite write lock is released periodically.
var largeTables = map[string]bool{
	"sensor_data": true,
}

// ImportBackup accepts a .zip archive (produced by CreateBackup), kicks off
// an async restore goroutine, and returns 202 immediately. The UI polls
// /settings/backup/restore/status for progress updates.
func ImportBackup(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ImportBackup")

	restoreMu.Lock()
	if currentRestore.InProgress {
		restoreMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": T(c, "api_restore_in_progress")})
		return
	}
	currentRestore = RestoreStatus{InProgress: true, Phase: "uploading"}
	restoreMu.Unlock()

	// ---- read the uploaded zip into memory --------------------------------
	file, header, err := c.Request.FormFile("backup")
	if err != nil {
		fieldLogger.WithError(err).Error("No backup file in request")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}
	defer file.Close()
	fieldLogger.Infof("Received backup file: %s (%d bytes)", header.Filename, header.Size)

	if header.Size > config.MaxBackupSize {
		fieldLogger.Errorf("Backup file too large: %d bytes (max %d)", header.Size, config.MaxBackupSize)
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_backup_file_too_large")
		return
	}

	body, err := io.ReadAll(file)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read uploaded file")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiInternalError(c, "api_database_error")
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		fieldLogger.WithError(err).Error("Uploaded file is not a valid zip")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}

	// ---- locate and parse backup.json ------------------------------------
	var payload BackupPayload
	foundJSON := false
	for _, zf := range zr.File {
		if zf.Name == "backup.json" {
			rc, err := zf.Open()
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to open backup.json in zip")
				restoreMu.Lock()
				currentRestore = RestoreStatus{}
				restoreMu.Unlock()
				apiInternalError(c, "api_database_error")
				return
			}
			dec := json.NewDecoder(rc)
			dec.UseNumber() // preserve int vs float fidelity
			if err := dec.Decode(&payload); err != nil {
				rc.Close()
				fieldLogger.WithError(err).Error("Failed to decode backup.json")
				restoreMu.Lock()
				currentRestore = RestoreStatus{}
				restoreMu.Unlock()
				apiBadRequest(c, "api_invalid_backup_file")
				return
			}
			rc.Close()
			foundJSON = true
			break
		}
	}
	if !foundJSON {
		fieldLogger.Error("backup.json not found in archive")
		restoreMu.Lock()
		currentRestore = RestoreStatus{}
		restoreMu.Unlock()
		apiBadRequest(c, "api_invalid_backup_file")
		return
	}

	fieldLogger.Infof("Backup manifest: version=%s driver=%s created=%s tables=%d files=%d images=%v sensor_days=%d",
		payload.Manifest.Version, payload.Manifest.Driver,
		payload.Manifest.CreatedAt, payload.Manifest.Tables, payload.Manifest.Files,
		payload.Manifest.IncludeImages, payload.Manifest.SensorDays)

	// Check if the user wants to skip sensor data (useful for SQLite where
	// large sensor_data imports are extremely slow).
	skipSensor := c.DefaultPostForm("skip_sensor_data", "false") == "true"
	if skipSensor {
		fieldLogger.Info("User opted to skip sensor_data import")
		payload.SensorData = nil
	}

	// Launch the restore in a background goroutine and return 202
	go runRestore(payload, body)

	c.JSON(http.StatusAccepted, gin.H{"message": T(c, "api_restore_started")})
}

// updateRestoreStatus safely updates the current restore status fields.
func updateRestoreStatus(phase, table string, batchNum, totalBatches, tablesLeft, totalTables int) {
	restoreMu.Lock()
	currentRestore.Phase = phase
	currentRestore.CurrentTable = table
	currentRestore.BatchNum = batchNum
	currentRestore.TotalBatches = totalBatches
	currentRestore.TablesLeft = tablesLeft
	currentRestore.TotalTables = totalTables
	restoreMu.Unlock()
}

// runRestore performs the actual database restore work in a background goroutine.
func runRestore(payload BackupPayload, zipBody []byte) {
	fieldLogger := logger.Log.WithField("handler", "runRestore")

	defer func() {
		restoreMu.Lock()
		currentRestore.InProgress = false
		restoreMu.Unlock()
	}()

	// ---- pause background watchers ---------------------------------------
	config.RestoreInProgress.Store(true)
	defer config.RestoreInProgress.Store(false)
	fieldLogger.Info("Paused background watchers for restore")

	// ---- restore database -------------------------------------------------
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to get DB handle")
		restoreMu.Lock()
		currentRestore.Error = "Failed to get database handle"
		restoreMu.Unlock()
		return
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
		{"sensor_data", payload.SensorData},
		{"rolling_averages", payload.RollingAvgs},
		{"strain", payload.Strains},
		{"strain_lineage", payload.StrainLineage},
		{"plant", payload.Plants},
		{"plant_status_log", payload.PlantStatusLog},
		{"plant_measurements", payload.PlantMeasure},
		{"plant_activity", payload.PlantActivity},
		{"plant_images", payload.PlantImages},
		{"streams", payload.Streams},
	}

	// Count tables with data for progress tracking
	totalTablesWithData := 0
	for _, tbl := range insertOrder {
		if len(tbl.rows) > 0 {
			totalTablesWithData++
		}
	}

	// For SQLite we pin a single database connection so that PRAGMA
	// settings (synchronous, journal_mode, foreign_keys) are guaranteed
	// to apply to the same connection that performs every INSERT.
	// Go's database/sql connection pool can otherwise hand out different
	// connections for db.Exec vs db.Begin, silently discarding PRAGMAs.
	ctx := context.Background()

	// execer wraps either a pinned *sql.Conn (SQLite) or the pool *sql.DB
	// so the rest of the function doesn't branch on every call.
	type execer interface {
		ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
		BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	}

	var exec execer
	if model.IsSQLite() {
		conn, err := db.Conn(ctx)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to pin SQLite connection")
			restoreMu.Lock()
			currentRestore.Error = "Failed to pin database connection"
			restoreMu.Unlock()
			return
		}
		defer conn.Close()
		exec = conn

		// PRAGMAs on the pinned connection — guaranteed to affect every
		// subsequent operation on this same conn.
		fieldLogger.Info("Applying SQLite PRAGMA optimizations for bulk import (pinned connection)")
		if _, err := conn.ExecContext(ctx, "PRAGMA synchronous = OFF"); err != nil {
			fieldLogger.WithError(err).Warn("Could not set PRAGMA synchronous=OFF")
		}
		if _, err := conn.ExecContext(ctx, "PRAGMA journal_mode = MEMORY"); err != nil {
			fieldLogger.WithError(err).Warn("Could not set PRAGMA journal_mode=MEMORY")
		}
		defer func() {
			fieldLogger.Info("Restoring SQLite PRAGMAs to safe defaults")
			conn.ExecContext(ctx, "PRAGMA synchronous = FULL")
			conn.ExecContext(ctx, "PRAGMA journal_mode = WAL")
		}()
	} else {
		exec = db
	}

	// Phase 1: truncate all tables + insert reference data in one txn.
	updateRestoreStatus("truncating", "", 0, 0, totalTablesWithData, totalTablesWithData)

	tx, err := exec.BeginTx(ctx, nil)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		restoreMu.Lock()
		currentRestore.Error = "Failed to begin transaction"
		restoreMu.Unlock()
		return
	}

	if model.IsPostgres() {
		if _, err := tx.Exec("SET CONSTRAINTS ALL DEFERRED"); err != nil {
			fieldLogger.WithError(err).Warn("Could not defer constraints (non-fatal)")
		}
	} else {
		if _, err := tx.Exec("PRAGMA foreign_keys = OFF"); err != nil {
			fieldLogger.WithError(err).Warn("Could not disable FK checks")
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
			fieldLogger.WithError(err).Errorf("Failed to truncate %s", tbl)
			restoreMu.Lock()
			currentRestore.Error = fmt.Sprintf("Failed to truncate %s", tbl)
			restoreMu.Unlock()
			return
		}
	}

	// Insert small/reference tables in the same transaction
	tablesRestored := 0
	for _, tbl := range insertOrder {
		if len(tbl.rows) == 0 || largeTables[tbl.name] {
			continue
		}
		tablesRestored++
		remaining := totalTablesWithData - tablesRestored
		updateRestoreStatus("restoring", tbl.name, 0, 0, remaining, totalTablesWithData)

		if err := insertRows(tx, tbl.name, tbl.rows); err != nil {
			tx.Rollback()
			fieldLogger.WithError(err).Errorf("Failed to insert into %s", tbl.name)
			restoreMu.Lock()
			currentRestore.Error = fmt.Sprintf("Failed to insert into %s", tbl.name)
			restoreMu.Unlock()
			return
		}
		fieldLogger.Infof("Restored %d rows into %s", len(tbl.rows), tbl.name)
	}

	if model.IsSQLite() {
		if _, err := tx.Exec("PRAGMA foreign_keys = ON"); err != nil {
			fieldLogger.WithError(err).Warn("Could not re-enable FK checks")
		}
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit reference data transaction")
		restoreMu.Lock()
		currentRestore.Error = "Failed to commit reference data"
		restoreMu.Unlock()
		return
	}
	fieldLogger.Info("Reference data restored, starting bulk data import")

	// Phase 2: insert large tables in batched multi-row INSERTs.
	// For SQLite, drop indexes first to avoid O(n log n) index maintenance
	// on every insert — they'll be recreated after the bulk load.
	sensorDataIndexes := []struct {
		name       string
		createStmt string
	}{
		{"idx_sensor_data_sensor_id", "CREATE INDEX IF NOT EXISTS idx_sensor_data_sensor_id ON sensor_data(sensor_id)"},
		{"idx_sensor_data_create_dt", "CREATE INDEX IF NOT EXISTS idx_sensor_data_create_dt ON sensor_data(create_dt)"},
	}

	if model.IsSQLite() {
		for _, idx := range sensorDataIndexes {
			if _, err := exec.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", idx.name)); err != nil {
				fieldLogger.WithError(err).Warnf("Could not drop index %s", idx.name)
			} else {
				fieldLogger.Infof("Dropped index %s for bulk import", idx.name)
			}
		}
	}

	for _, tbl := range insertOrder {
		if len(tbl.rows) == 0 || !largeTables[tbl.name] {
			continue
		}
		tablesRestored++
		remaining := totalTablesWithData - tablesRestored

		totalBatches := (len(tbl.rows) + insertBatchSize - 1) / insertBatchSize
		updateRestoreStatus("restoring", tbl.name, 0, totalBatches, remaining, totalTablesWithData)

		if err := insertRowsBatchedWithProgress(ctx, exec, tbl.name, tbl.rows, insertBatchSize, remaining, totalTablesWithData); err != nil {
			fieldLogger.WithError(err).Errorf("Failed to bulk insert into %s", tbl.name)
			restoreMu.Lock()
			currentRestore.Error = fmt.Sprintf("Failed to bulk insert into %s", tbl.name)
			restoreMu.Unlock()
			return
		}
		fieldLogger.Infof("Restored %d rows into %s (batched)", len(tbl.rows), tbl.name)
	}

	// Recreate indexes after bulk load (SQLite only — Postgres TRUNCATE
	// doesn't drop indexes so they're still there).
	if model.IsSQLite() {
		fieldLogger.Info("Recreating sensor_data indexes after bulk import")
		for _, idx := range sensorDataIndexes {
			if _, err := exec.ExecContext(ctx, idx.createStmt); err != nil {
				fieldLogger.WithError(err).Warnf("Could not recreate index %s", idx.name)
			} else {
				fieldLogger.Infof("Recreated index %s", idx.name)
			}
		}
	}

	// Phase 3: reset Postgres sequences.
	if model.IsPostgres() {
		updateRestoreStatus("sequences", "", 0, 0, 0, totalTablesWithData)
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
				fieldLogger.WithError(err).Debugf("Could not reset sequence for %s", tbl)
			}
		}
	}

	// ---- extract upload files ---------------------------------------------
	updateRestoreStatus("extracting", "", 0, 0, 0, totalTablesWithData)

	uploadsDir := "uploads"
	filesRestored := 0

	// Re-open the zip from the in-memory body (the original zr may be stale
	// since we're in a different goroutine context, but we kept zipBody).
	zr2, err := zip.NewReader(bytes.NewReader(zipBody), int64(len(zipBody)))
	if err != nil {
		fieldLogger.WithError(err).Warn("Could not re-open zip for file extraction")
	} else {
		hasUploads := false
		for _, zf := range zr2.File {
			if strings.HasPrefix(zf.Name, "uploads/") && !strings.HasSuffix(zf.Name, "/") {
				hasUploads = true
				break
			}
		}

		if hasUploads {
			if err := os.RemoveAll(uploadsDir); err != nil {
				fieldLogger.WithError(err).Warn("Could not clean existing uploads dir")
			}

			var extractedBytes int64
			extractLimit := config.MaxBackupSize
			extractAborted := false

			for _, zf := range zr2.File {
				if !strings.HasPrefix(zf.Name, "uploads/") || strings.HasSuffix(zf.Name, "/") {
					continue
				}
				dest := filepath.Join(".", zf.Name)
				if !strings.HasPrefix(filepath.Clean(dest), "uploads") {
					fieldLogger.Warnf("Skipping suspicious zip entry: %s", zf.Name)
					continue
				}

				// Check the declared uncompressed size before extraction
				if extractedBytes+int64(zf.UncompressedSize64) > extractLimit {
					fieldLogger.Errorf("Extraction limit exceeded: %d + %d > %d bytes",
						extractedBytes, zf.UncompressedSize64, extractLimit)
					extractAborted = true
					break
				}

				if err := os.MkdirAll(filepath.Dir(dest), os.ModePerm); err != nil {
					fieldLogger.WithError(err).Errorf("Failed to create dir for %s", dest)
					continue
				}

				rc, err := zf.Open()
				if err != nil {
					fieldLogger.WithError(err).Errorf("Failed to open zip entry %s", zf.Name)
					continue
				}

				out, err := os.Create(dest)
				if err != nil {
					rc.Close()
					fieldLogger.WithError(err).Errorf("Failed to create file %s", dest)
					continue
				}

				// Use a limited reader to enforce the cap even if the declared
				// size in the zip header is spoofed (decompression bomb defense).
				remaining := extractLimit - extractedBytes
				written, copyErr := io.Copy(out, io.LimitReader(rc, remaining+1))
				out.Close()
				rc.Close()

				if written > remaining {
					fieldLogger.Errorf("Extraction limit exceeded during write of %s", zf.Name)
					os.Remove(dest)
					extractAborted = true
					break
				}

				extractedBytes += written
				if copyErr != nil {
					fieldLogger.WithError(copyErr).Errorf("Failed to write file %s", dest)
				}
				filesRestored++
			}

			if extractAborted {
				restoreMu.Lock()
				currentRestore.Error = "api_backup_extract_too_large"
				restoreMu.Unlock()
				fieldLogger.Error("Restore aborted: extraction size limit exceeded")
			}
		}
	}

	fieldLogger.Infof("Restore complete: %d files extracted", filesRestored)

	// Reload in-memory config from the newly restored DB.
	if reopenedDB, dbErr := model.GetDB(); dbErr == nil {
		LoadSettings(reopenedDB)
	} else {
		fieldLogger.WithError(dbErr).Error("Failed to obtain DB after restore for LoadSettings")
	}

	// Mark complete with final stats
	restoreMu.Lock()
	currentRestore.Phase = "complete"
	currentRestore.Tables = payload.Manifest.Tables
	currentRestore.Files = filesRestored
	currentRestore.CurrentTable = ""
	currentRestore.BatchNum = 0
	currentRestore.TotalBatches = 0
	currentRestore.TablesLeft = 0
	restoreMu.Unlock()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dumpTable runs SELECT * on the given table and returns every row as a
// map[string]interface{}. Values are coerced to JSON-friendly types.
func dumpTable(db *sql.DB, table string) ([]map[string]interface{}, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", table)) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			row[col] = normaliseValue(vals[i])
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// normaliseValue converts sql-driver types to JSON-safe primitives.
func normaliseValue(v interface{}) interface{} {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	default:
		return val
	}
}

// insertRows inserts a slice of row-maps into the named table within the
// provided transaction. Column order is taken from the first row.
func insertRows(tx *sql.Tx, table string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	cols := columnsFromRow(rows[0])
	stmt := buildInsertStmt(table, cols)

	prepared, err := tx.Prepare(stmt)
	if err != nil {
		return fmt.Errorf("prepare %s: %w", table, err)
	}
	defer prepared.Close()

	for _, row := range rows {
		vals := rowValues(cols, row)
		if _, err := prepared.Exec(vals...); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
	}
	return nil
}

// insertRowsBatched inserts rows using multi-row INSERT statements in chunks,
// each in its own transaction. The write lock is released between batches.
func insertRowsBatched(db *sql.DB, table string, rows []map[string]interface{}, batchSize int) error {
	if len(rows) == 0 {
		return nil
	}

	cols := columnsFromRow(rows[0])
	totalRows := len(rows)
	totalBatches := (totalRows + batchSize - 1) / batchSize

	for i := 0; i < totalRows; i += batchSize {
		end := i + batchSize
		if end > totalRows {
			end = totalRows
		}
		batch := rows[i:end]
		batchNum := i/batchSize + 1

		stmt, allVals := buildMultiRowInsert(table, cols, batch)

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin batch %d: %w", batchNum, err)
		}

		if _, err := tx.Exec(stmt, allVals...); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert into %s batch %d: %w", table, batchNum, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit batch %d: %w", batchNum, err)
		}

		logger.Log.Infof("  %s: batch %d/%d (%d rows)", table, batchNum, totalBatches, len(batch))
	}
	return nil
}

// bulkInserter is the minimal interface needed for batched inserts — satisfied
// by both *sql.DB and *sql.Conn so we can pin a connection for SQLite.
type bulkInserter interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// insertRowsBatchedWithProgress inserts rows into a large table with
// progress updates. For Postgres it uses multi-row INSERTs in chunks.
// For SQLite it uses a single prepared statement executed per-row inside
// one transaction — this avoids the overhead of parsing a massive SQL
// string with 20K+ parameters per batch, which the pure-Go modernc
// SQLite driver handles poorly at scale.
func insertRowsBatchedWithProgress(ctx context.Context, exec bulkInserter, table string, rows []map[string]interface{}, batchSize, tablesLeft, totalTables int) error {
	if len(rows) == 0 {
		return nil
	}

	cols := columnsFromRow(rows[0])
	totalRows := len(rows)
	totalBatches := (totalRows + batchSize - 1) / batchSize

	if model.IsSQLite() {
		return insertRowsPreparedWithProgress(ctx, exec, table, cols, rows, batchSize, totalBatches, tablesLeft, totalTables)
	}

	// Postgres path: multi-row INSERT in batches (fast with lib/pq)
	tx, err := exec.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin bulk insert: %w", err)
	}

	for i := 0; i < totalRows; i += batchSize {
		end := i + batchSize
		if end > totalRows {
			end = totalRows
		}
		batch := rows[i:end]
		batchNum := i/batchSize + 1

		updateRestoreStatus("restoring", table, batchNum, totalBatches, tablesLeft, totalTables)

		stmt, allVals := buildMultiRowInsert(table, cols, batch)
		if _, err := tx.Exec(stmt, allVals...); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert into %s batch %d: %w", table, batchNum, err)
		}

		logger.Log.Infof("  %s: batch %d/%d (%d rows)", table, batchNum, totalBatches, len(batch))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bulk insert: %w", err)
	}
	return nil
}

// insertRowsPreparedWithProgress uses a single prepared INSERT statement
// executed once per row inside one transaction. This is dramatically faster
// on SQLite because: (1) the SQL is parsed/compiled once (tiny statement),
// (2) each Exec binds only N_cols values instead of thousands, and (3) the
// VDBE bytecode is cached across all executions.
func insertRowsPreparedWithProgress(ctx context.Context, exec bulkInserter, table string, cols []string, rows []map[string]interface{}, batchSize, totalBatches, tablesLeft, totalTables int) error {
	tx, err := exec.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin prepared insert: %w", err)
	}

	stmt := buildInsertStmt(table, cols)
	prepared, err := tx.Prepare(stmt)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare %s: %w", table, err)
	}
	defer prepared.Close()

	totalRows := len(rows)
	for i, row := range rows {
		vals := make([]interface{}, len(cols))
		for j, col := range cols {
			vals[j] = coerceJSONValue(row[col])
		}
		if _, err := prepared.Exec(vals...); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert into %s row %d: %w", table, i, err)
		}

		// Update progress every batchSize rows
		if (i+1)%batchSize == 0 || i == totalRows-1 {
			batchNum := i/batchSize + 1
			updateRestoreStatus("restoring", table, batchNum, totalBatches, tablesLeft, totalTables)
			logger.Log.Infof("  %s: batch %d/%d (%d rows)", table, batchNum, totalBatches, batchSize)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit prepared insert: %w", err)
	}
	return nil
}

func columnsFromRow(row map[string]interface{}) []string {
	cols := make([]string, 0, len(row))
	for k := range row {
		cols = append(cols, k)
	}
	return cols
}

func buildInsertStmt(table string, cols []string) string {
	placeholders := make([]string, len(cols))
	for i := range cols {
		if model.IsPostgres() {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		} else {
			placeholders[i] = "?"
		}
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
}

func buildMultiRowInsert(table string, cols []string, rows []map[string]interface{}) (string, []interface{}) {
	colCount := len(cols)
	allVals := make([]interface{}, 0, colCount*len(rows))
	rowPlaceholders := make([]string, 0, len(rows))

	paramIdx := 1
	for _, row := range rows {
		ph := make([]string, colCount)
		for i, col := range cols {
			if model.IsPostgres() {
				ph[i] = fmt.Sprintf("$%d", paramIdx)
				paramIdx++
			} else {
				ph[i] = "?"
			}
			allVals = append(allVals, coerceJSONValue(row[col]))
		}
		rowPlaceholders = append(rowPlaceholders, "("+strings.Join(ph, ", ")+")")
	}

	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		table,
		strings.Join(cols, ", "),
		strings.Join(rowPlaceholders, ", "),
	)
	return stmt, allVals
}

func rowValues(cols []string, row map[string]interface{}) []interface{} {
	vals := make([]interface{}, len(cols))
	for i, col := range cols {
		vals[i] = coerceJSONValue(row[col])
	}
	return vals
}

func coerceJSONValue(v interface{}) interface{} {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	}
	return v
}
