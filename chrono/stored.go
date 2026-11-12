package chrono

import (
	"fmt"
	"strings"
	"time"
)

// Stored scans a timestamp column as the digits that were stored, whatever
// form the driver hands back: time.Time (Postgres, and SQLite DATETIME
// columns), or text in one of the layouts Isley has written over the years.
// Pass the result to Zones.FromWall or Zones.FromSystem to get an instant.
//
// NULL scans to the zero time; check Valid to tell it from a real value.
type Stored struct {
	Time  time.Time
	Valid bool
}

// storedLayouts are the text forms found in Isley databases: browser input
// (date, datetime-local with and without seconds), CURRENT_TIMESTAMP, and
// time.Time values the SQLite driver wrote as RFC 3339.
var storedLayouts = []string{
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Scan implements sql.Scanner.
func (s *Stored) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s = Stored{}
		return nil
	case time.Time:
		*s = Stored{Time: stripZone(v), Valid: true}
		return nil
	case []byte:
		return s.scanText(string(v))
	case string:
		return s.scanText(v)
	default:
		return fmt.Errorf("chrono.Stored: cannot scan %T", src)
	}
}

func (s *Stored) scanText(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		*s = Stored{}
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, text); err == nil {
		*s = Stored{Time: stripZone(t), Valid: true}
		return nil
	}
	// SQLite's driver may write "2006-01-02 15:04:05.999999999-07:00".
	if t, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", text); err == nil {
		*s = Stored{Time: stripZone(t), Valid: true}
		return nil
	}
	for _, layout := range storedLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			*s = Stored{Time: t, Valid: true}
			return nil
		}
	}
	return fmt.Errorf("chrono.Stored: unrecognised timestamp %q", text)
}

// stripZone keeps t's digits and drops its zone: what was stored is what counts.
func stripZone(t time.Time) time.Time { return reinterpret(t, time.UTC) }
