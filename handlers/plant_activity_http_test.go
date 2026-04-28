package handlers_test

// HTTP-layer tests for handlers/plant_activity.go that complement
// tests/integration/plant_activity_test.go. The integration suite
// covers create/edit/delete/multi happy paths plus a few validation
// branches; this file adds:
//
//   - auth gating across the four api-protected endpoints
//   - EditActivity bad-JSON branch
//   - DeleteActivity no-op behavior on missing id
//   - RecordMultiPlantActivity bad-JSON branch

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

func TestPlantActivityHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plantActivity"},
		{http.MethodPost, "/plantActivity/edit"},
		{http.MethodDelete, "/plantActivity/delete/1"},
		{http.MethodPost, "/record-multi-activity"},
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
// EditActivity / DeleteActivity / RecordMultiPlantActivity — error paths
// ---------------------------------------------------------------------------

func TestPlantActivityHTTP_Edit_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-edit-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plantActivity/edit", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantActivityHTTP_Delete_NoOpOnMissing(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-del-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/plantActivity/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"deleting a non-existent activity is a no-op success")
}

func TestPlantActivityHTTP_Multi_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-multi-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/record-multi-activity", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantActivityHTTP_Create_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pa-create-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plantActivity", apiKey,
		bytes.NewBufferString(`bogus`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
