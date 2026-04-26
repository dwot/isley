package testutil

import (
	"database/sql"
	"testing"

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
	upsertSetting(t, db, "auth_username", "admin")
	upsertSetting(t, db, "auth_password", hashed)
	if forceChange {
		upsertSetting(t, db, "force_password_change", "true")
	} else {
		upsertSetting(t, db, "force_password_change", "false")
	}
}

// upsertSetting writes name=value to the settings table, replacing any
// existing row with the same name. SQLite-only; the harness only ever
// runs against testutil.NewTestDB which is in-memory SQLite.
func upsertSetting(t *testing.T, db *sql.DB, name, value string) {
	t.Helper()
	var existingID int
	err := db.QueryRow(`SELECT id FROM settings WHERE name = $1`, name).Scan(&existingID)
	switch {
	case err == sql.ErrNoRows:
		if _, err := db.Exec(`INSERT INTO settings (name, value) VALUES ($1, $2)`, name, value); err != nil {
			t.Fatalf("upsertSetting insert %q: %v", name, err)
		}
	case err != nil:
		t.Fatalf("upsertSetting select %q: %v", name, err)
	default:
		if _, err := db.Exec(`UPDATE settings SET value = $1 WHERE id = $2`, value, existingID); err != nil {
			t.Fatalf("upsertSetting update %q: %v", name, err)
		}
	}
}
