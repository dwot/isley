package handlers_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// An archive written before scoped keys has api_keys rows without a scopes
// column. Those keys could do everything when the archive was made, so they
// must come back as admin, not as the column default (read).
func TestApplyBackupToDB_PreScopeKeysRestoreAsAdmin(t *testing.T) {
	t.Parallel()
	assertPreScopeKeysRestoreAsAdmin(t, testutil.NewTestDB(t))
}

func assertPreScopeKeysRestoreAsAdmin(t *testing.T, db *sql.DB) {
	t.Helper()
	payload := handlers.BackupPayload{
		APIKeys: []map[string]interface{}{
			{"id": 1, "name": "Old key", "key_hash": "x", "prefix": "abcd1234"},
			{"id": 2, "name": "New key", "key_hash": "y", "prefix": "efgh5678", "scopes": "read,ingest"},
		},
	}
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), db, payload))

	got := map[string]string{}
	rows, err := db.Query(`SELECT name, scopes FROM api_keys`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name, scopes string
		require.NoError(t, rows.Scan(&name, &scopes))
		got[name] = scopes
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, map[string]string{"Old key": "admin", "New key": "read,ingest"}, got)
}
