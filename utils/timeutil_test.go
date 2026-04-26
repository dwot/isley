package utils

// Phase 6a tests for timeutil.go.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsZeroDate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   time.Time
		want bool
	}{
		{"zero value", time.Time{}, true},
		{"epoch UTC (1970)", time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"early 1970 still flagged", time.Date(1970, 6, 15, 12, 30, 0, 0, time.UTC), true},
		{"late 1969 NOT flagged", time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC), false},
		{"normal date", time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, IsZeroDate(tc.in))
		})
	}
}

// TestAsLocal_PreservesWallClockDigits confirms AsLocal keeps the
// wall-clock components verbatim while swapping the location to
// time.Local. SQL drivers tag DB strings with UTC even though they
// represent local-time digits; this function is what bridges that gap.
func TestAsLocal_PreservesWallClockDigits(t *testing.T) {
	t.Parallel()

	// A time tagged as UTC, intended to be interpreted as local.
	source := time.Date(2026, 4, 25, 14, 30, 45, 12345, time.UTC)
	got := AsLocal(source)

	assert.Equal(t, time.Local, got.Location(), "tag must be flipped to time.Local")
	assert.Equal(t, source.Year(), got.Year())
	assert.Equal(t, source.Month(), got.Month())
	assert.Equal(t, source.Day(), got.Day())
	assert.Equal(t, source.Hour(), got.Hour())
	assert.Equal(t, source.Minute(), got.Minute())
	assert.Equal(t, source.Second(), got.Second())
	assert.Equal(t, source.Nanosecond(), got.Nanosecond())
}

func TestAsLocal_AlreadyLocalIsNoOp(t *testing.T) {
	t.Parallel()

	source := time.Date(2026, 4, 25, 14, 30, 45, 0, time.Local)
	got := AsLocal(source)
	assert.True(t, source.Equal(got),
		"input already tagged time.Local should produce an equal time")
	assert.Equal(t, time.Local, got.Location())
}

// TestLayoutConstants_AreParseable acts as a smoke check on the
// formatting constants — all four must be parseable by time.Parse.
func TestLayoutConstants_AreParseable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		layout string
		input  string
	}{
		{LayoutDate, "2026-04-25"},
		{LayoutDateTime, "04/25/2026 02:30 PM"},
		{LayoutDateTimeLocal, "2026-04-25T14:30:45"},
		{LayoutDB, "2026-04-25 14:30:45"},
	}
	for _, tc := range cases {
		t.Run(tc.layout, func(t *testing.T) {
			t.Parallel()
			parsed, err := time.Parse(tc.layout, tc.input)
			if assert.NoError(t, err) {
				assert.False(t, parsed.IsZero())
			}
		})
	}
}
