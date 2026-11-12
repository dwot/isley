package service_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/service"
	"isley/tests/testutil"
)

// strainFamily is:
//
//	Grandparent A     Grandparent B
//	        \         /
//	       Parent Cross
//	            |
//	     Child Phenotype  ← also "Mystery", a parent with no strain
//
// plus Sibling Phenotype (Grandparent A only).
type strainFamily struct {
	breeder                int
	grandA, grandB, parent int
	child, sibling         int
}

func seedStrainFamily(t *testing.T, db *sql.DB) strainFamily {
	t.Helper()
	f := strainFamily{breeder: testutil.SeedBreeder(t, db, "Acme Genetics")}
	f.grandA = testutil.SeedStrain(t, db, f.breeder, "Grandparent A")
	f.grandB = testutil.SeedStrain(t, db, f.breeder, "Grandparent B")
	f.parent = testutil.SeedStrain(t, db, f.breeder, "Parent Cross")
	f.child = testutil.SeedStrain(t, db, f.breeder, "Child Phenotype")
	f.sibling = testutil.SeedStrain(t, db, f.breeder, "Sibling Phenotype")
	link := func(strain int, name string, parent *int) {
		testutil.MustExec(t, db,
			"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3)", strain, name, parent)
	}
	link(f.parent, "Grandparent B", &f.grandB)
	link(f.parent, "Grandparent A", &f.grandA)
	link(f.child, "Parent Cross", &f.parent)
	link(f.child, "Mystery", nil)
	link(f.sibling, "Grandparent A", &f.grandA)
	return f
}

func TestStrains(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkStrains(t, testutil.NewServices(t, db), db)
}

func checkStrains(t *testing.T, svc *service.Services, db *sql.DB) {
	ctx := context.Background()
	f := seedStrainFamily(t, db)

	// Parents come by name, linked where there is a strain.
	child, err := svc.Strains.Get(ctx, f.child)
	require.NoError(t, err)
	require.Len(t, child.Parents, 2)
	assert.Equal(t, "Mystery", child.Parents[0].ParentName)
	assert.Nil(t, child.Parents[0].Parent)
	assert.Equal(t, &service.Ref{ID: f.parent, Name: "Parent Cross"}, child.Parents[1].Parent)
	assert.Equal(t, service.Ref{ID: f.breeder, Name: "Acme Genetics"}, child.Breeder)

	tree, err := svc.Strains.Ancestry(ctx, f.child)
	require.NoError(t, err)
	require.Len(t, tree, 2)
	cross := tree[1]
	require.Len(t, cross.Parents, 2)
	assert.Equal(t, "Grandparent A", cross.Parents[0].ParentName)
	assert.Empty(t, cross.Parents[0].Parents)

	ds, err := svc.Strains.Descendants(ctx, f.grandA)
	require.NoError(t, err)
	require.Len(t, ds, 2)
	assert.Equal(t, "Parent Cross", ds[0].Name)
	assert.Equal(t, []string{"Grandparent B"}, ds[0].OtherParents)
	assert.Equal(t, "Sibling Phenotype", ds[1].Name)
	assert.Empty(t, ds[1].OtherParents)

	// A parent can't be the strain itself or one of its descendants.
	_, err = svc.Strains.AddParent(ctx, f.grandA, service.ParentInput{StrainID: &f.child})
	assert.True(t, service.IsKind(err, service.KindConflict), "%v", err)
	_, err = svc.Strains.SetLineage(ctx, f.parent, []service.ParentInput{{StrainID: &f.parent}})
	assert.True(t, service.IsKind(err, service.KindConflict), "%v", err)

	// A linked parent's name defaults to the strain's.
	entries, err := svc.Strains.SetLineage(ctx, f.sibling, []service.ParentInput{{StrainID: &f.grandB}, {Name: "Landrace"}})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "Grandparent B", entries[0].ParentName)
	assert.Equal(t, "Landrace", entries[1].ParentName)
	_, err = svc.Strains.SetLineage(ctx, f.sibling, []service.ParentInput{{Name: "ok"}, {Name: ""}})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Contains(t, e.Fields, "parents[1].name")

	// Creating with a breeder name reuses the breeder, ignoring case.
	cycle := 63
	st, err := svc.Strains.Create(ctx, service.StrainInput{
		Name: " Blue Dream ", BreederName: "acme genetics", IndicaPct: 40, SativaPct: 60,
		SeedCount: 5, CycleTimeDays: &cycle, URL: "www.example.com/blue",
	})
	require.NoError(t, err)
	assert.Equal(t, "Blue Dream", st.Name)
	assert.Equal(t, f.breeder, st.Breeder.ID)
	assert.Equal(t, "https://www.example.com/blue", st.URL)
	assert.Equal(t, 63, *st.CycleTimeDays)
	assert.Empty(t, st.Parents)

	_, err = svc.Strains.Create(ctx, service.StrainInput{Name: "x", BreederID: f.breeder, IndicaPct: 70, SativaPct: 70, SeedCount: -1, URL: "javascript:alert(1)"})
	e, ok = service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Contains(t, e.Fields, "sativa_pct")
	assert.Contains(t, e.Fields, "seed_count")
	assert.Contains(t, e.Fields, "url")
	_, err = svc.Strains.Create(ctx, service.StrainInput{Name: "x", BreederID: 9999, IndicaPct: 50, SativaPct: 50})
	e, ok = service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Contains(t, e.Fields, "breeder_id")

	// Patch leaves what isn't set.
	desc := "dense buds"
	st, err = svc.Strains.Update(ctx, st.ID, service.StrainUpdate{Description: service.Put(&desc), BreederName: "New Co"})
	require.NoError(t, err)
	assert.Equal(t, "dense buds", st.Description)
	assert.Equal(t, 40, st.IndicaPct)
	assert.Equal(t, "New Co", st.Breeder.Name)

	// Lists: by name, filtered.
	testutil.MustExec(t, db, "UPDATE strain SET seed_count = 0 WHERE breeder_id = $1", f.breeder)
	inStock := true
	list, total, err := svc.Strains.List(ctx, service.StrainFilter{InStock: &inStock})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "Blue Dream", list[0].Name)
	list, total, err = svc.Strains.List(ctx, service.StrainFilter{Query: "PHENO", Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, list, 1)
	assert.Equal(t, "Child Phenotype", list[0].Name)

	// Seed counts move by delta, together, and never below zero.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Strains.AdjustSeeds(ctx, st.ID, -1)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	st, err = svc.Strains.Get(ctx, st.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, st.SeedCount)
	_, err = svc.Strains.AdjustSeeds(ctx, st.ID, -2)
	assert.True(t, service.IsKind(err, service.KindConflict), "%v", err)

	// A strain with plants can't go; one without takes its lineage with it
	// and leaves its descendants' entries unlinked.
	zone := testutil.SeedZone(t, db, "Tent")
	testutil.SeedPlant(t, db, "P", f.grandB, zone)
	assert.True(t, service.IsKind(svc.Strains.Delete(ctx, f.grandB), service.KindConflict))
	require.NoError(t, svc.Strains.Delete(ctx, f.parent))
	child, err = svc.Strains.Get(ctx, f.child)
	require.NoError(t, err)
	assert.Equal(t, "Parent Cross", child.Parents[1].ParentName)
	assert.Nil(t, child.Parents[1].Parent)
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM strain_lineage WHERE strain_id = $1", f.parent).Scan(&n))
	assert.Zero(t, n)
	assert.True(t, service.IsKind(svc.Strains.Delete(ctx, f.parent), service.KindNotFound))
}

// A cycle already in the data (from an import or an old version) is shown
// once, not followed forever.
func TestStrains_AncestryStopsAtCycles(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	svc := testutil.NewServices(t, db)
	f := seedStrainFamily(t, db)
	testutil.MustExec(t, db,
		"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Cycle', $2)", f.grandA, f.child)

	done := make(chan []service.AncestryNode)
	go func() {
		tree, err := svc.Strains.Ancestry(context.Background(), f.child)
		assert.NoError(t, err)
		done <- tree
	}()
	select {
	case tree := <-done:
		require.Len(t, tree, 2)
	case <-time.After(5 * time.Second):
		t.Fatal("Ancestry did not finish; it followed the cycle")
	}
}

func TestBreeders(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkBreeders(t, testutil.NewServices(t, db), db)
}

func checkBreeders(t *testing.T, svc *service.Services, db *sql.DB) {
	ctx := context.Background()
	f := seedStrainFamily(t, db)

	b, err := svc.Breeders.Create(ctx, "  Zeta Seeds ")
	require.NoError(t, err)
	assert.Equal(t, "Zeta Seeds", b.Name)
	_, err = svc.Breeders.Create(ctx, "ZETA SEEDS")
	assert.True(t, service.IsKind(err, service.KindConflict))
	_, err = svc.Breeders.Rename(ctx, b.ID, "acme genetics")
	assert.True(t, service.IsKind(err, service.KindConflict))
	_, err = svc.Breeders.Rename(ctx, 9999, "x")
	assert.True(t, service.IsKind(err, service.KindNotFound))

	list, total, err := svc.Breeders.List(ctx, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, "Acme Genetics", list[0].Name)
	assert.Equal(t, 5, list[0].StrainCount)

	// The API's delete refuses a breeder with strains; the settings page's
	// takes everything with it.
	assert.True(t, service.IsKind(svc.Breeders.Delete(ctx, f.breeder), service.KindConflict))
	require.NoError(t, svc.Breeders.Delete(ctx, b.ID))

	plant := testutil.SeedPlant(t, db, "P", f.child, testutil.SeedZone(t, db, "Tent"))
	require.NoError(t, svc.Breeders.DeleteWithEverything(ctx, f.breeder))
	for table, want := range map[string]int{"breeder": 0, "strain": 0, "strain_lineage": 0, "plant": 0} {
		var n int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&n))
		assert.Equal(t, want, n, table)
	}
	_, err = svc.Plants.Get(ctx, plant)
	assert.True(t, service.IsKind(err, service.KindNotFound))
}

// TestStrains_Feminized: a new strain is feminized unless it says otherwise,
// an update can change it (cleared means feminized again), and the list
// filters on it.
func TestStrains_Feminized(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkStrainFeminized(t, testutil.NewServices(t, db), db)
}

func checkStrainFeminized(t *testing.T, svc *service.Services, db *sql.DB) {
	ctx := context.Background()
	breeder := testutil.SeedBreeder(t, db, "Fem Genetics")
	// A strain from before migration 030 picks up the column default.
	seeded := testutil.SeedStrain(t, db, breeder, "Old Strain")
	st, err := svc.Strains.Get(ctx, seeded)
	require.NoError(t, err)
	assert.True(t, st.Feminized, "existing strains default to feminized")

	fem, err := svc.Strains.Create(ctx, service.StrainInput{Name: "Default Fem", BreederID: breeder, IndicaPct: 50, SativaPct: 50})
	require.NoError(t, err)
	assert.True(t, fem.Feminized, "a new strain is feminized unless it says otherwise")

	no := false
	reg, err := svc.Strains.Create(ctx, service.StrainInput{Name: "A Regular", BreederID: breeder, IndicaPct: 50, SativaPct: 50, Feminized: &no})
	require.NoError(t, err)
	assert.False(t, reg.Feminized)

	list, total, err := svc.Strains.List(ctx, service.StrainFilter{Feminized: &no})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "A Regular", list[0].Name)
	yes := true
	_, total, err = svc.Strains.List(ctx, service.StrainFilter{Feminized: &yes})
	require.NoError(t, err)
	assert.Equal(t, 2, total)

	upd, err := svc.Strains.Update(ctx, fem.ID, service.StrainUpdate{Feminized: service.Put(&no)})
	require.NoError(t, err)
	assert.False(t, upd.Feminized)
	three := 3
	upd, err = svc.Strains.Update(ctx, fem.ID, service.StrainUpdate{SeedCount: service.Put(&three)})
	require.NoError(t, err)
	assert.False(t, upd.Feminized, "an update that doesn't mention it keeps it")
	upd, err = svc.Strains.Update(ctx, fem.ID, service.StrainUpdate{Feminized: service.Patch[bool]{Set: true}})
	require.NoError(t, err)
	assert.True(t, upd.Feminized, "cleared means feminized again")
}
