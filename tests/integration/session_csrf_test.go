package integration

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// Phase 5 of docs/TEST_PLAN_2.md.
//
// The rest of the integration suite mostly authenticates mutating
// handlers with X-API-KEY, which causes csrfMiddleware to skip
// validation entirely. That is correct for the ingest contract (sensor
// data, operator scripts), but it means the dashboard's actual
// browser-facing path — session cookie + CSRF token from
// <meta name="csrf-token"> — could regress without the existing tests
// noticing.
//
// Each test below covers one major dashboard resource and exercises:
//   - cookie + valid X-CSRF-Token   → success
//   - cookie + missing X-CSRF-Token → 403
//   - no cookie + no token          → 401 / 403 (rejected)
//
// A deliberate breakage of the CSRF check (commenting the
// subtle.ConstantTimeCompare branch in app/engine.go) makes the middle
// branch fail across all five tests — that is the regression we want
// the suite to catch.

const sessionCSRFPassword = "session-csrf-pw"

// TestSessionCSRF_PlantCreate covers POST /plants over the session
// path. The CSRF token is pulled from /plant/new (the dashboard's add
// form), matching what the real frontend reads.
func TestSessionCSRF_PlantCreate(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, sessionCSRFPassword)
	testutil.SeedBreeder(t, db, "Test Breeder")
	testutil.SeedStrain(t, db, 1, "Test Strain")
	testutil.SeedZone(t, db, "Test Zone")

	c, token := server.LoginAndFetchCSRF(t, sessionCSRFPassword, "/plant/new")

	body := map[string]interface{}{
		"name":      "Plant via session",
		"zone_id":   1,
		"strain_id": 1,
		"status_id": statusIDByNameInt(t, db, "Veg"),
		"date":      "2026-04-25",
		"sensors":   "[]",
		"clone":     0,
	}

	t.Run("cookie + valid token succeeds", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/plants", token, body)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("cookie + missing token is rejected by CSRF", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/plants", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"missing X-CSRF-Token must be rejected by CSRF middleware")
	})

	t.Run("no cookie is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		resp := anon.SessionPostJSON(t, "/plants", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode, "anonymous POST /plants must be rejected (got %d)", resp.StatusCode)
	})
}

// TestSessionCSRF_StrainCreate covers POST /strains. CSRF token comes
// from /strain/new.
func TestSessionCSRF_StrainCreate(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, sessionCSRFPassword)
	testutil.SeedBreeder(t, db, "Test Breeder")

	c, token := server.LoginAndFetchCSRF(t, sessionCSRFPassword, "/strain/new")

	body := map[string]interface{}{
		"name":       "Session Strain",
		"breeder_id": 1,
		"indica":     50,
		"sativa":     50,
		"autoflower": false,
		"seed_count": 0,
	}

	t.Run("cookie + valid token succeeds", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/strains", token, body)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("cookie + missing token is rejected by CSRF", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/strains", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("no cookie is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		resp := anon.SessionPostJSON(t, "/strains", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode, "anonymous POST /strains must be rejected (got %d)", resp.StatusCode)
	})
}

// TestSessionCSRF_SettingsSave covers POST /settings. CSRF token comes
// from /settings (which is itself an auth-gated edit page, not a
// dedicated "new" form).
func TestSessionCSRF_SettingsSave(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, sessionCSRFPassword)

	c, token := server.LoginAndFetchCSRF(t, sessionCSRFPassword, "/settings")

	// Empty payload is a valid SaveSettings call — every field is
	// optional and the handler treats omitted strings as "leave
	// untouched". The point of this test is the CSRF round-trip, not
	// the SaveSettings semantics.
	body := map[string]interface{}{}

	t.Run("cookie + valid token succeeds", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/settings", token, body)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("cookie + missing token is rejected by CSRF", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/settings", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("no cookie is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		resp := anon.SessionPostJSON(t, "/settings", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode, "anonymous POST /settings must be rejected (got %d)", resp.StatusCode)
	})
}

// TestSessionCSRF_SensorEdit covers POST /sensors/edit. CSRF token
// comes from /sensors. Editing requires a pre-existing sensor row.
func TestSessionCSRF_SensorEdit(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, sessionCSRFPassword)
	testutil.SeedZone(t, db, "Test Zone")
	sensorID := testutil.SeedSensor(t, db, "ec", "device-1", "temp")

	c, token := server.LoginAndFetchCSRF(t, sessionCSRFPassword, "/sensors")

	body := map[string]interface{}{
		"id":         sensorID,
		"name":       "Renamed via session",
		"visibility": "zone",
		"zone_id":    1,
		"unit":       "°C",
	}

	t.Run("cookie + valid token succeeds", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/sensors/edit", token, body)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("cookie + missing token is rejected by CSRF", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/sensors/edit", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("no cookie is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		resp := anon.SessionPostJSON(t, "/sensors/edit", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode, "anonymous POST /sensors/edit must be rejected (got %d)", resp.StatusCode)
	})
}

// TestSessionCSRF_ZoneCreate covers POST /zones. Zones are managed
// from /settings, so the CSRF token is fetched there.
func TestSessionCSRF_ZoneCreate(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	testutil.SeedAdmin(t, db, sessionCSRFPassword)

	c, token := server.LoginAndFetchCSRF(t, sessionCSRFPassword, "/settings")

	body := map[string]interface{}{"zone_name": "Session Zone"}

	t.Run("cookie + valid token succeeds", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/zones", token, body)
		defer testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("cookie + missing token is rejected by CSRF", func(t *testing.T) {
		resp := c.SessionPostJSON(t, "/zones", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("no cookie is rejected", func(t *testing.T) {
		anon := server.NewClient(t)
		resp := anon.SessionPostJSON(t, "/zones", "", body)
		defer testutil.DrainAndClose(resp)
		assert.Containsf(t, []int{http.StatusUnauthorized, http.StatusForbidden},
			resp.StatusCode, "anonymous POST /zones must be rejected (got %d)", resp.StatusCode)
	})
}
