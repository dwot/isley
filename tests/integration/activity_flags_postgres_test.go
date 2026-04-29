//go:build integration_postgres

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestPostgres_ActivityFlags_MigrationFlagsBuiltins is the Postgres
// counterpart to TestActivityFlags_MigrationFlagsBuiltins. The flag
// columns and the backfill UPDATE statements are duplicated across the
// sqlite/ and postgres/ migration trees — keeping coverage on both
// catches the easy mistake of editing one and forgetting the other.
func TestPostgres_ActivityFlags_MigrationFlagsBuiltins(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestPostgresDB(t)

	cases := []struct {
		name        string
		wantWater   bool
		wantFeeding bool
	}{
		{"Water", true, false},
		{"Feed", false, true},
		{"Note", false, false},
	}
	for _, tc := range cases {
		var isWatering, isFeeding bool
		require.NoErrorf(t,
			db.QueryRow(`SELECT is_watering, is_feeding FROM activity WHERE name = $1`, tc.name).
				Scan(&isWatering, &isFeeding),
			"built-in activity %q must exist after migrations", tc.name)
		assert.Equalf(t, tc.wantWater, isWatering, "is_watering for %q", tc.name)
		assert.Equalf(t, tc.wantFeeding, isFeeding, "is_feeding for %q", tc.name)
	}
}
