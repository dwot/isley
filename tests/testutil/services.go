package testutil

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
)

// NewServices builds a service layer over db for tests that call service
// methods or web adapters directly, without an HTTP server. The dialect
// comes from the driver; the clock is the system clock; times are stored
// in UTC, as after the #141 conversion, and the instance zone is UTC. Uploaded files go to a per-test directory.
func NewServices(t *testing.T, db *sql.DB) *service.Services {
	t.Helper()
	dialect := service.SQLite
	if strings.Contains(strings.ToLower(typeName(db)), "pq") {
		dialect = service.Postgres
	}
	svc, err := service.New(service.Deps{
		Version: "test",
		DB:      db,
		Dialect: dialect,
		Clock:   chrono.System(),
		Zones:   chrono.NewUTCZones(func() string { return "UTC" }),
		Store:   config.NewStore(),
		// Image files land in a per-test directory.
		UploadDir: t.TempDir(),
	})
	require.NoError(t, err)
	return svc
}

func typeName(db *sql.DB) string {
	return strings.TrimPrefix(fmtType(db.Driver()), "*")
}

func fmtType(v any) string { return fmt.Sprintf("%T", v) }
