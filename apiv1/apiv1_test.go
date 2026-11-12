package apiv1_test

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/apiv1"
	"isley/chrono"
	"isley/config"
	"isley/tests/testutil"
)

type errorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

func requireErrorEnvelope(t *testing.T, body []byte) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	require.NoError(t, json.Unmarshal(body, &env), "body: %s", body)
	require.NotEmpty(t, env.Error.Code, "body: %s", body)
	require.NotEmpty(t, env.Error.Message, "body: %s", body)
	return env
}

// fixedNow is 03:00 UTC on 11 Nov 2026: still the 10th in New York.
var fixedNow = time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)

type fixture struct {
	server *testutil.TestServer
	cc     *contractClient
	store  *config.Store
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.NewTestDB(t)
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	server := testutil.NewTestServer(t, db,
		testutil.WithConfigStore(store),
		testutil.WithUploadDir(t.TempDir()),
		testutil.WithClock(chrono.Fixed(fixedNow)))
	return fixture{server: server, cc: newContractClient(t, server), store: store}
}

func (f fixture) key(t *testing.T, scopes string) string {
	t.Helper()
	return testutil.SeedAPIKeyWithScopes(t, f.server.DB, "v1-"+strings.ReplaceAll(scopes, ",", "-")+"-000000000000000000000", scopes)
}

func TestSpecIsValid(t *testing.T) {
	t.Parallel()
	loadSpec()
	require.NoError(t, specErr)
	ok, errs := specValidator.ValidateDocument()
	assert.True(t, ok, "%v", errs)
	assert.NotEmpty(t, specOps)
}

// Every route mounted under /api/v1 must be documented, so the spec can't
// fall behind the code.
func TestSpecCoversEveryRoute(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	loadSpec()
	param := regexp.MustCompile(`:([A-Za-z_]+)`)

	routes := f.server.Engine.Routes()
	var seen int
	for _, r := range routes {
		if !apiv1.IsAPIPath(r.Path) || r.Path == apiv1.SpecPath || r.Path == apiv1.DocsPath {
			continue
		}
		seen++
		specPath := param.ReplaceAllString(strings.TrimPrefix(r.Path, apiv1.Prefix), "{$1}")
		_, ok := specOps[r.Method+" "+specPath]
		assert.True(t, ok, "%s %s is mounted but not in openapi.yaml", r.Method, specPath)
	}
	assert.Positive(t, seen)
}

func TestSpecIsServedWithoutAuth(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	resp, err := f.server.NewClient(t).Do(mustReq(t, http.MethodGet, f.server.URL+"/api/v1/openapi.yaml"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "yaml")
	assert.Equal(t, apiv1.Spec(), body)
}

func mustReq(t *testing.T, method, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	return req
}

func TestInstance(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.store.SetACIEnabled(1)
	f.store.SetAPIIngestEnabled(1)
	key := f.key(t, "read")

	resp := f.cc.get("/instance", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)

	var got struct {
		Version  string          `json:"version"`
		TimeZone string          `json:"time_zone"`
		Now      string          `json:"now"`
		Features map[string]bool `json:"features"`
		API      struct {
			KeyName string   `json:"key_name"`
			Scopes  []string `json:"scopes"`
		} `json:"api"`
	}
	require.NoError(t, json.Unmarshal(resp.Body, &got))
	assert.Equal(t, "isley-test", got.Version)
	assert.Equal(t, "America/New_York", got.TimeZone)
	assert.Equal(t, "2026-11-10T22:00:00-05:00", got.Now, "now is in the instance zone, with its offset")
	assert.True(t, got.Features["ac_infinity"])
	assert.True(t, got.Features["sensor_ingest"])
	assert.False(t, got.Features["ecowitt"])
	assert.Equal(t, []string{"read"}, got.API.Scopes)
	assert.Equal(t, "test key", got.API.KeyName)
}

// The time zone setting applies to the next request.
func TestInstance_TimeZoneFollowsSetting(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "read")
	f.store.SetTimezone("Asia/Kolkata")

	resp := f.cc.get("/instance", key)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got struct {
		TimeZone string `json:"time_zone"`
		Now      string `json:"now"`
	}
	require.NoError(t, json.Unmarshal(resp.Body, &got))
	assert.Equal(t, "Asia/Kolkata", got.TimeZone)
	assert.Equal(t, "2026-11-11T08:30:00+05:30", got.Now)
}

func TestAuth_BearerAndXAPIKeyBothWork(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "read")

	for name, headers := range map[string]map[string]string{
		"bearer":           {"Authorization": "Bearer " + key},
		"bearer lowercase": {"Authorization": "bearer " + key},
		"x-api-key":        {"X-API-KEY": key},
	} {
		resp := f.cc.do(http.MethodGet, "/instance", headers, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode, name)
	}
}

func TestAuth_Unauthorized(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "read")

	for name, headers := range map[string]map[string]string{
		"no key":       nil,
		"wrong key":    {"Authorization": "Bearer not-a-real-key"},
		"basic scheme": {"Authorization": "Basic " + key},
		// Authorization wins over X-API-KEY; a bad one isn't rescued.
		"bad bearer, good header": {"Authorization": "Bearer nope", "X-API-KEY": key},
	} {
		resp := f.cc.do(http.MethodGet, "/instance", headers, nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, name)
		env := requireErrorEnvelope(t, resp.Body)
		assert.Equal(t, apiv1.CodeUnauthorized, env.Error.Code, name)
		assert.Contains(t, resp.Header.Get("WWW-Authenticate"), "Bearer", name)
	}
}

// A browser session is not an API credential.
func TestAuth_SessionIsNotAccepted(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	testutil.SeedAdmin(t, f.server.DB, "v1-session-pw")
	c, _ := f.server.LoginAndFetchCSRF(t, "v1-session-pw", "/settings")

	resp, err := c.Do(mustReq(t, http.MethodGet, f.server.URL+"/api/v1/instance"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, apiv1.CodeUnauthorized, requireErrorEnvelope(t, body).Error.Code)
}

func TestAuth_InsufficientScope(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "ingest")

	resp := f.cc.get("/instance", key)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	env := requireErrorEnvelope(t, resp.Body)
	assert.Equal(t, apiv1.CodeInsufficientScope, env.Error.Code)
	assert.Equal(t, "read", env.Error.Details["required"])
}

// Higher scopes include read.
func TestAuth_HigherScopesCanRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, scopes := range []string{"write", "admin", "read,ingest"} {
		resp := f.cc.get("/instance", f.key(t, scopes))
		assert.Equal(t, http.StatusOK, resp.StatusCode, scopes)
	}
}

func TestUnknownPathIsJSON404(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "read")

	for _, path := range []string{"/nope", "/plants/1/nonsense"} {
		resp := f.cc.get(path, key)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, path)
		assert.Equal(t, apiv1.CodeNotFound, requireErrorEnvelope(t, resp.Body).Error.Code, path)
	}
	// Outside /api/v1 the web app's plain 404 is unchanged.
	resp, err := f.server.NewClient(t).Do(mustReq(t, http.MethodGet, f.server.URL+"/definitely-not-a-page"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, "404 page not found", string(body))
}

// /api/v1 doesn't use sessions: no CSRF check, no session cookie.
func TestNoSessionMachinery(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	key := f.key(t, "read")

	resp := f.cc.do(http.MethodPost, "/instance", bearer(key), []byte(`{}`))
	assert.NotEqual(t, http.StatusForbidden, resp.StatusCode, "a key-authenticated POST must not hit the CSRF check")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "POST /instance is not an endpoint")
	for _, c := range resp.Cookies() {
		assert.NotEqual(t, "isley_session", c.Name, "API responses must not set a session cookie")
	}

	get := f.cc.get("/instance", key)
	for _, c := range get.Cookies() {
		assert.NotEqual(t, "isley_session", c.Name)
	}
}

// Timestamps leave the API in whole seconds.
func TestInstance_NowIsWholeSeconds(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	store := config.NewStore()
	store.SetTimezone("UTC")
	server := testutil.NewTestServer(t, db,
		testutil.WithConfigStore(store),
		testutil.WithUploadDir(t.TempDir()),
		testutil.WithClock(chrono.Fixed(fixedNow.Add(987654321*time.Nanosecond))))
	cc := newContractClient(t, server)
	key := testutil.SeedAPIKeyWithScopes(t, db, "v1-seconds-read-00000000000000000", "read")

	resp := cc.get("/instance", key)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), `"now":"2026-11-11T03:00:00Z"`)
}
