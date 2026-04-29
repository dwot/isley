package handlers_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// External test package — handlers_test — keeps us clear of the
// handlers→app→handlers cycle that arises when handlers' internal
// _test.go files import tests/testutil (which transitively imports app
// which imports handlers).

func seedSampleData(t *testing.T, db *sql.DB) {
	t.Helper()
	exec := func(query string, args ...interface{}) {
		_, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
	}
	// settings rows — easy to compare round-trip.
	exec(`INSERT INTO settings (name, value) VALUES ('canary', 'green')`)
	exec(`INSERT INTO settings (name, value) VALUES ('polling_interval', '60')`)

	// zones — referenced by sensors via FK.
	exec(`INSERT INTO zones (id, name) VALUES (1, 'Tent A')`)

	// sensors — referenced by sensor_data via FK.
	exec(`INSERT INTO sensors (id, name, zone_id, source, device, type)
	      VALUES (1, 'Tent A Temp', 1, 'acinfinity', 'DEV1', 'ACI.tempC')`)

	// sensor_data — multiple rows.
	exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 21.0)`)
	exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 22.5)`)

	// breeder + strain to exercise more tables. strain has several
	// NOT NULL columns we must satisfy.
	exec(`INSERT INTO breeder (id, name) VALUES (1, 'Acme Genetics')`)
	exec(`INSERT INTO strain (id, name, sativa, indica, autoflower, description, seed_count, breeder_id)
	      VALUES (1, 'OG Test', 50, 50, 0, 'desc', 5, 1)`)
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

// ---------------------------------------------------------------------------
// BuildBackupArchive
// ---------------------------------------------------------------------------

func TestBuildBackupArchive_HappyPath(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	seedSampleData(t, db)

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	archive, manifest, err := handlers.BuildBackupArchive(db, handlers.BuildArchiveOptions{
		Version: "test-1.2.3",
		Now:     now,
	})
	require.NoError(t, err)
	require.NotEmpty(t, archive, "archive bytes must not be empty")

	// Manifest fields populated.
	assert.Equal(t, "test-1.2.3", manifest.Version)
	assert.Equal(t, "sqlite", manifest.Driver)
	assert.Equal(t, now.UTC().Format(time.RFC3339), manifest.CreatedAt)
	assert.False(t, manifest.IncludeImages)
	assert.Equal(t, 0, manifest.SensorDays, "default sensor_days = 0 (all)")
	assert.GreaterOrEqual(t, manifest.Tables, 5, "at least the five seeded tables should be in the count")

	// Zip contains backup.json.
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)

	var foundJSON bool
	for _, zf := range zr.File {
		if zf.Name == "backup.json" {
			foundJSON = true
			rc, err := zf.Open()
			require.NoError(t, err)
			defer rc.Close()

			var parsed handlers.BackupPayload
			require.NoError(t, json.NewDecoder(rc).Decode(&parsed))

			// Spot-check shape: settings table has the canary row.
			require.NotEmpty(t, parsed.Settings)
			gotSetting := false
			for _, row := range parsed.Settings {
				if row["name"] == "canary" && row["value"] == "green" {
					gotSetting = true
					break
				}
			}
			assert.True(t, gotSetting, "canary settings row must round-trip into backup.json")

			assert.Len(t, parsed.SensorData, 2, "two sensor_data rows seeded")
			assert.Len(t, parsed.Zones, 1)
			assert.Len(t, parsed.Sensors, 1)
		}
	}
	assert.True(t, foundJSON, "archive must contain backup.json")
}

func TestBuildBackupArchive_SkipSensorData(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	seedSampleData(t, db)

	archive, manifest, err := handlers.BuildBackupArchive(db, handlers.BuildArchiveOptions{
		SensorDays: -1,
	})
	require.NoError(t, err)
	assert.Equal(t, -1, manifest.SensorDays)

	parsed, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)
	assert.Empty(t, parsed.SensorData, "SensorDays=-1 must omit sensor_data")
}

func TestBuildBackupArchive_FilteredSensorDays(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	seedSampleData(t, db)

	// Insert one row dated 60 days ago (should be filtered out).
	res, err := db.Exec(`INSERT INTO sensor_data (sensor_id, value) VALUES (1, 999.0)`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	old := time.Now().AddDate(0, 0, -60).Format("2006-01-02 15:04:05")
	_, err = db.Exec(`UPDATE sensor_data SET create_dt = $1 WHERE id = $2`, old, id)
	require.NoError(t, err)

	archive, _, err := handlers.BuildBackupArchive(db, handlers.BuildArchiveOptions{
		SensorDays: 30,
	})
	require.NoError(t, err)

	parsed, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)

	for _, row := range parsed.SensorData {
		// json.Number → digits as string in dump.
		v, _ := row["value"].(json.Number)
		assert.NotEqual(t, "999", v.String(), "60-day-old row must be filtered out by SensorDays=30")
	}
}

// ---------------------------------------------------------------------------
// ParseBackupArchive — malformed inputs
// ---------------------------------------------------------------------------

func TestParseBackupArchive_EmptyBytes(t *testing.T) {
	t.Parallel()
	_, err := handlers.ParseBackupArchive(nil)
	assert.Error(t, err)
}

func TestParseBackupArchive_NotAZip(t *testing.T) {
	t.Parallel()
	_, err := handlers.ParseBackupArchive([]byte("definitely not a zip file"))
	assert.Error(t, err)
}

func TestParseBackupArchive_MissingManifest(t *testing.T) {
	t.Parallel()

	// Build a valid zip with a non-manifest entry inside.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("readme.txt")
	require.NoError(t, err)
	_, _ = w.Write([]byte("hello"))
	require.NoError(t, zw.Close())

	_, err = handlers.ParseBackupArchive(buf.Bytes())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup.json")
}

func TestParseBackupArchive_BadJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("backup.json")
	require.NoError(t, err)
	_, _ = w.Write([]byte("{ not valid json"))
	require.NoError(t, zw.Close())

	_, err = handlers.ParseBackupArchive(buf.Bytes())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ApplyBackupToDB — round-trip
// ---------------------------------------------------------------------------

func TestApplyBackupToDB_RoundTrip(t *testing.T) {
	t.Parallel()

	src := testutil.NewTestDB(t)
	seedSampleData(t, src)

	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)

	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)

	// Fresh DB — no overlap with src.
	dst := testutil.NewTestDB(t)
	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, payload))

	// Row counts must match for every seeded table.
	for _, table := range []string{"settings", "zones", "sensors", "sensor_data", "breeder", "strain"} {
		assert.Equalf(t, rowCount(t, src, table), rowCount(t, dst, table),
			"row count mismatch for %s after round-trip", table)
	}

	// Spot-check content: the canary setting survives.
	var v string
	require.NoError(t, dst.QueryRow(`SELECT value FROM settings WHERE name = 'canary'`).Scan(&v))
	assert.Equal(t, "green", v)

	// And the strain join: name + breeder_id correct.
	var name string
	var breederID int
	require.NoError(t, dst.QueryRow(`SELECT name, breeder_id FROM strain WHERE id = 1`).Scan(&name, &breederID))
	assert.Equal(t, "OG Test", name)
	assert.Equal(t, 1, breederID)
}

func TestApplyBackupToDB_OverwritesExistingData(t *testing.T) {
	t.Parallel()

	dst := testutil.NewTestDB(t)
	// Pre-populate with rows that should be wiped by the restore.
	_, err := dst.Exec(`INSERT INTO settings (name, value) VALUES ('stale', 'should-be-gone')`)
	require.NoError(t, err)

	src := testutil.NewTestDB(t)
	seedSampleData(t, src)
	archive, _, err := handlers.BuildBackupArchive(src, handlers.BuildArchiveOptions{})
	require.NoError(t, err)
	payload, err := handlers.ParseBackupArchive(archive)
	require.NoError(t, err)

	require.NoError(t, handlers.ApplyBackupToDB(context.Background(), dst, payload))

	var staleCount int
	require.NoError(t, dst.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'stale'`).Scan(&staleCount))
	assert.Zero(t, staleCount, "pre-existing rows must be truncated before restore")
}

func TestApplyBackupToDB_NilDB(t *testing.T) {
	t.Parallel()
	err := handlers.ApplyBackupToDB(context.Background(), nil, handlers.BackupPayload{})
	assert.Error(t, err)
}
