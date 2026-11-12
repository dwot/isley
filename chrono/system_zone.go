package chrono

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SystemZone returns the zone the database stamps CURRENT_TIMESTAMP in.
// SQLite always uses UTC. Postgres uses the session's TimeZone, which is
// the server default unless the connection sets it.
func SystemZone(ctx context.Context, db *sql.DB, postgres bool) (*time.Location, error) {
	if !postgres {
		return time.UTC, nil
	}
	var name string
	if err := db.QueryRowContext(ctx, "SHOW TIME ZONE").Scan(&name); err != nil {
		return nil, fmt.Errorf("read Postgres time zone: %w", err)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("Postgres time zone %q: %w", name, err)
	}
	return loc, nil
}
