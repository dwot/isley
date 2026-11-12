//go:build integration_postgres

package model

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

func TestPostgresMigration025_PlantSensorLinks(t *testing.T) {
	t.Parallel()
	db := freshPostgresDB(t, false)
	src, err := iofs.New(MigrationsFS, "migrations/postgres")
	require.NoError(t, err)
	drv, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(24))

	seedPlantSensorLinks(t, db)
	require.NoError(t, m.Migrate(25))
	assertPlantSensorLinks(t, db)

	require.NoError(t, m.Migrate(24))
	var links string
	require.NoError(t, db.QueryRow(`SELECT sensors FROM plant WHERE name = 'A'`).Scan(&links))
	require.Equal(t, "[2, 1]", links)
	require.NoError(t, m.Migrate(25))
	assertPlantSensorLinks(t, db)
}
