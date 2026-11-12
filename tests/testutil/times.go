package testutil

import (
	"fmt"
	"time"
)

// StoredWall returns the stored form of a wall time in zone, as the app
// writes it since the #141 conversion: UTC, "2006-01-02 15:04:05". wall is
// "2006-01-02", "2006-01-02T15:04" or "2006-01-02 15:04:05". Fixtures that
// insert wall-time columns (status log, activity, measurement dates) use it
// so their literals keep meaning local time.
func StoredWall(zone, wall string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		panic(err)
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, wall, loc); err == nil {
			return t.UTC().Format("2006-01-02 15:04:05")
		}
	}
	panic(fmt.Sprintf("StoredWall: can't read %q", wall))
}
