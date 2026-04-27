package handlers

import (
	"database/sql"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
)

// contextKeyBackupService is the key under which the engine middleware
// stores the per-engine *BackupService.
const contextKeyBackupService = "backupService"

// BackupService owns the in-memory state for backup and restore
// operations. One instance per running app; constructed by app.NewEngine
// and made available to handlers via the Gin context.
//
// All exported state is mutex-guarded; callers may freely read and write
// across goroutines.
type BackupService struct {
	db      *sql.DB
	dataDir string

	mu      sync.Mutex
	backup  BackupStatus
	restore RestoreStatus
}

// NewBackupService constructs a BackupService bound to db and using
// dataDir as the root for backup files (typically "data"). The directory
// is not created here; the service does that lazily when a backup is
// written.
func NewBackupService(db *sql.DB, dataDir string) *BackupService {
	if dataDir == "" {
		dataDir = "data"
	}
	return &BackupService{db: db, dataDir: dataDir}
}

// DB returns the *sql.DB the service was constructed with. The async
// backup/restore goroutines need a handle that does not depend on the
// model package's process-global; this is that handle.
func (s *BackupService) DB() *sql.DB { return s.db }

// DataDir returns the root data directory configured for this service.
func (s *BackupService) DataDir() string { return s.dataDir }

// BackupDir returns <dataDir>/backups, where backup archives are stored.
func (s *BackupService) BackupDir() string {
	return filepath.Join(s.dataDir, "backups")
}

// BeginBackup atomically marks a backup as in-flight. Returns true if
// the caller acquired the slot; false if another backup is already
// running. On success the caller must call CompleteBackup when done.
func (s *BackupService) BeginBackup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backup.InProgress {
		return false
	}
	s.backup = BackupStatus{InProgress: true}
	return true
}

// CompleteBackup clears the in-flight flag and records the result.
// filename is empty on failure; err is nil on success.
func (s *BackupService) CompleteBackup(filename string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backup.InProgress = false
	if filename != "" {
		s.backup.Filename = filename
	}
	if err != nil {
		s.backup.Error = err.Error()
	}
}

// BackupSnapshot returns a copy of the current backup status, safe for
// JSON encoding by callers.
func (s *BackupService) BackupSnapshot() BackupStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backup
}

// SetBackupInProgress is a deterministic state setter used by tests to
// verify the 409 branch without racing a real goroutine. Production code
// does not call it.
func (s *BackupService) SetBackupInProgress(inProgress bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backup = BackupStatus{InProgress: inProgress}
}

// BeginRestore atomically marks a restore as in-flight with the initial
// phase. Returns true on success, false if another restore is already
// running. On success the caller must call CompleteRestore when done.
func (s *BackupService) BeginRestore(phase string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restore.InProgress {
		return false
	}
	s.restore = RestoreStatus{InProgress: true, Phase: phase}
	return true
}

// AbortRestore clears the in-flight flag without recording success
// metadata. Used when the request is rejected after BeginRestore but
// before any work happens (e.g. file too large, payload not a zip).
func (s *BackupService) AbortRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = RestoreStatus{}
}

// CompleteRestore marks the restore as finished, recording the final
// counts on success or err on failure.
func (s *BackupService) CompleteRestore(tables, files int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore.InProgress = false
	if err != nil {
		s.restore.Error = err.Error()
		return
	}
	s.restore.Phase = "complete"
	s.restore.Tables = tables
	s.restore.Files = files
	s.restore.CurrentTable = ""
	s.restore.BatchNum = 0
	s.restore.TotalBatches = 0
	s.restore.TablesLeft = 0
}

// SetRestoreError records an error against the in-flight restore
// without flipping InProgress. Used when the goroutine wants to bail
// out early but the deferred CompleteRestore will set InProgress=false.
func (s *BackupService) SetRestoreError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore.Error = msg
}

// UpdateRestoreProgress safely overwrites the progress fields on the
// in-flight restore status.
func (s *BackupService) UpdateRestoreProgress(phase, table string, batchNum, totalBatches, tablesLeft, totalTables int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore.Phase = phase
	s.restore.CurrentTable = table
	s.restore.BatchNum = batchNum
	s.restore.TotalBatches = totalBatches
	s.restore.TablesLeft = tablesLeft
	s.restore.TotalTables = totalTables
}

// RestoreSnapshot returns a copy of the current restore status, safe
// for JSON encoding by callers.
func (s *BackupService) RestoreSnapshot() RestoreStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restore
}

// SetRestoreInProgress is a deterministic state setter used by tests to
// verify the 409 branch without racing a real goroutine. Production code
// does not call it.
func (s *BackupService) SetRestoreInProgress(inProgress bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restore = RestoreStatus{InProgress: inProgress}
}

// BackupServiceFromContext extracts the *BackupService that the engine
// middleware injected into the Gin context. Mirrors DBFromContext.
func BackupServiceFromContext(c *gin.Context) *BackupService {
	return c.MustGet(contextKeyBackupService).(*BackupService)
}

// SetBackupServiceOnContext is the small helper the engine middleware
// uses to bind a service to a request context.
func SetBackupServiceOnContext(c *gin.Context, s *BackupService) {
	c.Set(contextKeyBackupService, s)
}
