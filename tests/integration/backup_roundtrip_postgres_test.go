//go:build integration_postgres

package integration

import (
	"testing"

	"isley/tests/testutil"
)

// TestPostgresBackup_RoundTrip runs the HTTP backup → restore round trip
// (runRestore's batched Postgres load, TRUNCATE ... CASCADE, the hourly
// rollup) against Postgres.
func TestPostgresBackup_RoundTrip(t *testing.T) {
	t.Parallel()
	backupRoundTrip(t, testutil.NewTestPostgresDB(t))
}
