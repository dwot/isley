package handlers_test

// HTTP-layer tests for handlers/lineage.go that complement
// tests/integration/lineage_test.go. The integration suite already
// covers the handlers' main scenarios in depth; this file fills in:
//
//   - auth gating across the api-protected lineage write endpoints
//   - non-numeric :id / :lineageID branches that integration tests do
//     not consistently exercise
//   - bad-JSON branches on Add/Update/SetLineage
//   - public-read tests that need no auth (GetLineage, GetDescendants,
//     LookupStrainByName)
//
// Routes:
//
//   POST   /strains/:id/lineage   (api-protected) → AddLineageHandler
//   PUT    /strains/:id/lineage   (api-protected) → SetLineageHandler
//   PUT    /lineage/:lineageID    (api-protected) → UpdateLineageHandler
//   DELETE /lineage/:lineageID    (api-protected) → DeleteLineageHandler
//   GET    /strains/:id/lineage     (basic)       → GetLineageHandler
//   GET    /strains/:id/descendants (basic)       → GetDescendantsHandler
//   GET    /strains/lookup          (basic)       → LookupStrainByName

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// lineageSeedStrains inserts a breeder + two strains. The integration
// suite uses a near-identical helper; kept file-local because the IDs
// (1=Child, 2=Parent) are baked into the surrounding test assertions.
func lineageSeedStrains(t *testing.T, db *sql.DB) {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "B")
	testutil.SeedStrain(t, db, breederID, "Child Strain")
	testutil.SeedStrain(t, db, breederID, "Parent Strain")
}

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestLineageHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/strains/1/lineage"},
		{http.MethodPut, "/strains/1/lineage"},
		{http.MethodPut, "/lineage/1"},
		{http.MethodDelete, "/lineage/1"},
	}

	c := server.NewClient(t)
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, c.BaseURL+tc.path, nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer testutil.DrainAndClose(resp)
			assert.Containsf(t,
				[]int{http.StatusUnauthorized, http.StatusForbidden},
				resp.StatusCode,
				"%s %s should be rejected (got %d)", tc.method, tc.path, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// AddLineageHandler — non-numeric :id and bad-JSON branches
// ---------------------------------------------------------------------------

func TestLineageHTTP_Add_RejectsNonNumericStrainID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains/abc/lineage", apiKey,
		testutil.JSONBody(t, map[string]interface{}{"parent_name": "X"}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Add_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains/1/lineage", apiKey,
		bytes.NewBufferString(`{not json`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Add_RejectsLongParentName(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-add-long-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains/1/lineage", apiKey,
		testutil.JSONBody(t, map[string]interface{}{"parent_name": strings.Repeat("p", 1024)}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// UpdateLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Update_RejectsNonNumericLineageID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-upd-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/lineage/notanumber", apiKey,
		testutil.JSONBody(t, map[string]interface{}{"parent_name": "X"}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Update_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-upd-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/lineage/1", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DeleteLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Delete_RejectsNonNumericLineageID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-del-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/lineage/abc", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// SetLineageHandler
// ---------------------------------------------------------------------------

func TestLineageHTTP_Set_RejectsNonNumericStrainID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-set-id-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/strains/abc/lineage", apiKey,
		testutil.JSONBody(t, map[string]interface{}{"parents": []interface{}{}}), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Set_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "lin-set-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPut, c.BaseURL+"/strains/1/lineage", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// GetLineageHandler / GetDescendantsHandler  (public reads)
// ---------------------------------------------------------------------------

func TestLineageHTTP_Get_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/abc/lineage")
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Get_EmptyReturnsArray(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	lineageSeedStrains(t, db)

	c := server.NewClient(t)
	resp := c.Get("/strains/1/lineage")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t,
		bytes.HasPrefix(body, []byte("[")),
		"empty lineage must serialise as [], not null; got %q", body)
}

func TestLineageHTTP_Descendants_RejectsNonNumericID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	resp := c.Get("/strains/abc/descendants")
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestLineageHTTP_Descendants_EmptyReturnsArray(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())
	lineageSeedStrains(t, db)

	c := server.NewClient(t)
	resp := c.Get("/strains/2/descendants")
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(body, []byte("[")),
		"empty descendants must serialise as []; got %q", body)
}

// ---------------------------------------------------------------------------
// LookupStrainByName  (public read)
// ---------------------------------------------------------------------------

func TestLineageHTTP_Lookup_TruncatesOverlongQuery(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithGuestMode())

	c := server.NewClient(t)
	// Long query is truncated to MaxNameLength internally — handler must
	// still respond 200 with a JSON array (possibly empty).
	resp := c.Get("/strains/lookup?q=" + strings.Repeat("z", 4096))
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(body, []byte("[")), "lookup must return []")
}
