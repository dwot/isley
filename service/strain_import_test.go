package service_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/config"
	"isley/service"
	"isley/tests/testutil"
)

func TestImportCannadb(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkImportCannadb(t, db, service.SQLite)
}

// checkImportCannadb: an import adds the strain, its breeder and its
// parents; importing it again updates them in place.
func checkImportCannadb(t *testing.T, db *sql.DB, dialect service.Dialect) {
	ctx := context.Background()
	store := config.NewStore()
	svc := newServices(t, db, dialect, store, chrono.System())
	// A breeder the user added by hand, and another that will hold the
	// name CannaDB later spells its breeder with.
	mine := testutil.SeedBreeder(t, db, "Ethos Genetics")
	taken := testutil.SeedBreeder(t, db, "Ethos")

	in := service.CannadbStrain{
		URI: "at://did/org.cannadb.strain/rk", IndexedAt: "2026-11-01T00:00:00Z",
		Name: "Mandarin Cookies", IndicaPct: 30, SativaPct: 70, CycleTimeDays: 63,
		Description: "# md", URL: "https://ethos.example/mc",
		Breeder:     service.CannadbBreeder{Name: "ethos genetics", URI: "at://did/org.cannadb.breeder/b", IndexedAt: "2026-10-01"},
		ParentNames: []string{"Mandarin Sunset", " ", "Forum Cut Cookies"},
	}
	s, err := svc.Strains.ImportCannadb(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, "Mandarin Cookies", s.Name)
	assert.Equal(t, mine, s.Breeder.ID, "the hand-added breeder, matched by name and now linked")
	assert.Equal(t, 70, s.SativaPct)
	require.NotNil(t, s.CycleTimeDays)
	assert.Equal(t, 63, *s.CycleTimeDays)
	assert.Equal(t, in.URI, s.CannadbURI)
	var parents []string
	for _, p := range s.Parents {
		parents = append(parents, p.ParentName)
	}
	assert.ElementsMatch(t, []string{"Mandarin Sunset", "Forum Cut Cookies"}, parents)
	var link string
	require.NoError(t, db.QueryRow("SELECT cannadb_uri FROM breeder WHERE id = $1", mine).Scan(&link))
	assert.Equal(t, in.Breeder.URI, link)

	// Again, with changes: same strain, same breeder (by its link), whose
	// new spelling is taken by another breeder, so it keeps its name.
	in.Name, in.SativaPct, in.IndicaPct = "Mandarin Cookies v2", 60, 40
	in.Breeder.Name = "ETHOS"
	in.ParentNames = []string{"Mandarin Sunset"}
	again, err := svc.Strains.ImportCannadb(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, s.ID, again.ID, "updated in place")
	assert.Equal(t, "Mandarin Cookies v2", again.Name)
	assert.Equal(t, mine, again.Breeder.ID)
	assert.Equal(t, "Ethos Genetics", again.Breeder.Name, "%q belongs to breeder %d", "Ethos", taken)
	require.Len(t, again.Parents, 1)
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM strain WHERE cannadb_uri = $1", in.URI).Scan(&n))
	assert.Equal(t, 1, n)

	// A breeder CannaDB only names is added; no name at all is "Unknown Breeder".
	other, err := svc.Strains.ImportCannadb(ctx, service.CannadbStrain{
		URI: "at://did/org.cannadb.strain/other", Name: "Other", IndicaPct: 50, SativaPct: 50,
	})
	require.NoError(t, err)
	assert.Equal(t, "Unknown Breeder", other.Breeder.Name)

	// The cache the pages read sees the import.
	found := false
	for _, st := range store.Strains() {
		found = found || st.Name == "Mandarin Cookies v2"
	}
	assert.True(t, found, "strain cache refreshed")

	_, err = svc.Strains.ImportCannadb(ctx, service.CannadbStrain{URI: "https://x", Name: "", IndicaPct: 50, SativaPct: 60})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Contains(t, e.Fields, "name")
	assert.Contains(t, e.Fields, "uri")
	assert.Contains(t, e.Fields, "indica_pct")
}
