// +parallel:serial: Seed mutates the package-level rng and
// plantNameCounters vars (reset at the top of each call, not
// synchronized) -- two Seed calls racing across parallel tests would
// race on that state.

package main

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestSeed is a smoke test, not a correctness check of the random
// distribution -- it exists to catch "the data Seed writes doesn't match
// what the app's real read paths expect," which is the exact bug class
// this fixture kept hitting in practice (a date encoding the driver
// wrote but julianday() couldn't parse, and a missing Germinating/
// Planted status). A real DB and real migrations are what make this
// catch anything -- asserting against Seed's own output wouldn't.
func TestSeed(t *testing.T) {
	db := testutil.NewTestDB(t)

	require.NoError(t, Seed(db, 3, 12))

	// alreadySeeded should make a second call a no-op instead of
	// erroring or duplicating rows.
	require.NoError(t, Seed(db, 3, 12))

	var plantCount int
	require.NoError(t,
		db.QueryRow(`SELECT COUNT(*) FROM plant`).Scan(&plantCount),
	)
	require.Equal(t, 12, plantCount,
		"second Seed call should not have added more plants")

	// Every plant should have logged a Germinating entry. This is the
	// exact gap that prompted this test: the two earliest lifecycle
	// statuses were missing from the seeder entirely.
	var missingGerminating int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*)
		FROM plant p
		WHERE NOT EXISTS (
			SELECT 1
			FROM plant_status_log l
			JOIN plant_status s ON s.id = l.status_id
			WHERE l.plant_id = p.id
			  AND s.status = 'Germinating'
		)
	`).Scan(&missingGerminating))
	require.Zero(t, missingGerminating,
		"every seeded plant should have a Germinating status log entry")

	// current_week is computed live from start_dt via julianday() in
	// getPlantsByStatus (handlers/plant.go) -- it isn't a stored column.
	// If start_dt isn't written as the plain date string the rest of the
	// app expects, julianday() returns NULL here and that handler's Scan
	// fails in production. Reproducing the same computation directly
	// means a regression in date formatting fails this test instead of
	// only surfacing as a scan error in the server log.
	var nullCurrentWeek int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*)
		FROM plant
		WHERE CAST(
			(julianday('now', 'localtime') - julianday(start_dt)) / 7 + 1
			AS INT
		) IS NULL
	`).Scan(&nullCurrentWeek))
	require.Zero(t, nullCurrentWeek,
		"current_week should compute for every seeded plant")
}

// TestSeed_StrainsHaveCycleTime guards strain.cycle_time, which the app
// uses to estimate harvest dates. It is not part of the strain's required
// columns, so leaving it out of the insert fails silently as a NULL.
func TestSeed_StrainsHaveCycleTime(t *testing.T) {
	db := testutil.NewTestDB(t)

	require.NoError(t, Seed(db, 8, 12))

	var missing int
	require.NoError(t, db.QueryRow(`
		SELECT COUNT(*)
		FROM strain s
		WHERE EXISTS (SELECT 1 FROM plant p WHERE p.strain_id = s.id)
		  AND (s.cycle_time IS NULL OR s.cycle_time <= 0)
	`).Scan(&missing))
	require.Zero(t, missing, "every seeded strain should have a cycle_time")
}

// TestSeed_EveryBreederGetsAStrain proves that every generated breeder
// ends up with at least one strain in the database. Seed resets rng to
// seed 42 and generates breeder names first, so the same call here
// reproduces the names Seed will have used.
func TestSeed_EveryBreederGetsAStrain(t *testing.T) {
	db := testutil.NewTestDB(t)

	const strainCount = 10

	require.NoError(t, Seed(db, strainCount, 12))

	breeders := generateBreederNames(
		rand.New(rand.NewSource(42)),
		strainCount,
	)

	for _, breeder := range breeders {
		var strains int
		require.NoError(t, db.QueryRow(`
			SELECT COUNT(*)
			FROM strain s
			JOIN breeder b ON b.id = s.breeder_id
			WHERE b.name = $1
		`, breeder).Scan(&strains))

		require.GreaterOrEqual(t, strains, 1,
			"breeder %q should have at least one strain", breeder)
	}
}

func TestSeed_RejectsNonPositiveStrainCount(t *testing.T) {
	db := testutil.NewTestDB(t)

	require.ErrorContains(t, Seed(db, 0, 1), "strainCount")
}
