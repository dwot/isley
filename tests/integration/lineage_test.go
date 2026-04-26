package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// lineageHTTPFixture wires up the strain tree from lineage_db_test plus
// an API key suitable for X-API-KEY auth on the protected lineage write
// routes.
type lineageHTTPFixture struct {
	APIKey                     string
	GrandparentA, GrandparentB int
	Parent, Child, Sibling     int
}

func seedLineageHTTP(t *testing.T, db *sql.DB) lineageHTTPFixture {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "Acme Genetics")

	fix := lineageHTTPFixture{
		GrandparentA: testutil.SeedStrain(t, db, breederID, "Grandparent A"),
		GrandparentB: testutil.SeedStrain(t, db, breederID, "Grandparent B"),
		Parent:       testutil.SeedStrain(t, db, breederID, "Parent Cross"),
		Child:        testutil.SeedStrain(t, db, breederID, "Child Phenotype"),
		Sibling:      testutil.SeedStrain(t, db, breederID, "Sibling Phenotype"),
	}

	exec := func(query string, args ...interface{}) {
		_, err := db.Exec(query, args...)
		require.NoErrorf(t, err, "seed: %s", query)
	}
	exec(`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Grandparent A', $2)`,
		fix.Parent, fix.GrandparentA)
	exec(`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Grandparent B', $2)`,
		fix.Parent, fix.GrandparentB)
	exec(`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Parent Cross', $2)`,
		fix.Child, fix.Parent)
	exec(`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Mystery', NULL)`,
		fix.Child)
	exec(`INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, 'Grandparent A', $2)`,
		fix.Sibling, fix.GrandparentA)

	fix.APIKey = testutil.SeedAPIKey(t, db, "test-lineage-key")
	return fix
}

// ---------------------------------------------------------------------------
// GET /strains/:id/lineage
// ---------------------------------------------------------------------------

func TestLineage_GetTree(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lineage-pw")
	c := server.LoginAsAdmin(t, "lineage-pw")

	resp := c.Get("/strains/" + strconv.Itoa(fix.Child) + "/lineage")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tree []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tree))
	require.Len(t, tree, 2)

	// "Mystery" is alphabetical first; no children expanded.
	assert.Equal(t, "Mystery", tree[0]["parent_name"])
	// "Parent Cross" has its own ancestry array.
	assert.Equal(t, "Parent Cross", tree[1]["parent_name"])
	children, _ := tree[1]["children"].([]interface{})
	assert.Len(t, children, 2, "Parent Cross has Grandparent A + B as children")
}

func TestLineage_GetTreeEmptyReturnsArray(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lineage-pw")
	c := server.LoginAsAdmin(t, "lineage-pw")

	// GrandparentA has no parents.
	resp := c.Get("/strains/" + strconv.Itoa(fix.GrandparentA) + "/lineage")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tree []interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tree))
	assert.Empty(t, tree, "endpoint returns [] not null when there are no parents")
}

func TestLineage_GetTreeBadID(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	_ = seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lineage-pw")
	c := server.LoginAsAdmin(t, "lineage-pw")

	resp := c.Get("/strains/not-a-number/lineage")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// POST /strains/:id/lineage
// ---------------------------------------------------------------------------

func TestLineage_AddEntryWithStrainLink(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	parentID := fix.GrandparentB
	resp := c.APIPostJSON(t, "/strains/"+strconv.Itoa(fix.Sibling)+"/lineage", fix.APIKey, map[string]interface{}{
		"parent_name":      "Grandparent B",
		"parent_strain_id": parentID,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.ID)

	var pname string
	var pid sql.NullInt64
	require.NoError(t, db.QueryRow(
		`SELECT parent_name, parent_strain_id FROM strain_lineage WHERE id = $1`, got.ID,
	).Scan(&pname, &pid))
	assert.Equal(t, "Grandparent B", pname)
	require.True(t, pid.Valid)
	assert.EqualValues(t, parentID, pid.Int64)
}

func TestLineage_AddEntryFreeTextParent(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/strains/"+strconv.Itoa(fix.Sibling)+"/lineage", fix.APIKey, map[string]interface{}{
		"parent_name": "Unknown Landrace",
		// no parent_strain_id → free-text
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var pid sql.NullInt64
	require.NoError(t, db.QueryRow(
		`SELECT parent_strain_id FROM strain_lineage WHERE strain_id = $1 AND parent_name = 'Unknown Landrace'`,
		fix.Sibling,
	).Scan(&pid))
	assert.False(t, pid.Valid, "free-text parent stores NULL parent_strain_id")
}

func TestLineage_AddRejectsEmptyParentName(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/strains/"+strconv.Itoa(fix.Sibling)+"/lineage", fix.APIKey, map[string]interface{}{
		"parent_name": "",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineage_AddBadStrainID(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIPostJSON(t, "/strains/abc/lineage", fix.APIKey, map[string]interface{}{
		"parent_name": "Anything",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// PUT /lineage/:lineageID
// ---------------------------------------------------------------------------

func TestLineage_UpdateEntry(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	// Find the "Mystery" lineage row on Child.
	var lineageID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM strain_lineage WHERE strain_id = $1 AND parent_name = 'Mystery'`, fix.Child,
	).Scan(&lineageID))

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/lineage/"+strconv.Itoa(lineageID), fix.APIKey, map[string]interface{}{
		"parent_name":      "Identified Parent",
		"parent_strain_id": fix.GrandparentB,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var pname string
	var pid sql.NullInt64
	require.NoError(t, db.QueryRow(
		`SELECT parent_name, parent_strain_id FROM strain_lineage WHERE id = $1`, lineageID,
	).Scan(&pname, &pid))
	assert.Equal(t, "Identified Parent", pname)
	require.True(t, pid.Valid)
	assert.EqualValues(t, fix.GrandparentB, pid.Int64)
}

func TestLineage_UpdateBadLineageID(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/lineage/abc", fix.APIKey, map[string]interface{}{
		"parent_name": "Anything",
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DELETE /lineage/:lineageID
// ---------------------------------------------------------------------------

func TestLineage_DeleteEntry(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	var lineageID int
	require.NoError(t, db.QueryRow(
		`SELECT id FROM strain_lineage WHERE strain_id = $1 AND parent_name = 'Mystery'`, fix.Child,
	).Scan(&lineageID))

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/lineage/"+strconv.Itoa(lineageID), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM strain_lineage WHERE id = $1`, lineageID).Scan(&n))
	assert.Zero(t, n)
}

func TestLineage_DeleteMissing(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/lineage/9999", fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// PUT /strains/:id/lineage  (bulk replace)
// ---------------------------------------------------------------------------

func TestLineage_SetReplacesAllEntries(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	// Child currently has 2 lineage rows. Replace with a single entry.
	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/strains/"+strconv.Itoa(fix.Child)+"/lineage", fix.APIKey, map[string]interface{}{
		"parents": []map[string]interface{}{
			{"parent_name": "Sole Parent", "parent_strain_id": fix.Parent},
		},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	rows, err := db.Query(
		`SELECT parent_name, parent_strain_id FROM strain_lineage WHERE strain_id = $1`, fix.Child,
	)
	require.NoError(t, err)
	defer rows.Close()

	var seen []string
	for rows.Next() {
		var pname string
		var pid sql.NullInt64
		require.NoError(t, rows.Scan(&pname, &pid))
		seen = append(seen, pname)
	}
	assert.Equal(t, []string{"Sole Parent"}, seen, "previous lineage rows must be wiped")
}

func TestLineage_SetSkipsEmptyParentNames(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/strains/"+strconv.Itoa(fix.Child)+"/lineage", fix.APIKey, map[string]interface{}{
		"parents": []map[string]interface{}{
			{"parent_name": ""},     // skipped
			{"parent_name": "Kept"}, // inserted
		},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM strain_lineage WHERE strain_id = $1`, fix.Child,
	).Scan(&n))
	assert.Equal(t, 1, n, "empty parent_name entries must be skipped")
}

// ---------------------------------------------------------------------------
// GET /strains/:id/descendants
// ---------------------------------------------------------------------------

func TestLineage_Descendants(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "desc-pw")
	c := server.LoginAsAdmin(t, "desc-pw")

	resp := c.Get("/strains/" + strconv.Itoa(fix.GrandparentA) + "/descendants")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 2)
	// Sorted alphabetically: "Parent Cross" then "Sibling Phenotype".
	assert.Equal(t, "Parent Cross", got[0]["name"])
	assert.Equal(t, "Grandparent B", got[0]["other_parents"],
		"other_parents should aggregate non-self parents")
	assert.Equal(t, "Sibling Phenotype", got[1]["name"])
	assert.Equal(t, "", got[1]["other_parents"],
		"sibling has no other parents; aggregation returns empty string")
}

// ---------------------------------------------------------------------------
// GET /strains/lookup
// ---------------------------------------------------------------------------

func TestLineage_LookupByName_CaseInsensitive(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	_ = seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lookup-pw")
	c := server.LoginAsAdmin(t, "lookup-pw")

	// "PHENOTYPE" should match both "Child Phenotype" and "Sibling Phenotype".
	resp := c.Get("/strains/lookup?q=PHENOTYPE")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Len(t, got, 2)
}

func TestLineage_LookupEmptyQueryReturnsEmptyArray(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	_ = seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lookup-pw")
	c := server.LoginAsAdmin(t, "lookup-pw")

	resp := c.Get("/strains/lookup?q=")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestLineage_LookupNoMatches(t *testing.T) {
	resetRateLimit(t)
	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	_ = seedLineageHTTP(t, db)

	testutil.SeedAdmin(t, db, "lookup-pw")
	c := server.LoginAsAdmin(t, "lookup-pw")

	resp := c.Get("/strains/lookup?q=zzznonexistent")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Empty(t, got, "no matches returns [] not null")
}
