// Package integration holds the in-process integration tests built on
// the tests/testutil harness from docs/TEST_PLAN.md.
//
// The legacy subprocess-based TestMain that spawned the isley binary on
// port 8080 was retired in Phase 5 along with the rest of
// tests/integration/main_flow_test.go. New end-to-end tests live here
// and use httptest.NewServer via testutil.NewTestServer.
package integration

// +parallel:serial — login rate limiter package-global
//
// TestAppSmoke drives /login via the real handler chain, which mutates
// the process-global handlers.loginAttempts map. Cleared by Phase 4.1
// of TEST_PLAN_2.md when RateLimiterService lifts the singleton.

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestAppSmoke is the canary for the new harness. It boots a fresh Gin
// engine against an in-memory SQLite database, exercises the
// unauthenticated /health probe, walks through the login flow with a
// seeded admin, and confirms post-login navigation works.
func TestAppSmoke(t *testing.T) {
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	t.Run("health is reachable without auth", func(t *testing.T) {
		c := server.NewClient(t)
		resp := c.Get("/health")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("dashboard redirects to /login when unauthenticated", func(t *testing.T) {
		c := server.NewClient(t)
		resp := c.Get("/")
		defer resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, "/login", resp.Header.Get("Location"))
	})

	t.Run("login form renders with a CSRF token", func(t *testing.T) {
		c := server.NewClient(t)
		token := c.FetchCSRFToken("/login")
		assert.NotEmpty(t, token, "csrf_token field should be present in the login form")
	})

	t.Run("admin can log in and reach the dashboard", func(t *testing.T) {
		testutil.SeedAdmin(t, db, "isley-test-pw")
		c := server.LoginAsAdmin(t, "isley-test-pw")

		// Cookie jar now carries the session — the dashboard at "/"
		// should render fully (200) rather than redirecting to /login.
		resp := c.Get("/")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forced password change redirects post-login", func(t *testing.T) {
		// Fresh server + DB so this subtest's force flag does not
		// affect the LoginAsAdmin subtest above (cookie jars are per
		// client, but the server-side state lives in the DB).
		db2 := testutil.NewTestDB(t)
		s2 := testutil.NewTestServer(t, db2)
		testutil.SeedAdminWithForceChange(t, db2, "needs-change")

		c := s2.NewClient(t)
		token := c.FetchCSRFToken("/login")
		resp := c.PostForm("/login", loginForm("admin", "needs-change", token))
		defer resp.Body.Close()

		require.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, "/change-password", resp.Header.Get("Location"))
	})

	t.Run("login with wrong password returns 401", func(t *testing.T) {
		db2 := testutil.NewTestDB(t)
		s2 := testutil.NewTestServer(t, db2)
		testutil.SeedAdmin(t, db2, "right-password")

		c := s2.NewClient(t)
		token := c.FetchCSRFToken("/login")
		resp := c.PostForm("/login", loginForm("admin", "wrong-password", token))
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func loginForm(username, password, csrfToken string) url.Values {
	v := url.Values{}
	v.Set("username", username)
	v.Set("password", password)
	v.Set("csrf_token", csrfToken)
	return v
}
