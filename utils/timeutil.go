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

// AsLocal reinterprets a time.Time that was incorrectly tagged as UTC
// (because the SQL driver parsed a naive datetime string) as local time.
// This preserves the wall-clock values (year, month, day, hour, minute, second)
// while changing only the timezone from UTC to the server's local zone.
// If t is already in a non-UTC zone, it is returned unchanged.
func AsLocal(t time.Time) time.Time {
	if t.Location() == time.UTC {
		return time.Date(t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
	}
	return t
}
