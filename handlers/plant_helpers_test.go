package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"isley/model/types"
)

// ---------------------------------------------------------------------------
// deriveActivityDates
// ---------------------------------------------------------------------------

func TestDeriveActivityDates_Empty(t *testing.T) {
	t.Parallel()

	water, feed := deriveActivityDates(nil)
	assert.True(t, water.IsZero(), "no activities → zero water date")
	assert.True(t, feed.IsZero(), "no activities → zero feed date")
}

func TestDeriveActivityDates_LatestWins(t *testing.T) {
	t.Parallel()

	d1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	activities := []types.PlantActivity{
		{ActivityId: ActivityWater, Date: d1},
		{ActivityId: ActivityWater, Date: d3},
		{ActivityId: ActivityWater, Date: d2},
		{ActivityId: ActivityFeed, Date: d2},
		{ActivityId: ActivityFeed, Date: d1},
	}

	water, feed := deriveActivityDates(activities)
	assert.Equal(t, d3, water, "latest water date should win")
	assert.Equal(t, d2, feed, "latest feed date should win")
}

func TestDeriveActivityDates_IgnoresOtherActivities(t *testing.T) {
	t.Parallel()

	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// ID 99 is neither water (1) nor feed (2)
	activities := []types.PlantActivity{
		{ActivityId: 99, Date: d},
	}
	water, feed := deriveActivityDates(activities)
	assert.True(t, water.IsZero())
	assert.True(t, feed.IsZero())
}

// ---------------------------------------------------------------------------
// deriveHarvestDate
// ---------------------------------------------------------------------------

func TestDeriveHarvestDate_NoTerminalStatus(t *testing.T) {
	t.Parallel()

	// With no Success/Dead/Curing/Drying entries the function returns
	// the current time (a sentinel for "not yet harvested"). We assert
	// the value is recent rather than equal to a specific timestamp.
	before := time.Now()
	got := deriveHarvestDate([]types.Status{
		{Status: "Vegetative", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	})
	after := time.Now()

	assert.False(t, got.Before(before), "result should be at or after pre-call time")
	assert.False(t, got.After(after), "result should be at or before post-call time")
}

func TestDeriveHarvestDate_PicksEarliestTerminal(t *testing.T) {
	t.Parallel()

	// All dates are in the past so they always satisfy
	// `s.Date.Before(time.Now())` regardless of when the test runs.
	// The function uses time.Now() as the upper bound — see
	// TestDeriveHarvestDate_NoTerminalStatus for that branch.
	d1 := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		history  []types.Status
		expected time.Time
	}{
		{
			"single Success",
			[]types.Status{{Status: "Success", Date: d2}},
			d2,
		},
		{
			"earliest of multiple terminals wins",
			[]types.Status{
				{Status: "Curing", Date: d3},
				{Status: "Drying", Date: d1},
				{Status: "Success", Date: d2},
			},
			d1,
		},
		{
			"non-terminal statuses are ignored — only Dead counts",
			[]types.Status{
				{Status: "Vegetative", Date: d1},
				{Status: "Flower", Date: d2},
				{Status: "Dead", Date: d3},
			},
			d3,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, deriveHarvestDate(tc.history))
		})
	}
}

// ---------------------------------------------------------------------------
// deriveEstimatedHarvestDate
// ---------------------------------------------------------------------------

func TestDeriveEstimatedHarvestDate_CycleTimeZero(t *testing.T) {
	t.Parallel()

	got := deriveEstimatedHarvestDate(nil, time.Now(), 0, true)
	assert.True(t, got.IsZero(), "cycleTime=0 must produce zero time")
}

func TestDeriveEstimatedHarvestDate_Autoflower(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := deriveEstimatedHarvestDate(nil, start, 70, true)
	assert.Equal(t, start.AddDate(0, 0, 70), got)
}

func TestDeriveEstimatedHarvestDate_PhotosensitiveNoFlower(t *testing.T) {
	t.Parallel()

	got := deriveEstimatedHarvestDate(
		[]types.Status{{Status: "Vegetative", Date: time.Now()}},
		time.Now(), 56, false,
	)
	assert.True(t, got.IsZero(), "photosensitive without Flower status returns zero")
}

func TestDeriveEstimatedHarvestDate_PhotosensitiveWithFlower(t *testing.T) {
	t.Parallel()

	flower := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	earlierFlower := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	got := deriveEstimatedHarvestDate(
		[]types.Status{
			{Status: "Vegetative", Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			{Status: "Flower", Date: flower},
			{Status: "Flower", Date: earlierFlower},
		},
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		56,
		false,
	)
	// Earliest "Flower" entry should be the anchor.
	assert.Equal(t, earlierFlower.AddDate(0, 0, 56), got)
}
