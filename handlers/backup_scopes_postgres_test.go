//go:build integration_postgres

package handlers_test

import (
	"testing"

	"isley/tests/testutil"
)

// Postgres counterpart of TestApplyBackupToDB_PreScopeKeysRestoreAsAdmin:
// the restore path truncates and resets sequences differently there.
func TestPostgresApplyBackupToDB_PreScopeKeysRestoreAsAdmin(t *testing.T) {
	t.Parallel()
	assertPreScopeKeysRestoreAsAdmin(t, testutil.NewTestPostgresDB(t))
}

func TestPostgresApplyBackupToDB_LegacyPlantSensorJSON(t *testing.T) {
	t.Parallel()
	assertLegacyPlantSensorJSON(t, testutil.NewTestPostgresDB(t))
}
