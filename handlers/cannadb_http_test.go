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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
