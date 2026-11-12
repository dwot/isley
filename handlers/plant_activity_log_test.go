package handlers

// The activity log query itself is tested in package service
// (activities_test.go); these cover the export helpers.

import (
	"isley/chrono"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// slugify
// ---------------------------------------------------------------------------

func TestSlugify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"basic", "Plant One", "plant-one"},
		{"mixed punctuation", "Plant!#1.A", "plant-1-a"},
		{"trims dashes", "  --foo--bar--  ", "foo-bar"},
		{"unicode is dashed", "Café", "caf"}, // non-ASCII letters dashed-out
		{"empty input → fallback", "", "plant"},
		{"only punctuation → fallback", "!!!", "plant"},
		{"underscores collapsed", "a__b__c", "a-b-c"},
		{"already a slug", "ok-name", "ok-name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, slugify(tc.in))
		})
	}
}

// ---------------------------------------------------------------------------
// formatActivityDate (timezone-aware)
// ---------------------------------------------------------------------------

func TestFormatActivityDate_InInstanceZone(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 4, 25, 14, 30, 0, 0, time.UTC)
	ny := chrono.NewZones(func() string { return "America/New_York" }, time.UTC)
	utc := chrono.NewZones(func() string { return "UTC" }, time.UTC)
	// LayoutDateTime is "01/02/2006 03:04 PM" — month/day/year, 12-hour.
	assert.Equal(t, "04/25/2026 10:30 AM", formatActivityDate(stamp, ny))
	assert.Equal(t, "04/25/2026 02:30 PM", formatActivityDate(stamp, utc))
}

// ---------------------------------------------------------------------------
// cellRef
// ---------------------------------------------------------------------------

func TestCellRef(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "A3", cellRef("A", 3))
	assert.Equal(t, "AB12", cellRef("AB", 12))
	assert.Equal(t, "Z1", cellRef("Z", 1))
}
