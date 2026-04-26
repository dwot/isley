package handlers_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/handlers"
	"isley/tests/testutil"
)

// External package — handlers_test — to dodge the
// handlers→app→handlers cycle when test files import tests/testutil.

// seedStrainTree wires up a small strain family. Returns the strain ids
// keyed by name. The tree:
//
//	Grandparent A     Grandparent B
//	        \         /
//	         \       /
//	       Parent (Cross of A & B)
//	          |
//	        Child  ← also has a non-strain parent "Mystery" entry
type strainTreeIDs struct {
	GrandparentA, GrandparentB int
	Parent, Child              int
	Sibling                    int // descendant of GrandparentA only
}

func seedStrainTree(t *testing.T, db *sql.DB) strainTreeIDs {
	t.Helper()

	breederID := testutil.SeedBreeder(t, db, "Acme Genetics")

	ids := strainTreeIDs{
		GrandparentA: testutil.SeedStrain(t, db, breederID, "Grandparent A"),
		GrandparentB: testutil.SeedStrain(t, db, breederID, "Grandparent B"),
		Parent:       testutil.SeedStrain(t, db, breederID, "Parent Cross"),
		Child:        testutil.SeedStrain(t, db, breederID, "Child Phenotype"),
		Sibling:      testutil.SeedStrain(t, db, breederID, "Sibling Phenotype"),
	}

	// strain_lineage isn't modeled in testutil — those rows are
	// lineage-test-specific and stay inline.
	insertLineage := func(strainID int, parentName string, parentStrainID *int) {
		testutil.MustExec(t, db,
			`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3)`,
			strainID, parentName, parentStrainID,
		)
	}

	// Parent has Grandparent A and Grandparent B as parents.
	insertLineage(ids.Parent, "Grandparent A", &ids.GrandparentA)
	insertLineage(ids.Parent, "Grandparent B", &ids.GrandparentB)

	// Child has Parent as a parent + a free-text "Mystery" entry without a strain link.
	insertLineage(ids.Child, "Parent Cross", &ids.Parent)
	insertLineage(ids.Child, "Mystery", nil)

	// Sibling has Grandparent A only (used by descendants test for the
	// other-parents aggregation column).
	insertLineage(ids.Sibling, "Grandparent A", &ids.GrandparentA)

	return ids
}

// ---------------------------------------------------------------------------
// GetLineage
// ---------------------------------------------------------------------------

func TestGetLineage_OrderedByParentName(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	got := handlers.GetLineage(db, ids.Parent)
	require.Len(t, got, 2)
	// Alphabetical: "Grandparent A" before "Grandparent B".
	assert.Equal(t, "Grandparent A", got[0].ParentName)
	assert.Equal(t, "Grandparent B", got[1].ParentName)

	// parent_strain_id should be populated for both.
	require.NotNil(t, got[0].ParentStrainID)
	require.NotNil(t, got[1].ParentStrainID)
	assert.Equal(t, ids.GrandparentA, *got[0].ParentStrainID)
	assert.Equal(t, ids.GrandparentB, *got[1].ParentStrainID)
}

func TestGetLineage_StrainWithoutParents(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	// Grandparent A has no parents seeded.
	got := handlers.GetLineage(db, ids.GrandparentA)
	assert.Empty(t, got)
}

func TestGetLineage_FreeTextParentHasNullStrainID(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	got := handlers.GetLineage(db, ids.Child)
	require.Len(t, got, 2)

	// "Mystery" comes before "Parent Cross" alphabetically.
	assert.Equal(t, "Mystery", got[0].ParentName)
	assert.Nil(t, got[0].ParentStrainID, "free-text parent has no strain id")
	assert.Equal(t, "Parent Cross", got[1].ParentName)
	require.NotNil(t, got[1].ParentStrainID)
	assert.Equal(t, ids.Parent, *got[1].ParentStrainID)
}

// ---------------------------------------------------------------------------
// GetAncestryTree
// ---------------------------------------------------------------------------

func TestGetAncestryTree_Recurses(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	tree := handlers.GetAncestryTree(db, ids.Child, 0)
	require.Len(t, tree, 2)

	// "Mystery" — free-text parent, no children expanded.
	assert.Equal(t, "Mystery", tree[0].ParentName)
	assert.Empty(t, tree[0].Children, "free-text parent shouldn't recurse")

	// "Parent Cross" — has its own ancestry (Grandparent A + B).
	parentBranch := tree[1]
	assert.Equal(t, "Parent Cross", parentBranch.ParentName)
	require.Len(t, parentBranch.Children, 2)
	assert.Equal(t, "Grandparent A", parentBranch.Children[0].ParentName)
	assert.Empty(t, parentBranch.Children[0].Children, "leaf has no further parents")
	assert.Equal(t, "Grandparent B", parentBranch.Children[1].ParentName)
}

// TestGetAncestryTree_DepthCap pins the cycle-prevention contract: even
// if the lineage table contains a cycle, GetAncestryTree returns rather
// than recursing forever.
func TestGetAncestryTree_DepthCap(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	// Inject a cycle: Grandparent A names Child as its parent.
	_, err := db.Exec(
		`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Cycle', $2)`,
		ids.GrandparentA, ids.Child,
	)
	require.NoError(t, err)

	// Test contract: the call must terminate. We don't assert the
	// shape past the cap because that's an implementation detail.
	done := make(chan struct{})
	go func() {
		_ = handlers.GetAncestryTree(db, ids.Child, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GetAncestryTree did not terminate within 5 seconds; cycle may have caused infinite recursion")
	}
}

// ---------------------------------------------------------------------------
// GetDescendants
// ---------------------------------------------------------------------------

func TestGetDescendants_ReturnsAllDescendantsWithOtherParents(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	// Grandparent A is referenced by Parent Cross (which also has
	// Grandparent B as a parent) and Sibling (no other parent).
	got := handlers.GetDescendants(db, ids.GrandparentA)
	require.Len(t, got, 2)

	// Sorted by name ASC: "Parent Cross" then "Sibling Phenotype".
	assert.Equal(t, "Parent Cross", got[0]["name"])
	assert.Equal(t, "Acme Genetics", got[0]["breeder"])
	assert.Equal(t, "Grandparent B", got[0]["other_parents"],
		"Parent Cross's only other parent is Grandparent B")

	assert.Equal(t, "Sibling Phenotype", got[1]["name"])
	assert.Empty(t, got[1]["other_parents"], "Sibling has no other parents")
}

func TestGetDescendants_NoneReturnsNil(t *testing.T) {
	db := testutil.NewTestDB(t)
	ids := seedStrainTree(t, db)

	// Child has no descendants.
	got := handlers.GetDescendants(db, ids.Child)
	assert.Empty(t, got)
}
