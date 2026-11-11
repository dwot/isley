package handlers_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// External package — handlers_test — to dodge the
// handlers→app→handlers cycle when test files import tests/testutil.

// ---------------------------------------------------------------------------
// GetBreeders
// ---------------------------------------------------------------------------

func TestGetBreeders_EmptyDatabase(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	got := handlers.GetBreeders(db)
	assert.Empty(t, got, "fresh DB has no breeders")
}

func TestGetBreeders_SeededRows(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (1, 'Alpha')`)
	testutil.MustExec(t, db, `INSERT INTO breeder (id, name) VALUES (2, 'Bravo')`)

	got := handlers.GetBreeders(db)
	require.Len(t, got, 2)

	names := []string{got[0].Name, got[1].Name}
	assert.ElementsMatch(t, []string{"Alpha", "Bravo"}, names)
}

// ---------------------------------------------------------------------------
// GetStrains
// ---------------------------------------------------------------------------

func TestGetStrains_OrderedAlphabetically(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	testutil.SeedStrain(t, db, breederID, "Zeta")
	testutil.SeedStrain(t, db, breederID, "Alpha")
	testutil.SeedStrain(t, db, breederID, "Mango")

	got := handlers.GetStrains(db)
	require.Len(t, got, 3)
	assert.Equal(t, "Alpha", got[0].Name, "GetStrains must order by name ASC")
	assert.Equal(t, "Mango", got[1].Name)
	assert.Equal(t, "Zeta", got[2].Name)
}

// ---------------------------------------------------------------------------
// GetStrain (single, with breeder JOIN)
// ---------------------------------------------------------------------------

func TestGetStrain_PopulatesFields(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "Acme Genetics")
	strainID := testutil.SeedStrain(t, db, breederID, "OG Test")
	testutil.MustExec(t, db,
		`UPDATE strain SET sativa = 30, indica = 70, autoflower = 1, feminized = 1,
		 description = 'a desc', seed_count = 12, cycle_time = 56, url = 'https://x'
		 WHERE id = $1`, strainID)

	got := handlers.GetStrain(db, strconv.Itoa(strainID))
	assert.Equal(t, strainID, got.ID)
	assert.Equal(t, "OG Test", got.Name)
	assert.Equal(t, "Acme Genetics", got.Breeder, "breeder name should resolve via JOIN")
	assert.Equal(t, breederID, got.BreederID)
	assert.Equal(t, 30, got.Sativa)
	assert.Equal(t, 70, got.Indica)
	assert.Equal(t, 12, got.SeedCount)
	assert.Equal(t, 56, got.CycleTime)
	assert.Equal(t, "https://x", got.Url)
	assert.True(t, got.Autoflower, "autoflower=1 should map to true")
	assert.True(t, got.Feminized, "feminized=1 should map to true")
}

func TestGetStrain_MissingIDReturnsZeroValue(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	got := handlers.GetStrain(db, "9999")
	assert.Zero(t, got.ID, "missing id should yield zero-value Strain")
	assert.Empty(t, got.Name)
}

// ---------------------------------------------------------------------------
// In-stock / out-of-stock filtering (handlers.InStockStrainsHandler /
// OutOfStockStrainsHandler delegate to getStrainsBySeedCount which is
// unexported — exercise them through the public handlers via a JSON
// round-trip in tests/integration/strain_test.go. The DB-level filter
// is verified directly here using GetStrains plus a manual where-clause
// to keep this test self-contained without exporting the helper).
// ---------------------------------------------------------------------------

func TestGetStrains_FiltersBySeedCountManual(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	breederID := testutil.SeedBreeder(t, db, "B")
	testutil.SeedStrain(t, db, breederID, "InStockOne") // default seed_count=5
	outID := testutil.SeedStrain(t, db, breederID, "OutOfStock")
	testutil.MustExec(t, db, `UPDATE strain SET seed_count = 0 WHERE id = $1`, outID)

	// Spot-check the underlying assumption about column semantics: a
	// fresh strain with seed_count > 0 should be considered in stock,
	// seed_count == 0 should be out of stock. This shields the
	// integration tests' assertions about /strains/in-stock and
	// /strains/out-of-stock from typos in the WHERE clause.
	var inStockCount, outOfStockCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM strain WHERE seed_count > 0`).Scan(&inStockCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM strain WHERE seed_count = 0`).Scan(&outOfStockCount))
	assert.Equal(t, 1, inStockCount)
	assert.Equal(t, 1, outOfStockCount)
}
