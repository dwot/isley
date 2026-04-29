package handlers_test

// HTTP-layer tests for handlers/plant_image.go that complement
// tests/integration/plant_image_test.go. The integration suite covers
// the upload happy path, MIME validation, bad-ID, and delete; this file
// adds:
//
//   - auth gating across both endpoints
//   - upload with no files (handler should still respond 200 with an
//     empty ids array)

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Auth gating
// ---------------------------------------------------------------------------

func TestPlantImageHTTP_AuthGating(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/plant/1/images/upload"},
		{http.MethodDelete, "/plant/images/1/delete"},
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
// UploadPlantImages — extra branches
// ---------------------------------------------------------------------------

// TestPlantImageHTTP_Upload_NonNumericPlantID confirms the up-front
// strconv.Atoi guard surfaces a non-numeric :plantID as 400.
func TestPlantImageHTTP_Upload_NonNumericPlantID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "img-bad-plantid-key"
	testutil.SeedAPIKey(t, db, apiKey)

	// Empty multipart body — handler validates :plantID first.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.Close())

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/notanumber/images/upload",
		apiKey, &buf, w.FormDataContentType()))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestPlantImageHTTP_Upload_EmptyFormReturns200 verifies that submitting
// an empty multipart form (no `images[]` field) returns 200 with an
// empty `ids` array — there are simply no files to process.
func TestPlantImageHTTP_Upload_EmptyFormReturns200(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db, testutil.WithUploadDir(t.TempDir()))

	const apiKey = "img-empty-key"
	testutil.SeedAPIKey(t, db, apiKey)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.Close())

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodPost, c.BaseURL+"/plant/1/images/upload",
		apiKey, &buf, w.FormDataContentType()))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		IDs     []int  `json:"ids"`
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Empty(t, got.IDs)
}

// ---------------------------------------------------------------------------
// DeletePlantImage — extra branches
// ---------------------------------------------------------------------------

func TestPlantImageHTTP_Delete_NonNumericImageID(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)

	const apiKey = "img-del-bad-key"
	testutil.SeedAPIKey(t, db, apiKey)

	c := server.NewClient(t)
	resp, err := c.Do(testutil.APIReq(t, http.MethodDelete, c.BaseURL+"/plant/images/notanumber/delete", apiKey, nil, ""))
	require.NoError(t, err)
	defer testutil.DrainAndClose(resp)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
