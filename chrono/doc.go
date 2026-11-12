// Package chrono is the one place Isley converts between stored date/time
// values and the instance's time zone, and the one place it asks what time
// it is.
//
// # Why
//
// Stored dates come in three kinds:
//
//   - Wall times: when something happened, entered by a person (activity,
//     measurement, status change). Before the UTC migration (#141 Phase 2)
//     they were naive strings in the instance's wall time, e.g.
//     "2026-11-10T21:30"; after it they are UTC ("2026-11-11 02:30:00").
//   - System times: stamped by the database (CURRENT_TIMESTAMP): sensor
//     readings, create_dt/update_dt, last_used. SQLite always writes UTC;
//     Postgres writes the session zone, which Isley pins to UTC.
//   - Calendar days: a date with no time or zone: a plant's start day, an
//     image's day, a smoke report's day. See Date. These never shift.
//
// Zones knows how each kind is stored and converts to and from real
// instants in the instance zone. Code outside this package must not call
// time.Local or .Local() on stored values (boundary_lint_test.go checks);
// it goes through Zones. NewZones reads the old storage, NewUTCZones the
// converted one.
//
// # Instance zone
//
// The instance zone is the timezone setting (Settings → General), or the
// server's zone when the setting is empty. It is what API responses use
// for offsets and what "today" means.
package chrono
