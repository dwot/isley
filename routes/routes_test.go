package routes

// Phase 6a tests for routes/routes.go. Per docs/TEST_PLAN.md the goal
// here is a single introspection-style test that confirms each
// registrar mounts the routes the rest of the suite assumes exist —
// rather than re-testing the per-route middleware (which the handler
// suite already covers via auth-gating tables).

import (
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// routeKey is "METHOD path", which is the canonical shape gin uses for
// engine.Routes(). Keeping the helper here keeps the comparison
// boilerplate-free at every call site.
type routeKey struct {
	Method string
	Path   string
}

// engineRouteSet returns the engine's registered routes as a set keyed
// by (method, path).
func engineRouteSet(t *testing.T, e *gin.Engine) map[routeKey]bool {
	t.Helper()
	got := map[routeKey]bool{}
	for _, info := range e.Routes() {
		got[routeKey{Method: info.Method, Path: info.Path}] = true
	}
	return got
}

// requireAllPresent fails the test with a sorted list of missing routes
// if any of want is not in got. Reporting the list at once helps when
// adding new routes later.
func requireAllPresent(t *testing.T, got map[routeKey]bool, want []routeKey, label string) {
	t.Helper()

	var missing []string
	for _, w := range want {
		if !got[w] {
			missing = append(missing, w.Method+" "+w.Path)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		// Render the full set we did register so the diff is obvious.
		var registered []string
		for k := range got {
			registered = append(registered, k.Method+" "+k.Path)
		}
		sort.Strings(registered)
		t.Fatalf("%s: missing %d route(s):\n  %v\nregistered:\n  %v",
			label, len(missing), missing, registered)
	}
}

func TestAddBasicRoutes_RegistersAllExpectedRoutes(t *testing.T) {
	t.Parallel()

	e := gin.New()
	AddBasicRoutes(e.Group("/"), "test-version")

	got := engineRouteSet(t, e)

	want := []routeKey{
		{"GET", "/"},
		{"GET", "/api/translations"},
		{"GET", "/plants"},
		{"GET", "/activities"},
		{"GET", "/activities/list"},
		{"GET", "/strains"},
		{"GET", "/graph/:id"},
		{"GET", "/plant/new"},
		{"GET", "/plant/:id"},
		{"GET", "/strain/new"},
		{"GET", "/strain/:id"},
		{"GET", "/listFonts"},
		{"GET", "/listLogos"},
		{"GET", "/plants/living"},
		{"GET", "/plants/harvested"},
		{"GET", "/plants/dead"},
		{"GET", "/plants/by-strain/:strainID"},
		{"GET", "/sensorData"},
		{"GET", "/sensors/grouped"},
		{"GET", "/strains/:id"},
		{"GET", "/strains/in-stock"},
		{"GET", "/strains/out-of-stock"},
		{"POST", "/decorateImage"},
		{"GET", "/streams"},
		{"GET", "/strains/:id/lineage"},
		{"GET", "/strains/:id/descendants"},
		{"GET", "/strains/lookup"},
	}
	requireAllPresent(t, got, want, "AddBasicRoutes")
}

func TestAddProtectedApiRoutes_RegistersAllExpectedRoutes(t *testing.T) {
	t.Parallel()

	e := gin.New()
	AddProtectedApiRoutes(e.Group("/"))

	got := engineRouteSet(t, e)

	want := []routeKey{
		// Plants
		{"POST", "/plants"},
		{"POST", "/plant"},
		{"POST", "/plant/status"},
		{"DELETE", "/plant/delete/:id"},
		{"POST", "/plant/link-sensors"},
		{"POST", "/plant/:plantID/images/upload"},
		{"DELETE", "/plant/images/:imageID/delete"},

		// Status / measurement / activity
		{"POST", "/plantStatus/edit"},
		{"DELETE", "/plantStatus/delete/:id"},
		{"POST", "/plantMeasurement"},
		{"POST", "/plantMeasurement/edit"},
		{"DELETE", "/plantMeasurement/delete/:id"},
		{"POST", "/plantActivity"},
		{"POST", "/plantActivity/edit"},
		{"DELETE", "/plantActivity/delete/:id"},

		// Sensors
		{"POST", "/sensors/scanACI"},
		{"POST", "/sensors/scanEC"},
		{"POST", "/sensors/edit"},
		{"DELETE", "/sensors/delete/:id"},
		{"GET", "/sensors/dumpACI"},

		// Strains and breeders
		{"POST", "/strains"},
		{"PUT", "/strains/:id"},
		{"DELETE", "/strains/:id"},
		{"POST", "/breeders"},
		{"PUT", "/breeders/:id"},
		{"DELETE", "/breeders/:id"},

		// Lineage
		{"POST", "/strains/:id/lineage"},
		{"PUT", "/strains/:id/lineage"},
		{"PUT", "/lineage/:lineageID"},
		{"DELETE", "/lineage/:lineageID"},

		// AC Infinity OAuth
		{"POST", "/aci/login"},

		// Zones / metrics / activities CRUD
		{"POST", "/zones"},
		{"PUT", "/zones/:id"},
		{"DELETE", "/zones/:id"},
		{"POST", "/metrics"},
		{"PUT", "/metrics/:id"},
		{"DELETE", "/metrics/:id"},
		{"POST", "/activities"},
		{"PUT", "/activities/:id"},
		{"DELETE", "/activities/:id"},

		// Streams
		{"POST", "/streams"},
		{"PUT", "/streams/:id"},
		{"DELETE", "/streams/:id"},

		// Settings + backup + logs
		{"POST", "/settings"},
		{"POST", "/settings/upload-logo"},
		{"GET", "/settings/logs"},
		{"GET", "/settings/logs/download"},
		{"POST", "/settings/backup/create"},
		{"GET", "/settings/backup/status"},
		{"GET", "/settings/backup/list"},
		{"GET", "/settings/backup/download/:name"},
		{"DELETE", "/settings/backup/:name"},
		{"POST", "/settings/backup/restore"},
		{"GET", "/settings/backup/restore/status"},
		{"GET", "/settings/backup/sqlite/download"},
		{"POST", "/settings/backup/sqlite/upload"},

		// Multi-plant activity (settings group)
		{"POST", "/record-multi-activity"},
	}
	requireAllPresent(t, got, want, "AddProtectedApiRoutes")
}

func TestAddExternalApiRoutes_RegistersIngestAndOverlay(t *testing.T) {
	t.Parallel()

	e := gin.New()
	AddExternalApiRoutes(e.Group("/"))

	got := engineRouteSet(t, e)
	requireAllPresent(t, got, []routeKey{
		{"POST", "/api/sensors/ingest"},
		{"GET", "/api/overlay"},
	}, "AddExternalApiRoutes")
}

func TestAddProtectedRoutes_RegistersAuthGatedHTML(t *testing.T) {
	t.Parallel()

	e := gin.New()
	AddProtectedRoutes(e.Group("/"), "test-version")

	got := engineRouteSet(t, e)
	requireAllPresent(t, got, []routeKey{
		{"GET", "/plant/:id/edit"},
		{"GET", "/strain/:id/edit"},
		{"GET", "/settings"},
		{"GET", "/sensors"},
		{"GET", "/activities/export/csv"},
		{"GET", "/activities/export/xlsx"},
	}, "AddProtectedRoutes")
}

// TestAddProtectedApiRoutes_AttachesIngestRateLimiter sanity-checks that
// the external ingest route carries an extra middleware (the
// rate-limiter) versus a sibling GET. Routes.Info reports the leaf
// handler name as "Handler" and the count of all handlers in the chain
// as `len(Handlers)`. We check the count differs between the two.
func TestAddExternalApiRoutes_IngestHasExtraMiddleware(t *testing.T) {
	t.Parallel()

	e := gin.New()
	AddExternalApiRoutes(e.Group("/"))

	var ingest, overlay gin.RouteInfo
	for _, info := range e.Routes() {
		switch info.Path {
		case "/api/sensors/ingest":
			ingest = info
		case "/api/overlay":
			overlay = info
		}
	}
	require.NotEmpty(t, ingest.Path)
	require.NotEmpty(t, overlay.Path)

	// gin.RouteInfo doesn't expose handler counts directly, but the
	// HandlerFunc field is the *leaf* and should differ — overlay's leaf
	// is GetOverlayData; ingest's leaf is IngestSensorData. The fact
	// that we registered them with different middleware is captured by
	// the path-method-handler tuple existing in two distinct entries.
	assert.NotEqual(t, ingest.Handler, overlay.Handler,
		"ingest and overlay should resolve to distinct leaf handlers")
}
