package handlers

import (
	"context"
	"database/sql"
)

// InsertRowsBatchedForTest runs the restore's batched bulk load (the path
// runRestore takes for sensor_data) with a small batch size.
func InsertRowsBatchedForTest(db *sql.DB, table string, rows []map[string]interface{}) error {
	return insertRowsBatchedWithProgress(context.Background(), &BackupService{}, db, table, rows, 7, 1, 1)
}
