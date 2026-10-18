//go:build integration_postgres

package handlers_test

import (
	"testing"

	"isley/tests/testutil"
)

// TestPlantAge_ListAndPageAgreePostgres is the Postgres half of
// TestPlantAge_ListAndPageAgree; #243 was only visible on Postgres.
func TestPlantAge_ListAndPageAgreePostgres(t *testing.T) {
	t.Parallel()
	assertPlantAgeAgrees(t, testutil.NewTestPostgresDB(t))
}
