package chrono

import (
	"fmt"
	"strings"
	"time"
)

// WallLayout is how Zones writes a wall time to storage. It matches the
// "T"-separated strings the web UI has always stored, so new and old rows
// sort together as text in SQLite.
const WallLayout = "2006-01-02T15:04:05"

// inputLayouts are the zone-less forms ParseInput accepts, longest first.
var inputLayouts = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
}

// Zones converts between stored values and instants in the instance zone.
// See the package documentation for the kinds of stored value.
//
// It is safe for concurrent use. The instance zone is looked up on every
// call, so a change to the timezone setting applies at once.
type Zones struct {
	instanceTZ func() string
	system     *time.Location
	// utc is set once the database has been converted (#141 Phase 2):
	// wall times are then stored in UTC too.
	utc bool
}

// NewZones builds Zones.
//
// instanceTZ returns the timezone setting (an IANA name, or "" to use the
// server's zone); it is called on every conversion. system is the zone the
// database stamps CURRENT_TIMESTAMP in: UTC for SQLite, the session zone
// for Postgres (see SystemZone).
func NewZones(instanceTZ func() string, system *time.Location) *Zones {
	if instanceTZ == nil {
		instanceTZ = func() string { return "" }
	}
	if system == nil {
		system = time.UTC
	}
	return &Zones{instanceTZ: instanceTZ, system: system}
}

// NewUTCZones builds Zones for a database that stores every instant in
// UTC: wall times are written and read as UTC, and the database stamps
// CURRENT_TIMESTAMP in UTC.
func NewUTCZones(instanceTZ func() string) *Zones {
	z := NewZones(instanceTZ, time.UTC)
	z.utc = true
	return z
}

// StoresUTC reports whether wall times are stored in UTC.
func (z *Zones) StoresUTC() bool { return z.utc }

// wall is the zone wall-time columns are stored in.
func (z *Zones) wall() *time.Location {
	if z.utc {
		return time.UTC
	}
	return z.Instance()
}

// Instance returns the instance zone: the timezone setting, or the
// server's zone when the setting is empty or not a known zone.
func (z *Zones) Instance() *time.Location {
	if name := z.instanceTZ(); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.Local
}

// System returns the zone the database stamps CURRENT_TIMESTAMP in.
func (z *Zones) System() *time.Location { return z.system }

// reinterpret keeps t's wall-clock digits and gives them loc. SQL drivers
// return naive timestamps tagged UTC (or a zero-offset zone); the digits
// are what was stored.
func reinterpret(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
}

// FromWall turns a stored wall time into an instant in the instance zone.
// The zero time stays zero.
func (z *Zones) FromWall(stored time.Time) time.Time {
	if stored.IsZero() {
		return stored
	}
	return reinterpret(stored, z.wall()).In(z.Instance())
}

// FromSystem turns a stored system time (CURRENT_TIMESTAMP) into an
// instant in the instance zone. The zero time stays zero.
func (z *Zones) FromSystem(stored time.Time) time.Time {
	if stored.IsZero() {
		return stored
	}
	return reinterpret(stored, z.system).In(z.Instance())
}

// ToWall returns the stored form of an instant for a wall-time column.
func (z *Zones) ToWall(t time.Time) string {
	if z.utc {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	return t.In(z.Instance()).Format(WallLayout)
}

// ToSystem returns the stored form of an instant for a system-time column,
// for queries that compare against CURRENT_TIMESTAMP values ("readings
// since t").
func (z *Zones) ToSystem(t time.Time) string {
	return t.In(z.system).Format("2006-01-02 15:04:05")
}

// Now returns the clock's time in the instance zone.
func (z *Zones) Now(c Clock) time.Time { return c.Now().In(z.Instance()) }

// Today returns the current calendar day in the instance zone.
func (z *Zones) Today(c Clock) Date { return DateOf(c.Now(), z.Instance()) }

// DateOf returns the calendar day of t in the instance zone.
func (z *Zones) DateOf(t time.Time) Date { return DateOf(t, z.Instance()) }

// ParseInput reads a date-time sent by a client. RFC 3339 with an offset
// ("2026-11-11T11:45:00-05:00" or "...Z") is taken as given. A value with
// no offset ("2026-11-11T11:45", with or without seconds, "T" or space) is
// read in the instance zone. A bare date is rejected: use ParseDate for
// calendar-day fields.
func (z *Zones) ParseInput(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	loc := z.Instance()
	for _, layout := range inputLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date-time %q: want RFC 3339, e.g. 2026-11-11T11:45:00-05:00", s)
}
