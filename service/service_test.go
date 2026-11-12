package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

// newServices builds Services over db with a fixed clock and the store's
// timezone setting as the instance zone.
func newServices(t *testing.T, db *sql.DB, dialect service.Dialect, store *config.Store, clock chrono.Clock) *service.Services {
	t.Helper()
	svc, err := service.New(service.Deps{
		Version: "9.9.9-test",
		DB:      db,
		Dialect: dialect,
		Clock:   clock,
		Zones:   chrono.NewUTCZones(store.Timezone),
		Store:   store,
	})
	require.NoError(t, err)
	return svc
}

func TestNew_RequiresDeps(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	full := service.Deps{
		DB: db, Dialect: service.SQLite, Clock: chrono.System(),
		Zones: chrono.NewZones(nil, nil), Store: config.NewStore(),
	}
	_, err := service.New(full)
	require.NoError(t, err)

	for name, mutate := range map[string]func(*service.Deps){
		"DB":      func(d *service.Deps) { d.DB = nil },
		"Dialect": func(d *service.Deps) { d.Dialect = "" },
		"Clock":   func(d *service.Deps) { d.Clock = nil },
		"Zones":   func(d *service.Deps) { d.Zones = nil },
		"Store":   func(d *service.Deps) { d.Store = nil },
	} {
		d := full
		mutate(&d)
		_, err := service.New(d)
		assert.ErrorContains(t, err, name)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	nf := service.NotFound("plant_not_found", "plant 12 does not exist")
	wrapped := fmt.Errorf("loading plant: %w", nf)

	got, ok := service.AsError(wrapped)
	require.True(t, ok)
	assert.Equal(t, "plant_not_found", got.Code)
	assert.True(t, service.IsKind(wrapped, service.KindNotFound))
	assert.False(t, service.IsKind(wrapped, service.KindConflict))
	assert.False(t, service.IsKind(errors.New("boom"), service.KindNotFound))
	_, ok = service.AsError(errors.New("boom"))
	assert.False(t, ok)

	assert.Equal(t, service.KindConflict, service.Conflict("name_taken", "x").Kind)
	assert.Equal(t, "not_found", service.KindNotFound.String())
	assert.Equal(t, "invalid", service.KindInvalid.String())
	assert.Equal(t, "conflict", service.KindConflict.String())
}

func TestValidation(t *testing.T) {
	t.Parallel()
	var v service.Validation
	assert.NoError(t, v.Err())

	v.Add("name", "is required")
	v.Add("zone_id", "does not exist")
	v.Add("name", "second problem is ignored")
	err := v.Err()
	require.Error(t, err)

	e, ok := service.AsError(err)
	require.True(t, ok)
	assert.Equal(t, service.KindInvalid, e.Kind)
	assert.Equal(t, service.CodeInvalidRequest, e.Code)
	assert.Equal(t, map[string]string{"name": "is required", "zone_id": "does not exist"}, e.Fields)
	assert.Equal(t, "invalid_request: the request has invalid fields (name: is required; zone_id: does not exist)", e.Error())
}

func TestDialect(t *testing.T) {
	t.Parallel()
	d, err := service.ParseDialect("postgres")
	require.NoError(t, err)
	assert.True(t, d.IsPostgres())
	assert.Equal(t, "GREATEST(0, seed_count - 1)", d.Greatest("0", "seed_count - 1"))

	d, err = service.ParseDialect("sqlite")
	require.NoError(t, err)
	assert.False(t, d.IsPostgres())
	assert.Equal(t, "MAX(0, seed_count - 1)", d.Greatest("0", "seed_count - 1"))

	_, err = service.ParseDialect("mysql")
	assert.Error(t, err)

	assert.Equal(t, "$3, $4, $5", service.Placeholders(3, 3))
	assert.Equal(t, "", service.Placeholders(1, 0))
}

func TestStatusMeanings(t *testing.T) {
	t.Parallel()
	growing := []service.StatusName{service.StatusGerminating, service.StatusPlanted, service.StatusSeedling, service.StatusVeg, service.StatusFlower}
	for _, s := range growing {
		assert.True(t, s.IsGrowing(), s)
		assert.False(t, s.EndsGrowth(), s)
	}
	for _, s := range []service.StatusName{service.StatusDrying, service.StatusCuring, service.StatusSuccess} {
		assert.True(t, s.IsHarvested(), s)
		assert.True(t, s.EndsGrowth(), s)
		assert.False(t, s.IsGrowing(), s)
	}
	assert.True(t, service.StatusDead.IsDead())
	assert.True(t, service.StatusDead.EndsGrowth())
	assert.False(t, service.StatusDead.IsHarvested())
	assert.True(t, service.StatusFlower.IsFlowering())
	assert.False(t, service.StatusVeg.IsFlowering())
}

// The Go status definitions must agree with what the migrations seed:
// every status exists, "growing" is plant_status.active, and the
// lifecycle order is status_order.
func TestStatusMeanings_MatchDatabase(t *testing.T) {
	t.Parallel()
	assertStatusesMatchDatabase(t, testutil.NewTestDB(t))
}

func assertStatusesMatchDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT status, active FROM plant_status ORDER BY status_order`)
	require.NoError(t, err)
	defer rows.Close()

	var names []service.StatusName
	for rows.Next() {
		var name string
		var active bool
		require.NoError(t, rows.Scan(&name, &active))
		s := service.StatusName(name)
		assert.Equal(t, active, s.IsGrowing(), "%s: IsGrowing vs plant_status.active", name)
		names = append(names, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, service.AllStatuses(), names)
}

func TestInstance_Get(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	store := config.NewStore()
	store.SetTimezone("Asia/Kolkata")
	store.SetGuestMode(1)
	store.SetACIEnabled(1)
	store.SetAPIIngestEnabled(0)
	clock := chrono.Fixed(time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC))
	svc := newServices(t, db, service.SQLite, store, clock)

	got, err := svc.Instance.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "9.9.9-test", got.Version)
	assert.Equal(t, "Asia/Kolkata", got.TimeZone)
	assert.Equal(t, "2026-11-11T08:30:00+05:30", got.Now.Format(time.RFC3339))
	assert.True(t, got.GuestMode)
	assert.True(t, got.Features.ACInfinity)
	assert.False(t, got.Features.SensorIngest)
	assert.False(t, got.Features.EcoWitt)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.Instance.Get(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSystemZone_SQLiteIsUTC(t *testing.T) {
	t.Parallel()
	loc, err := chrono.SystemZone(context.Background(), testutil.NewTestDB(t), false)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, loc)
}
