package handlers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/handlers"
	"isley/tests/testutil"
)

// Key times come from the database's stamps (UTC on SQLite) and leave in
// the instance zone with their offset, so the settings page doesn't have
// to guess the zone.
func TestListAPIKeys_TimesInInstanceZone(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	testutil.SeedAPIKey(t, db, "list-zone-key-000000000000000000000000")
	testutil.MustExec(t, db, `UPDATE api_keys SET create_dt = '2026-11-11 03:00:00', last_used = NULL`)

	zones := chrono.NewZones(func() string { return "America/New_York" }, time.UTC)
	keys, err := handlers.ListAPIKeys(db, zones)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "2026-11-10T22:00:00-05:00", keys[0].Created)
	assert.Empty(t, keys[0].LastUsed, "never used")
}

// A verified key is reused without re-reading api_keys until the cache is
// cleared.
func TestVerifyAPIKey_CachesSuccessfulChecks(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	const plaintext = "cache-check-key-0000000000000000000000"
	testutil.SeedAPIKey(t, db, plaintext)

	key, err := handlers.VerifyAPIKey(db, plaintext)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Change the stored hash behind the cache's back.
	testutil.MustExec(t, db, `UPDATE api_keys SET key_hash = 'not-a-hash'`)
	key, err = handlers.VerifyAPIKey(db, plaintext)
	require.NoError(t, err)
	assert.NotNil(t, key, "served from the cache")

	handlers.ForgetVerifiedKeys(db)
	key, err = handlers.VerifyAPIKey(db, plaintext)
	require.NoError(t, err)
	assert.Nil(t, key, "checked again after the cache is cleared")

	bad, err := handlers.VerifyAPIKey(db, "wrong-key-000000000000000000000000000")
	require.NoError(t, err)
	assert.Nil(t, bad)
}
