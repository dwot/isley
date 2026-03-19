package utils

import "time"

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
