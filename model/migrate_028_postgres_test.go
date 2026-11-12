//go:build integration_postgres

package model

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

func TestPostgresMigration028_RepairHourlyRollup(t *testing.T) {
	t.Parallel()
	db := freshPostgresDB(t, false)
	src, err := iofs.New(MigrationsFS, "migrations/postgres")
	require.NoError(t, err)
	drv, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(27))

	seedShortRollup(t, db)
	require.NoError(t, m.Migrate(28))
	assertRollupRepaired(t, db)
	require.NoError(t, m.Migrate(27))
	require.NoError(t, m.Migrate(28))
}
