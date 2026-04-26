package testutil

// Seed helpers for the resource tree (breeder → strain → zone → plant)
// plus settings, sensors, and plant_activity. Phase 2 of
// docs/TEST_PLAN.md consolidates these out of ~10 file-local copies in
// handlers/*_test.go and tests/integration/*.go. The goal is one
// canonical place for "minimum viable row" so tests don't drift on
// column ordering or default values.

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/utils"
)

// SeedAdmin writes the admin credentials directly to the settings table,
// mirroring what main.go's startup hook does for a fresh database. It
// sets force_password_change to false so LoginAsAdmin redirects to "/"
// rather than "/change-password" — tests that exercise the forced-change
// flow should call SeedAdminWithForceChange instead.
//
// Implementation note: this uses raw SQL rather than handlers.UpdateSetting
// because UpdateSetting calls LoadSettings, which still depends on the
// model.GetDB() global (Phase 7 of TEST_PLAN.md). Until that decoupling
// lands, the test harness avoids the global by writing the rows directly.
func SeedAdmin(t *testing.T, db *sql.DB, password string) {
	t.Helper()
	seedAdmin(t, db, password, false)
}

// SeedAdminWithForceChange seeds the admin credentials and leaves
// force_password_change set to true. After login the harness redirects
// to /change-password.
func SeedAdminWithForceChange(t *testing.T, db *sql.DB, password string) {
	t.Helper()
	seedAdmin(t, db, password, true)
}

func seedAdmin(t *testing.T, db *sql.DB, password string, forceChange bool) {
	t.Helper()
	hashed, err := utils.HashPassword(password)
	if err != nil {
		t.Fatalf("SeedAdmin: hash password: %v", err)
	}
	UpsertSetting(t, db, "auth_username", "admin")
	UpsertSetting(t, db, "auth_password", hashed)
	if forceChange {
		UpsertSetting(t, db, "force_password_change", "true")
	} else {
		UpsertSetting(t, db, "force_password_change", "false")
	}
}

// UpsertSetting writes name=value to the settings table, replacing any
// existing row with the same name. Tests previously duplicated this
// helper across at least five test files.
func UpsertSetting(t *testing.T, db *sql.DB, name, value string) {
	t.Helper()
	var existingID int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = $1`, name).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		if _, err := db.Exec(`INSERT INTO settings (name, value) VALUES ($1, $2)`, name, value); err != nil {
			t.Fatalf("UpsertSetting insert %q: %v", name, err)
		}
	case err != nil:
		t.Fatalf("UpsertSetting select %q: %v", name, err)
	default:
		if _, err := db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, value, existingID); err != nil {
			t.Fatalf("UpsertSetting update %q: %v", name, err)
		}
	}
}

// SeedAPIKey hashes plaintext with handlers.HashAPIKey, writes the
// resulting hash into settings.api_key, and returns plaintext so the
// caller can pass it back through the X-API-KEY header.
func SeedAPIKey(t *testing.T, db *sql.DB, plaintext string) string {
	t.Helper()
	UpsertSetting(t, db, "api_key", handlers.HashAPIKey(plaintext))
	return plaintext
}

// SeedBreeder inserts a breeder row and returns its id.
func SeedBreeder(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO breeder (name) VALUES ($1)`, name)
	require.NoErrorf(t, err, "SeedBreeder insert %q", name)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// SeedStrain inserts a strain rooted at breederID and returns its id.
// Defaults: 50/50 indica/sativa split, photoperiod, no description, 5
// seeds — matches the sentinel "test strain" most existing fixtures use.
func SeedStrain(t *testing.T, db *sql.DB, breederID int, name string) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ($1, $2, 50, 50, 0, '', 5)`,
		name, breederID,
	)
	require.NoErrorf(t, err, "SeedStrain insert %q", name)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// SeedZone inserts a zones row and returns its id.
func SeedZone(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO zones (name) VALUES ($1)`, name)
	require.NoErrorf(t, err, "SeedZone insert %q", name)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// SeedPlant inserts a plant linked to strainID + zoneID and returns its
// id. Defaults: not a clone, start_dt 2026-01-01, no sensors. Tests that
// need other values should UPDATE the row after seeding rather than
// branching the helper.
func SeedPlant(t *testing.T, db *sql.DB, name string, strainID, zoneID int) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors)
		 VALUES ($1, $2, $3, '', 0, '2026-01-01', '[]')`,
		name, zoneID, strainID,
	)
	require.NoErrorf(t, err, "SeedPlant insert %q", name)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// SeedSensor inserts a sensors row keyed by (source, device, kind) and
// returns its id. The display name defaults to "device:kind"; tests
// that care about the name can UPDATE it afterwards.
func SeedSensor(t *testing.T, db *sql.DB, source, device, kind string) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO sensors (name, source, device, type) VALUES ($1, $2, $3, $4)`,
		device+":"+kind, source, device, kind,
	)
	require.NoErrorf(t, err, "SeedSensor insert %s/%s/%s", source, device, kind)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}

// SeedActivity inserts a plant_activity row for plantID and returns
// its id. Tests that need a non-empty note should UPDATE the row.
func SeedActivity(t *testing.T, db *sql.DB, plantID, activityID int, when time.Time) int {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, '', $3)`,
		plantID, activityID, when.Format("2006-01-02 15:04:05"),
	)
	require.NoErrorf(t, err, "SeedActivity plant=%d activity=%d", plantID, activityID)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return int(id)
}
