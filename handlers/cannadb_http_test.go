package handlers_test

// HTTP-layer tests for handlers/cannadb.go (CannaDB import).
//
// The success paths require live calls to the CannaDB API, which tests must
// not make, so this file covers auth gating, the integration-disabled branch
// (the default), and basic request validation.
//
// Routes (both api-protected):
//   GET  /strains/cannadb/search  → CannadbSearchHandler
//   POST /strains/cannadb/import  → CannadbImportHandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/tests/testutil"
)

func TestCannadbHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct{ method, path string }{
		{http.MethodGet, "/strains/cannadb/search?q=og"},
		{http.MethodPost, "/strains/cannadb/import"},
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

// With the integration disabled (the default — no cannadb.enabled setting),
// authenticated requests are rejected with 400 before any outbound call.
func TestCannadbHTTP_DisabledByDefault(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "cannadb-disabled-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)

	t.Run("search", func(t *testing.T) {
		resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/strains/cannadb/search?q=og", apiKey, nil, ""))
		require.NoError(t, err)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("import", func(t *testing.T) {
		resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains/cannadb/import", apiKey,
			testutil.JSONBody(t, map[string]interface{}{"uri": "at://did/org.cannadb.strain/x"}), "application/json"))
		require.NoError(t, err)
		defer testutil.DrainAndClose(resp)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// Search and import against a fake CannaDB: the strain lands with its
// breeder (from the breeder record) and parents, and a second import
// updates it rather than adding another.
func TestCannadbHTTP_SearchAndImport(t *testing.T) {
	t.Parallel()

	const strainURI = "at://did:plc:x/org.cannadb.strain/rk"
	const breederURI = "at://did:plc:x/org.cannadb.breeder/b"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xrpc/org.cannadb.searchStrains":
			_, _ = io.WriteString(w, `{"strains":[{"uri":"`+strainURI+`","name":"Gelato 41","breederName":"Cookies","similarity":0.9}]}`)
		case "/xrpc/org.cannadb.getStrain":
			assert.Equal(t, strainURI, r.URL.Query().Get("uri"))
			_, _ = io.WriteString(w, `{"uri":"`+strainURI+`","indexedAt":"2026-11-01T00:00:00Z","value":{
				"name":"Gelato 41","breeder":"`+breederURI+`","breederName":"cookies fam","indicaSativa":45,
				"cycleTime":{"min":56,"max":63},"parentNames":["Sunset Sherbet","Thin Mint GSC"]}}`)
		case "/xrpc/org.cannadb.getBreeder":
			_, _ = io.WriteString(w, `{"uri":"`+breederURI+`","indexedAt":"2026-10-01T00:00:00Z","value":{"name":"Cookies"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fake.Close)

	db := testutil.NewTestDB(t)
	store := config.NewStore()
	store.SetCannadbEnabled(1)
	store.SetCannadbBaseURL(fake.URL + "/xrpc/")
	server := testutil.NewTestServer(t, db, testutil.WithConfigStore(store))
	const apiKey = "cannadb-import-key-000000000000"
	testutil.SeedAPIKey(t, db, apiKey)
	c := server.NewClient(t)

	resp, err := c.Do(testutil.APIReq(t, http.MethodGet, c.BaseURL+"/strains/cannadb/search?q=gelato", apiKey, nil, ""))
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Contains(t, string(body), `"name":"Gelato 41"`)

	importOnce := func() int {
		resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/strains/cannadb/import", apiKey,
			testutil.JSONBody(t, map[string]interface{}{"uri": strainURI}), "application/json"))
		require.NoError(t, err)
		var out struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			CannadbURL string `json:"cannadb_url"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		testutil.DrainAndClose(resp)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "Gelato 41", out.Name)
		assert.Equal(t, "https://cannadb.org/strain/did:plc:x/rk", out.CannadbURL)
		return out.ID
	}
	id := importOnce()
	assert.Equal(t, id, importOnce(), "a re-import updates the same strain")

	var breeder string
	var sativa, cycle, parents int
	require.NoError(t, db.QueryRow(`SELECT b.name, s.sativa, s.cycle_time,
		(SELECT COUNT(*) FROM strain_lineage l WHERE l.strain_id = s.id)
		FROM strain s JOIN breeder b ON b.id = s.breeder_id WHERE s.id = $1`, id).Scan(&breeder, &sativa, &cycle, &parents))
	assert.Equal(t, "Cookies", breeder, "the breeder record's name beats breederName")
	assert.Equal(t, 45, sativa)
	assert.Equal(t, 63, cycle)
	assert.Equal(t, 2, parents)
}
