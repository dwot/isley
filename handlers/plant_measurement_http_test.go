package handlers_test

// HTTP-layer tests for handlers/plant_measurement.go that complement
// tests/integration/plant_measurement_test.go.

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

func TestPlantMeasurementHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plantMeasurement"},
		{http.MethodPost, "/plantMeasurement/edit"},
		{http.MethodDelete, "/plantMeasurement/delete/1"},
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
// Validation branches
// ---------------------------------------------------------------------------

func TestPlantMeasurementHTTP_Create_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-create-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plantMeasurement", apiKey,
		bytes.NewBufferString(`{`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantMeasurementHTTP_Edit_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-edit-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plantMeasurement/edit", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantMeasurementHTTP_Delete_NoOpOnMissing(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "pm-del-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/plantMeasurement/delete/99999", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
