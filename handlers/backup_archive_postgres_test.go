//go:build integration_postgres

// Postgres-flavored ApplyBackupToDB tests. The default-build counterparts
// in backup_archive_test.go exercise the SQLite branch (DELETE FROM,
// PRAGMA foreign_keys = OFF, no sequence reset). This file mirrors them
// against a real PostgreSQL so the Postgres-specific branches —
// TRUNCATE TABLE ... CASCADE, SET CONSTRAINTS ALL DEFERRED, and the
// setval(pg_get_serial_sequence(...)) sequence-reset loop — actually
// execute end-to-end against the dialect they target.
//
// Build tag: integration_postgres
//
// Local invocation:
//
//	go test -tags=integration_postgres -run Postgres ./handlers/...
//
// Note: this file builds the BackupPayload directly rather than going
// through BuildBackupArchive → ParseBackupArchive. The dump path is
// itself dialect-aware (dumpTableFiltered branches on IsPostgres()) and
// pulling in a SQLite source under this build tag would mix dialects in
// the same process. Constructing the payload by hand keeps the test
// focused on the apply path.

package handlers_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// jsonNum is a small helper for building row-maps that mirror what
// ParseBackupArchive returns: numeric columns come back as json.Number
// because the parser uses json.Decoder with UseNumber.
func jsonNum(s string) json.Number { return json.Number(s) }

// samplePayloadForPostgres mirrors the seedSampleData fixture used in
// backup_archive_test.go but as a hand-built BackupPayload. Same shape:
// settings, zones, sensors, sensor_data, breeder, strain. Lets the
// Postgres apply path exercise multi-table FK ordering, sequence reset,
// and constraint deferral without depending on a SQLite source DB.
func samplePayloadForPostgres() handlers.BackupPayload {
	return handlers.BackupPayload{
		Settings: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "canary", "value": "green"},
			{"id": jsonNum("2"), "name": "polling_interval", "value": "60"},
		},
		Zones: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "Tent A"},
		},
		Sensors: []map[string]interface{}{
			{
				"id":         jsonNum("1"),
				"name":       "Tent A Temp",
				"zone_id":    jsonNum("1"),
				"source":     "acinfinity",
				"device":     "DEV1",
				"type":       "ACI.tempC",
				"unit":       "C",
				"visibility": "zone_plant",
			},
		},
		SensorData: []map[string]interface{}{
			{"id": jsonNum("1"), "sensor_id": jsonNum("1"), "value": jsonNum("21.0")},
			{"id": jsonNum("2"), "sensor_id": jsonNum("1"), "value": jsonNum("22.5")},
		},
		Breeders: []map[string]interface{}{
			{"id": jsonNum("1"), "name": "Acme Genetics"},
		},
		Strains: []map[string]interface{}{
			{
				"id":          jsonNum("1"),
				"name":        "OG Test",
				"sativa":      jsonNum("50"),
				"indica":      jsonNum("50"),
				"autoflower":  jsonNum("0"),
				"description": "desc",
				"seed_count":  jsonNum("5"),
				"breeder_id":  jsonNum("1"),
			},
		},
	}
}

// TestApplyBackupToDB_RoundTrip_Postgres exercises the Postgres branch of
// ApplyBackupToDB end-to-end. A regression in the truncate ordering, the
// CASCADE clause, or the constraint deferral would surface here as an
// FK error or a row-count mismatch.
func TestApplyBackupToDB_RoundTrip_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)
	payload := samplePayloadForPostgres()

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, payload))

	expectedCounts := map[string]int{
		"settings":    2,
		"zones":       1,
		"sensors":     1,
		"sensor_data": 2,
		"breeder":     1,
		"strain":      1,
	}
	for table, want := range expectedCounts {
		var got int
		require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&got))
		assert.Equalf(t, want, got, "row count for %s after Postgres apply", table)
	}

	var v string
	require.NoError(t, dst.QueryRow(`SELECT value FROM settings WHERE name = 'canary'`).Scan(&v))
	assert.Equal(t, "green", v)

	var name string
	var breederID int
	require.NoError(t, dst.QueryRow(`SELECT name, breeder_id FROM strain WHERE id = 1`).Scan(&name, &breederID))
	assert.Equal(t, "OG Test", name)
	assert.Equal(t, 1, breederID)
}

// TestApplyBackupToDB_OverwritesExistingData_Postgres asserts that
// TRUNCATE ... CASCADE wipes pre-existing rows. SQLite uses DELETE FROM;
// PG needs CASCADE because dependent FK rows would otherwise block the
// truncate. A test that forgets CASCADE fails here with a foreign-key
// violation.
func TestApplyBackupToDB_OverwritesExistingData_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)

	_, err := dst.Exec(`INSERT INTO settings (name, value) VALUES ('stale', 'should-be-gone')`)
	require.NoError(t, err)

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, samplePayloadForPostgres()))

	var staleCount int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'stale'`).Scan(&staleCount))
	assert.Zero(t, staleCount, "pre-existing rows must be truncated before restore")
}

// TestApplyBackupToDB_ResetsSequences_Postgres locks in the
// post-restore setval loop. After restoring rows with explicit ids, the
// next INSERT without an id must NOT collide with an imported id —
// which only holds if the sequence got bumped past MAX(id) by the
// production setval(pg_get_serial_sequence(...)) loop. SQLite has no
// equivalent because AUTOINCREMENT tracks the high-water mark via
// sqlite_sequence automatically.
func TestApplyBackupToDB_ResetsSequences_Postgres(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestPostgresDB(t)
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, samplePayloadForPostgres()))

	// Insert without specifying id — should auto-assign past MAX(id)=1.
	var newID int
	require.NoError(t,
		dst.QueryRow(`INSERT INTO breeder (name) VALUES ($1) RETURNING id`, "Fresh").Scan(&newID),
	)
	assert.Greaterf(t, newID, 1, "sequence must be reset past the imported MAX(id); got %d", newID)
}
