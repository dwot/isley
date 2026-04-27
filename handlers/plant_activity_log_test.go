package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/model"
	"isley/utils"
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

func TestFormatActivityDate_NoConfiguredTimezone(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	// We don't assert the exact string (depends on the host's local
	// zone) but we DO assert the function follows
	// `t.In(time.Local).Format(utils.LayoutDateTime)`.
	want := stamp.In(time.Local).Format(utils.LayoutDateTime)
	assert.Equal(t, want, formatActivityDate(stamp, ""))
}

func TestFormatActivityDate_WithUTCTimezone(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	// LayoutDateTime is "01/02/2006 03:04 PM" — month/day/year, 12-hour.
	assert.Equal(t, "04/25/2026 10:30 AM", formatActivityDate(stamp, "UTC"))
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

// ---------------------------------------------------------------------------
// buildActivityLogQuery — every filter branch
// ---------------------------------------------------------------------------

func TestBuildActivityLogQuery_NoFilters(t *testing.T) {
	t.Parallel()

	q, args := buildActivityLogQuery(ActivityLogFilters{})
	assert.Empty(t, args, "no-filter call should produce no args")
	assert.NotContains(t, q, "WHERE", "no-filter call should not emit WHERE")
	assert.Contains(t, q, "ORDER BY pa.date DESC", "default order is date_desc")
}

func TestBuildActivityLogQuery_PlantIDFilter(t *testing.T) {
	t.Parallel()

	pid := 7
	q, args := buildActivityLogQuery(ActivityLogFilters{PlantID: &pid})
	assert.Contains(t, q, "pa.plant_id = $1")
	assert.Equal(t, []interface{}{7}, args)
}

func TestBuildActivityLogQuery_ActivityIDsFilter(t *testing.T) {
	t.Parallel()

	q, args := buildActivityLogQuery(ActivityLogFilters{ActivityIDs: []int{1, 2, 3}})
	assert.Contains(t, q, "pa.activity_id IN ($1,$2,$3)")
	assert.Equal(t, []interface{}{1, 2, 3}, args)
}

func TestBuildActivityLogQuery_DateRange(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	q, args := buildActivityLogQuery(ActivityLogFilters{From: &from, To: &to})

	assert.Contains(t, q, "pa.date >= $1")
	assert.Contains(t, q, "pa.date <= $2")
	require.Len(t, args, 2)
	assert.Equal(t, from, args[0])
	assert.Equal(t, to, args[1])
}

func TestBuildActivityLogQuery_FreeTextSearch_SQLite(t *testing.T) {
	prevDriver := swapDriver("sqlite")
	t.Cleanup(func() { swapDriver(prevDriver) })

	q, args := buildActivityLogQuery(ActivityLogFilters{Query: "leaf"})
	assert.Contains(t, q, "pa.note LIKE $1")
	assert.NotContains(t, q, "ILIKE", "SQLite branch must not use ILIKE")
	assert.Equal(t, []interface{}{"%leaf%"}, args)
}

func TestBuildActivityLogQuery_FreeTextSearch_Postgres(t *testing.T) {
	prevDriver := swapDriver("postgres")
	t.Cleanup(func() { swapDriver(prevDriver) })

	q, _ := buildActivityLogQuery(ActivityLogFilters{Query: "leaf"})
	assert.Contains(t, q, "pa.note ILIKE", "Postgres branch should ILIKE")
}

func TestBuildActivityLogQuery_OrderVariants(t *testing.T) {
	cases := []struct {
		order, want string
	}{
		{"date_asc", "ORDER BY pa.date ASC, pa.id ASC"},
		{"plant", "ORDER BY p.name ASC"},
		{"activity", "ORDER BY a.name ASC, pa.date DESC"},
		{"", "ORDER BY pa.date DESC, pa.id DESC"}, // fallback
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.order, func(t *testing.T) {
			// SQLite branch: NULLS LAST is stripped from the "plant" order.
			prev := swapDriver("sqlite")
			t.Cleanup(func() { swapDriver(prev) })

			q, _ := buildActivityLogQuery(ActivityLogFilters{Order: tc.order})
			assert.Contains(t, q, tc.want)
			assert.NotContains(t, q, "NULLS LAST",
				"SQLite branch must strip NULLS LAST")
		})
	}
}

func TestBuildActivityLogQuery_CombinedFilters(t *testing.T) {
	t.Parallel()

	pid := 3
	zid := 1
	q, args := buildActivityLogQuery(ActivityLogFilters{
		PlantID:     &pid,
		ActivityIDs: []int{2, 4},
		ZoneID:      &zid,
	})
	// All filters AND-joined; placeholder numbers are allocated in order.
	assert.Contains(t, q, "pa.plant_id = $1")
	assert.Contains(t, q, "pa.activity_id IN ($2,$3)")
	assert.Contains(t, q, "p.zone_id = $4")
	require.Equal(t, []interface{}{3, 2, 4, 1}, args)
	whereCount := strings.Count(q, " AND ")
	assert.Equal(t, 2, whereCount, "three predicates → two ANDs")
}

// ---------------------------------------------------------------------------
// helpers — keep this test file self-contained without exporting more
// than necessary from the production package
// ---------------------------------------------------------------------------

// swapDriver flips model's package-global driver via the test setter
// and returns the prior value. buildActivityLogQuery reads
// model.IsPostgres() which depends on this global.
func swapDriver(next string) string {
	prev := model.GetDriver()
	model.SetDriverForTesting(next)
	return prev
}
