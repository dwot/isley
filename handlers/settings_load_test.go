package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/handlers"
	"isley/tests/testutil"
)

// Switches saved on the settings page come back after a restart.
func TestLoadSettings_FeatureSwitches(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db, "UPDATE settings SET value = '0' WHERE name = 'api_ingest_enabled'")
	testutil.MustExec(t, db, "INSERT INTO settings (name, value) VALUES ('mcp_enabled', '1')")

	store := config.NewStore()
	assert.Equal(t, 1, store.APIIngestEnabled())
	assert.Equal(t, 0, store.MCPEnabled())
	handlers.LoadSettings(db, store)
	assert.Equal(t, 0, store.APIIngestEnabled(), "ingest stays off")
	assert.Equal(t, 1, store.MCPEnabled())
}

// UpdateSetting adds or replaces a setting, reloads the Store, and reports
// a failed write instead of swallowing it.
func TestUpdateSetting(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	store := config.NewStore()

	require.NoError(t, handlers.UpdateSetting(db, store, "mcp_enabled", "1"))
	assert.Equal(t, 1, store.MCPEnabled(), "reloaded")
	require.NoError(t, handlers.UpdateSetting(db, store, "mcp_enabled", "0"))
	assert.Equal(t, 0, store.MCPEnabled())
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM settings WHERE name = 'mcp_enabled'").Scan(&n))
	assert.Equal(t, 1, n, "one row, updated in place")

	testutil.MustExec(t, db, "DROP TABLE settings")
	assert.Error(t, handlers.UpdateSetting(db, nil, "mcp_enabled", "1"))
}
