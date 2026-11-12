package service

import "fmt"

// Dialect is the SQL database Isley is running on. Services take it as a
// value from Deps, so two engines in one process (tests) can use different
// databases.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// ParseDialect reads the driver name used by model ("sqlite" or
// "postgres").
func ParseDialect(driver string) (Dialect, error) {
	switch Dialect(driver) {
	case SQLite, Postgres:
		return Dialect(driver), nil
	default:
		return "", fmt.Errorf("unknown database driver %q", driver)
	}
}

// IsPostgres reports whether d is Postgres.
func (d Dialect) IsPostgres() bool { return d == Postgres }

// Greatest returns the SQL for the larger of two expressions: GREATEST on
// Postgres, the two-argument MAX scalar on SQLite.
func (d Dialect) Greatest(a, b string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("GREATEST(%s, %s)", a, b)
	}
	return fmt.Sprintf("MAX(%s, %s)", a, b)
}

// DayOf returns the SQL for the calendar day (YYYY-MM-DD) of a stored
// wall-time column, comparable with a 'YYYY-MM-DD' parameter. SQLite text
// dates come in several layouts but all start with the day.
func (d Dialect) DayOf(col string) string {
	if d.IsPostgres() {
		return fmt.Sprintf("CAST(%s AS DATE)", col)
	}
	return fmt.Sprintf("substr(%s, 1, 10)", col)
}

// Placeholders returns "$start, $start+1, ..." for n parameters. Both
// dialects accept $N placeholders.
func Placeholders(start, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("$%d", start+i)
	}
	return out
}
