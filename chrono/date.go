package chrono

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// DateLayout is the wire and storage form of a Date.
const DateLayout = "2006-01-02"

// Date is a calendar day with no time of day and no zone: a plant's start
// day, a harvest day. It is the same day wherever you are, so it is never
// converted between zones. The zero Date means "not set".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar day of t in loc.
func DateOf(t time.Time, loc *time.Location) Date {
	y, m, d := t.In(loc).Date()
	return Date{y, m, d}
}

// ParseDate reads "YYYY-MM-DD".
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date %q: want YYYY-MM-DD", s)
	}
	return Date{t.Year(), t.Month(), t.Day()}, nil
}

// IsZero reports whether d is unset.
func (d Date) IsZero() bool { return d == Date{} }

// String returns "YYYY-MM-DD", or "" for the zero Date.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// midnightUTC is d as an instant, for day arithmetic only. Using UTC keeps
// every day exactly 24 hours long (no DST).
func (d Date) midnightUTC() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// DaysSince returns the whole days from earlier to d (negative if earlier
// is after d).
func (d Date) DaysSince(earlier Date) int {
	return int(d.midnightUTC().Sub(earlier.midnightUTC()).Hours() / 24)
}

// AddDays returns d moved by n days.
func (d Date) AddDays(n int) Date {
	t := d.midnightUTC().AddDate(0, 0, n)
	return Date{t.Year(), t.Month(), t.Day()}
}

// Before reports whether d is earlier than other.
func (d Date) Before(other Date) bool { return d.midnightUTC().Before(other.midnightUTC()) }

// MarshalJSON writes "YYYY-MM-DD", or null for the zero Date.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON reads "YYYY-MM-DD" or null.
func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*d = Date{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("date must be a string: %w", err)
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Value stores a Date as "YYYY-MM-DD" (NULL when zero). Both SQLite and
// Postgres accept that for DATE and TIMESTAMP columns.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

// Scan reads a calendar day from a DATE/TIMESTAMP column or a string. A
// stored time of day is dropped: the stored digits are the day, whatever
// zone the driver tags them with.
func (d *Date) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		*d = Date{}
		return nil
	case time.Time:
		*d = Date{v.Year(), v.Month(), v.Day()}
		return nil
	case string:
		return d.scanString(v)
	case []byte:
		return d.scanString(string(v))
	default:
		return fmt.Errorf("chrono.Date: cannot scan %T", src)
	}
}

func (d *Date) scanString(s string) error {
	if s == "" {
		*d = Date{}
		return nil
	}
	if len(s) < len(DateLayout) {
		return fmt.Errorf("chrono.Date: cannot scan %q", s)
	}
	parsed, err := ParseDate(s[:len(DateLayout)])
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
