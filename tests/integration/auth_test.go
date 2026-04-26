package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// resetRateLimit clears the process-global login-attempt map so this
// test does not see counts accumulated by earlier tests (or vice
// versa). Phase 7 of TEST_PLAN.md replaces the global with an
// instance-scoped store.
func resetRateLimit(t *testing.T) {
	t.Helper()
	handlers.ResetLoginAttempts()
}

// TestAuth_Logout verifies the full session-clear contract: after
// /logout the session cookie no longer authenticates against /, which
// goes back to redirecting to /login.
func TestAuth_Logout(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "logout-test-pw")

	c := server.LoginAsAdmin(t, "logout-test-pw")

	// Sanity: dashboard is reachable while logged in.
	resp := c.Get("/")
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Logout itself redirects to /login.
	logoutResp := c.Get("/logout")
	logoutResp.Body.Close()
	assert.Equal(t, http.StatusFound, logoutResp.StatusCode)
	assert.Equal(t, "/login", logoutResp.Header.Get("Location"))

	// Subsequent dashboard request must redirect — session was cleared.
	postResp := c.Get("/")
	postResp.Body.Close()
	assert.Equal(t, http.StatusFound, postResp.StatusCode)
	assert.Equal(t, "/login", postResp.Header.Get("Location"))
}

// TestAuth_CSRFRejectsPostWithoutToken verifies that a POST without a
// csrf_token field is rejected with 403, even on an unauthenticated
// route like /login. The session-bound CSRF token must be submitted
// with every POST/PUT/DELETE.
func TestAuth_CSRFRejectsPostWithoutToken(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "irrelevant")

	c := server.NewClient(t)

	// Prime the session by GETting /login first so the server-side
	// session has a CSRF token assigned. We deliberately do NOT submit
	// it back — that's the assertion.
	primer := c.Get("/login")
	primer.Body.Close()

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "irrelevant")
	// no csrf_token
	resp := c.PostForm("/login", form)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "POST without csrf_token must be 403")
}

// TestAuth_LoginRateLimiter verifies that after MaxLoginAttempts failed
// POSTs from one IP, the next attempt returns 429 rather than 401.
func TestAuth_LoginRateLimiter(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "the-real-password")

	c := server.NewClient(t)
	// Get a CSRF token once; the cookie jar carries the session across
	// POSTs so the same token applies.
	token := c.FetchCSRFToken("/login")

	// MaxLoginAttempts wrong-password POSTs should each return 401 —
	// CSRF passes, auth fails.
	for i := 0; i < handlers.MaxLoginAttempts; i++ {
		resp := c.PostForm("/login", loginForm("admin", "wrong", token))
		resp.Body.Close()
		require.Equalf(t, http.StatusUnauthorized, resp.StatusCode, "POST %d/%d should be 401", i+1, handlers.MaxLoginAttempts)
	}

	// The next POST trips the limiter.
	limited := c.PostForm("/login", loginForm("admin", "wrong", token))
	limited.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, limited.StatusCode, "MaxLoginAttempts+1 should be 429")
}

// TestAuth_SessionVersionInvalidation verifies that bumping the
// session_version setting forces all existing sessions to re-login.
// This is the mechanism HandleChangePassword uses to revoke other
// devices when a user rotates their password.
func TestAuth_SessionVersionInvalidation(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdmin(t, db, "session-pw")

	c := server.LoginAsAdmin(t, "session-pw")

	// Sanity: still logged in.
	ok := c.Get("/")
	ok.Body.Close()
	require.Equal(t, http.StatusOK, ok.StatusCode)

	// Bump session_version on the server side. The session cookie still
	// carries the OLD version; AuthMiddleware should detect the mismatch
	// and clear the session.
	testutil.UpsertSetting(t, db, "session_version", "v2-after-rotation")

	resp := c.Get("/")
	resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/login", resp.Header.Get("Location"))
}

// TestAuth_ChangePasswordFlow exercises the full force-change path:
// admin logs in, gets redirected to /change-password, posts a new
// password, ends up at "/", and can re-login with the new credentials.
func TestAuth_ChangePasswordFlow(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdminWithForceChange(t, db, "old-pw")

	c := server.NewClient(t)

	// Initial login lands on /change-password (force flag set).
	loginToken := c.FetchCSRFToken("/login")
	loginResp := c.PostForm("/login", loginForm("admin", "old-pw", loginToken))
	loginResp.Body.Close()
	require.Equal(t, http.StatusFound, loginResp.StatusCode)
	require.Equal(t, "/change-password", loginResp.Header.Get("Location"))

	// Fetch the change-password form to pick up its CSRF token.
	cpToken := c.FetchCSRFToken("/change-password")

	// Submit a new password.
	cpForm := url.Values{}
	cpForm.Set("new_password", "new-pw-12345")
	cpForm.Set("confirm_password", "new-pw-12345")
	cpForm.Set("csrf_token", cpToken)

	cpResp := c.PostForm("/change-password", cpForm)
	cpResp.Body.Close()
	require.Equal(t, http.StatusFound, cpResp.StatusCode)
	require.Equal(t, "/", cpResp.Header.Get("Location"))

	// The dashboard should now render directly — force flag cleared.
	dash := c.Get("/")
	dash.Body.Close()
	assert.Equal(t, http.StatusOK, dash.StatusCode)

	// And the new credentials should authenticate from a brand-new
	// client. Use a fresh server reset to dodge accumulated rate-limit
	// counters from this test.
	resetRateLimit(t)
	c2 := server.LoginAsAdmin(t, "new-pw-12345")
	again := c2.Get("/")
	again.Body.Close()
	assert.Equal(t, http.StatusOK, again.StatusCode)
}

// TestAuth_ChangePasswordValidation verifies the input checks: mismatched
// passwords and too-short passwords both return 400 without persisting
// anything.
func TestAuth_ChangePasswordValidation(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	testutil.SeedAdminWithForceChange(t, db, "starting-pw")

	c := server.NewClient(t)
	loginToken := c.FetchCSRFToken("/login")
	loginResp := c.PostForm("/login", loginForm("admin", "starting-pw", loginToken))
	loginResp.Body.Close()
	require.Equal(t, http.StatusFound, loginResp.StatusCode)

	cases := []struct {
		name, newPw, confirmPw string
	}{
		{"mismatch", "abcdefgh", "abcdefgX"},
		{"too short", "1234567", "1234567"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpToken := c.FetchCSRFToken("/change-password")
			form := url.Values{}
			form.Set("new_password", tc.newPw)
			form.Set("confirm_password", tc.confirmPw)
			form.Set("csrf_token", cpToken)

			resp := c.PostForm("/change-password", form)
			resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}

	// The original password should still work.
	resetRateLimit(t)
	c2 := server.NewClient(t)
	tok := c2.FetchCSRFToken("/login")
	resp := c2.PostForm("/login", loginForm("admin", "starting-pw", tok))
	resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode, "original password must still authenticate after rejected changes")
}

// TestAuth_APIKey_HappyPath verifies that an X-API-KEY header satisfies
// AuthMiddlewareApi on /api/overlay without any session cookie.
func TestAuth_APIKey_HappyPath(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const plaintextKey = "test-api-key-happy"
	seedHashedAPIKey(t, db, handlers.HashAPIKey(plaintextKey))

	c := server.NewClient(t)
	resp := c.APIGet(t, "/api/overlay", plaintextKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Body should be a JSON object with the documented keys.
	var payload map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Contains(t, payload, "plants")
	assert.Contains(t, payload, "sensors")
}

// TestAuth_APIKey_RejectsBadKey verifies a wrong key produces 401.
func TestAuth_APIKey_RejectsBadKey(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	seedHashedAPIKey(t, db, handlers.HashAPIKey("the-real-key"))

	c := server.NewClient(t)
	resp := c.APIGet(t, "/api/overlay", "not-the-real-key")
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestAuth_APIKey_RejectsNoCredentials verifies that a request with
// neither an X-API-KEY header nor a session cookie returns 401.
func TestAuth_APIKey_RejectsNoCredentials(t *testing.T) {
	resetRateLimit(t)

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	seedHashedAPIKey(t, db, handlers.HashAPIKey("the-real-key"))

	c := server.NewClient(t)
	resp := c.Get("/api/overlay")
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// seedHashedAPIKey writes an already-hashed api_key into settings. Used
// by the auth tests that exercise plaintext/SHA-256/bcrypt branches and
// need full control over what is stored.
func seedHashedAPIKey(t *testing.T, db *sql.DB, hashed string) {
	t.Helper()
	testutil.UpsertSetting(t, db, "api_key", hashed)
}
