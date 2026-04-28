package handlers_test

// HTTP-layer tests for handlers/plant_status_log.go (EditStatus,
// DeleteStatus). The integration suite covers the meaningful branches;
// this file fills in:
//
//   - auth gating across both endpoints
//   - bad-JSON branch on EditStatus

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlantStatusLogHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plantStatus/edit"},
		{http.MethodDelete, "/plantStatus/delete/1"},
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
// EditStatus / DeleteStatus error branches not covered by the integration suite
// ---------------------------------------------------------------------------

func TestPlantStatusLogHTTP_Edit_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psl-edit-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plantStatus/edit", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
