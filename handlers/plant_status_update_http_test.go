package handlers_test

// HTTP-layer tests for handlers/plant_status_update.go (UpdatePlantStatus
// at POST /plant/status). The integration suite already covers the
// happy path and "no-op when same status" branch; this file fills in
// auth gating and the missing-field branches.

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

func TestPlantStatusUpdateHTTP_RequiresAuth(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	c := server.NewClient(t)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/plant/status", bytes.NewBufferString(`{}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Containsf(t,
		[]int{http.StatusUnauthorized, http.StatusForbidden},
		resp.StatusCode,
		"unauthenticated POST /plant/status should be rejected (got %d)", resp.StatusCode)
}

// ---------------------------------------------------------------------------
// Validation branches
// ---------------------------------------------------------------------------

func TestPlantStatusUpdateHTTP_RejectsBadJSON(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-json-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantStatusUpdateHTTP_RejectsMissingPlantID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-missing-pid-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`{"status_id":1}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPlantStatusUpdateHTTP_RejectsMissingStatusID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "psu-missing-sid-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/status", apiKey,
		bytes.NewBufferString(`{"plant_id":1}`), "application/json"))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
