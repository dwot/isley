// Package smoke holds the in-process integration tests built on the new
// tests/testutil harness from docs/TEST_PLAN.md Phase 1.
//
// They live in their own package, separate from tests/integration, so
// they do not inherit the legacy subprocess-based TestMain that spawns
// the isley binary on port 8080. Once Phase 5 retires that subprocess
// harness, these tests fold back into tests/integration.
package smoke

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
