package integration

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/tests/testutil"
)

// TestPlantsPage_UsesConfiguredWeightUnit guards the starting display unit
// for the plant index: the page's weightUnit constant must come from the
// setting so the cards and table start in oz when that is configured.
func TestPlantsPage_UsesConfiguredWeightUnit(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	store := config.NewStore()
	store.SetWeightUnit("oz")
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(store))

	testutil.SeedAdmin(t, db, "weight-unit-pw")
	c := server.LoginAsAdmin(t, "weight-unit-pw")

	resp := c.Get("/plants")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `const weightUnit = "oz";`)
	assert.Contains(t, string(body), `const weightToggleHint = "Click to switch between grams and ounces";`)
}
