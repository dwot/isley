//go:build integration_postgres

package mcpserver_test

import (
	"testing"

	"isley/service"
	"isley/tests/testutil"
)

func newPostgresFixture(t *testing.T) fixture {
	t.Helper()
	return newFixtureOn(t, testutil.NewTestPostgresDB(t), testutil.WithDialect(service.Postgres))
}

func TestPostgresMCP_ReadKey(t *testing.T) {
	t.Parallel()
	checkReadKey(t, newPostgresFixture(t))
}

func TestPostgresMCP_WriteKey(t *testing.T) {
	t.Parallel()
	checkWriteKey(t, newPostgresFixture(t))
}
