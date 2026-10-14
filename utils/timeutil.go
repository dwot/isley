package utils

import (
	"fmt"
	"time"
)

const (
	LayoutDate          = "2006-01-02"          // HTML date input, ISO date
	LayoutDateTime      = "01/02/2006 03:04 PM" // Human-readable display
	LayoutDateTimeLocal = "2006-01-02T15:04:05" // HTML datetime-local input
	LayoutDB            = "2006-01-02 15:04:05" // Raw DB string queries
)

// IsZeroDate reports whether t is the zero/null sentinel (zero value or 1970-01-01).
func IsZeroDate(t time.Time) bool {
	return t.IsZero() || t.Year() == 1970
}

// AsLocal reinterprets a time.Time whose wall-clock values represent local
// time but whose timezone tag may be wrong (e.g. UTC or a zero-offset
// FixedZone from the SQL driver) as the server's local timezone.
//
// All dates in the database are stored as naive local-time strings.  SQL
// drivers (lib/pq, modernc/sqlite) parse these into time.Time tagged with
// UTC or an equivalent zero-offset zone.  This function preserves the
// wall-clock digits and re-tags with time.Local so that JSON serialisation
// emits the correct offset and template formatting works as expected.
//
// Safe to call unconditionally: when the input is already in time.Local the
// wall-clock values are unchanged, producing an identical time.Time.
func AsLocal(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
}

// layoutDateTimeLocalNoSeconds matches the value browsers actually emit for
// <input type="datetime-local"> when the field has no seconds step: seconds
// are omitted. LayoutDateTimeLocal (which requires them) is kept as-is
// because it mirrors utils.ValidateDate's layout list exactly — changing a
// shared validator used across the app is out of scope here. Accepting this
// additional, seconds-less layout only widens what ParseFlexibleDate parses
// beyond what ValidateDate validates, which preserves the property that
// matters: anything that validates, parses.
const layoutDateTimeLocalNoSeconds = "2006-01-02T15:04"

// ParseFlexibleDate parses the date formats the UI submits: a plain date from
// <input type="date">, and the datetime-local variant used elsewhere in the
// app. Returns an error when none matches.
func ParseFlexibleDate(s string) (time.Time, error) {
	for _, layout := range []string{LayoutDate, LayoutDateTimeLocal, layoutDateTimeLocalNoSeconds, LayoutDB, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date format: %q", s)
}
