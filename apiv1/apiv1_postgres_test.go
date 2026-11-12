//go:build integration_postgres

package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

// The engine starts on Postgres (system-zone detection, dialect) and the
// API answers in contract.
func TestPostgresInstance(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestPostgresDB(t)
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	server := testutil.NewTestServer(t, db,
		testutil.WithConfigStore(store),
		testutil.WithUploadDir(t.TempDir()),
		testutil.WithDialect(service.Postgres),
		testutil.WithClock(chrono.Fixed(fixedNow)))
	cc := newContractClient(t, server)
	key := testutil.SeedAPIKeyWithScopes(t, db, "v1-postgres-read-000000000000000", "read")

	resp := cc.get("/instance", key)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	require.Contains(t, string(resp.Body), `"now":"2026-11-10T22:00:00-05:00"`)
}

func newPostgresFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.NewTestPostgresDB(t)
	store := config.NewStore()
	store.SetTimezone("America/New_York")
	server := testutil.NewTestServer(t, db,
		testutil.WithConfigStore(store),
		testutil.WithUploadDir(t.TempDir()),
		testutil.WithDialect(service.Postgres),
		testutil.WithClock(chrono.Fixed(fixedNow)))
	return fixture{server: server, cc: newContractClient(t, server), store: store}
}

func TestPostgresLookups(t *testing.T) {
	t.Parallel()
	checkLookups(t, newPostgresFixture(t))
}

// The reading timestamps assume the test database's session zone is UTC,
// which it is for the stock postgres image CI uses.
func TestPostgresZonesRead(t *testing.T) {
	t.Parallel()
	checkZonesRead(t, newPostgresFixture(t))
}

func TestPostgresZonesWrite(t *testing.T) {
	t.Parallel()
	checkZonesWrite(t, newPostgresFixture(t))
}

func TestPostgresPlantsRead(t *testing.T) {
	t.Parallel()
	checkPlantsRead(t, newPostgresFixture(t))
}

func TestPostgresPlantsWrite(t *testing.T) {
	t.Parallel()
	checkPlantsWrite(t, newPostgresFixture(t))
}

func TestPostgresActivities(t *testing.T) {
	t.Parallel()
	checkActivities(t, newPostgresFixture(t))
}

func TestPostgresMeasurements(t *testing.T) {
	t.Parallel()
	checkMeasurements(t, newPostgresFixture(t))
}

func TestPostgresIdempotencyKey(t *testing.T) {
	t.Parallel()
	checkIdempotencyKey(t, newPostgresFixture(t))
}

func TestPostgresImages(t *testing.T) {
	t.Parallel()
	checkImages(t, newPostgresFixture(t))
}

func TestPostgresBreedersAPI(t *testing.T) {
	t.Parallel()
	checkBreedersAPI(t, newPostgresFixture(t))
}

func TestPostgresStrainsAPI(t *testing.T) {
	t.Parallel()
	checkStrainsAPI(t, newPostgresFixture(t))
}

func TestPostgresStrainsAPI_Feminized(t *testing.T) {
	t.Parallel()
	checkStrainsAPIFeminized(t, newPostgresFixture(t))
}

func TestPostgresSensorsAPI(t *testing.T) {
	t.Parallel()
	checkSensorsAPI(t, newPostgresFixture(t))
}

func TestPostgresSensorDataAPI(t *testing.T) {
	t.Parallel()
	checkSensorDataAPI(t, newPostgresFixture(t))
}

func TestPostgresStreamsAPI(t *testing.T) {
	t.Parallel()
	checkStreamsAPI(t, newPostgresFixture(t))
}

func TestPostgresSmokeReportsAPI(t *testing.T) {
	t.Parallel()
	checkSmokeReportsAPI(t, newPostgresFixture(t))
}
