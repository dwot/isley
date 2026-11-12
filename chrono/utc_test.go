package chrono_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"isley/chrono"
)

// With UTC storage a wall time is written and read as UTC, and comes back
// as the same instant in the instance zone, on either side of a DST change.
func TestUTCZones_WallRoundTrip(t *testing.T) {
	t.Parallel()
	z := chrono.NewUTCZones(func() string { return "America/New_York" })
	ny := z.Instance()
	assert.True(t, z.StoresUTC())

	for _, at := range []time.Time{
		time.Date(2026, 7, 4, 21, 30, 0, 0, ny),   // EDT
		time.Date(2026, 11, 10, 21, 30, 0, 0, ny), // EST
	} {
		stored := z.ToWall(at)
		assert.Equal(t, at.UTC().Format("2006-01-02 15:04:05"), stored)
		parsed, err := time.Parse("2006-01-02 15:04:05", stored)
		assert.NoError(t, err)
		back := z.FromWall(parsed)
		assert.True(t, at.Equal(back), "%s → %s → %s", at, stored, back)
		assert.Equal(t, ny, back.Location())
	}
	assert.Equal(t, "2026-11-11 02:30:00", z.ToWall(time.Date(2026, 11, 10, 21, 30, 0, 0, ny)))
}

// The legacy form keeps the instance zone's wall digits.
func TestZones_LegacyWall(t *testing.T) {
	t.Parallel()
	z := chrono.NewZones(func() string { return "America/New_York" }, time.UTC)
	assert.False(t, z.StoresUTC())
	at := time.Date(2026, 11, 10, 21, 30, 0, 0, z.Instance())
	assert.Equal(t, "2026-11-10T21:30:00", z.ToWall(at))
}
