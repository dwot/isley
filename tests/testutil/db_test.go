package testutil_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// expectedTables is a small subset of the schema we know must be present
// after migrations run. It is deliberately not exhaustive — exhaustive
// schema introspection lives in model/* tests once the schema-test slice
// in TEST_PLAN.md lands.
var expectedTables = []string{
	"activity",
	"plant",
	"plant_activity",
	"plant_images",
	"plant_measurements",
	"plant_status",
	"plant_status_log",
	"sensor_data",
	"sensors",
	"settings",
	"strain",
	"zones",
}

func TestNewTestDB_AppliesMigrations(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%schema_migrations%'`)
	require.NoError(t, err)
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got = append(got, name)
	}
	require.NoError(t, rows.Err())
	sort.Strings(got)

	for _, want := range expectedTables {
		assert.Containsf(t, got, want, "expected table %q to exist after migrations; got tables: %v", want, got)
	}
}

func TestNewTestDB_IsolatedBetweenCalls(t *testing.T) {
	t.Parallel()

	a := testutil.NewTestDB(t)
	b := testutil.NewTestDB(t)

	_, err := a.Exec(`INSERT INTO settings (name, value) VALUES ('canary', 'a-only')`)
	require.NoError(t, err)

	var count int
	require.NoError(t, b.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'canary'`).Scan(&count))
	assert.Zero(t, count, "rows written to db A must not appear in db B")
}

func TestNewTestDB_ParallelSubtests(t *testing.T) {
	t.Parallel()

	for i := 0; i < 4; i++ {
		i := i
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			db := testutil.NewTestDB(t)

			_, err := db.Exec(`INSERT INTO settings (name, value) VALUES (?, ?)`, "canary", "v")
			require.NoErrorf(t, err, "iteration %d", i)

			var n int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'canary'`).Scan(&n))
			assert.Equal(t, 1, n)
		})
	}
}
