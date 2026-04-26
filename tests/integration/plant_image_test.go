package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// jpegMagic is a tiny but valid JPEG byte sequence — enough that
// http.DetectContentType returns "image/jpeg". The file content beyond
// the magic doesn't matter for the upload handler's MIME-sniff check.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00}

// pngMagic recognised as image/png.
var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}

type plantImageFixture struct {
	APIKey  string
	PlantID int64
}

func seedPlantImageHTTP(t *testing.T, db *sql.DB) plantImageFixture {
	t.Helper()
	breederID := testutil.SeedBreeder(t, db, "B")
	strainID := testutil.SeedStrain(t, db, breederID, "S")
	zoneID := testutil.SeedZone(t, db, "Z")
	pid := int64(testutil.SeedPlant(t, db, "Plant 1", strainID, zoneID))

	return plantImageFixture{
		APIKey:  testutil.SeedAPIKey(t, db, "test-image-key"),
		PlantID: pid,
	}
}

// uploadImages issues a multipart POST to /plant/:plantID/images/upload
// with the provided file payloads.
func uploadImages(t *testing.T, c *testutil.Client, plantID int64, apiKey string, files []multipartFile) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("images[]", f.name)
		require.NoError(t, err)
		_, err = w.Write(f.body)
		require.NoError(t, err)
	}
	require.NoError(t, mw.WriteField("descriptions[]", "test description"))
	require.NoError(t, mw.WriteField("dates[]", "2026-04-25"))
	require.NoError(t, mw.Close())

	req, err := http.NewRequest(
		http.MethodPost,
		c.BaseURL+"/plant/"+strconv.FormatInt(plantID, 10)+"/images/upload",
		&buf,
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-KEY", apiKey)

	resp, err := c.Do(req)
	require.NoError(t, err)
	return resp
}

type multipartFile struct {
	name string
	body []byte
}

// ---------------------------------------------------------------------------
// POST /plant/:plantID/images/upload
// ---------------------------------------------------------------------------

func TestPlantImage_UploadHappyPath(t *testing.T) {
	resetRateLimit(t)
	// Scope file writes to a tempdir; UploadPlantImages writes under
	// uploads/plants/ relative to the working directory.
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	c := server.NewClient(t)
	resp := uploadImages(t, c, fix.PlantID, fix.APIKey, []multipartFile{
		{name: "photo.jpg", body: jpegMagic},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got struct {
		IDs     []int  `json:"ids"`
		Message string `json:"message"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got.IDs, 1)

	// DB row exists with the path the handler chose.
	var path string
	require.NoError(t, db.QueryRow(
		`SELECT image_path FROM plant_images WHERE id = $1`, got.IDs[0],
	).Scan(&path))
	assert.True(t, filepath.IsLocal(path), "image path should be a local relative path")

	// File on disk exists at that path.
	_, err := os.Stat(path)
	require.NoError(t, err, "uploaded file should be saved on disk")
}

func TestPlantImage_UploadAcceptsPNG(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	c := server.NewClient(t)
	resp := uploadImages(t, c, fix.PlantID, fix.APIKey, []multipartFile{
		{name: "photo.png", body: pngMagic},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPlantImage_UploadRejectsTextFile(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	c := server.NewClient(t)
	resp := uploadImages(t, c, fix.PlantID, fix.APIKey, []multipartFile{
		{name: "evil.jpg", body: []byte("this is plain text not a real image")},
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"non-image content must be rejected by MIME sniffing")
}

func TestPlantImage_UploadRejectsBadPlantID(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	// non-numeric :plantID — handler returns 400 from strconv.Atoi.
	c := server.NewClient(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, _ := mw.CreateFormFile("images[]", "photo.jpg")
	_, _ = w.Write(jpegMagic)
	mw.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		c.BaseURL+"/plant/abc/images/upload",
		&buf,
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-KEY", fix.APIKey)

	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// DELETE /plant/images/:imageID/delete
// ---------------------------------------------------------------------------

func TestPlantImage_DeleteRemovesFileAndRow(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	// Upload first so we have a real file + row to delete.
	c := server.NewClient(t)
	upResp := uploadImages(t, c, fix.PlantID, fix.APIKey, []multipartFile{
		{name: "photo.jpg", body: jpegMagic},
	})
	require.Equal(t, http.StatusOK, upResp.StatusCode)
	var up struct {
		IDs []int `json:"ids"`
	}
	require.NoError(t, json.NewDecoder(upResp.Body).Decode(&up))
	upResp.Body.Close()
	require.Len(t, up.IDs, 1)
	imageID := up.IDs[0]

	var path string
	require.NoError(t, db.QueryRow(
		`SELECT image_path FROM plant_images WHERE id = $1`, imageID,
	).Scan(&path))
	require.FileExists(t, path)

	delResp := c.APIDelete(t, "/plant/images/"+strconv.Itoa(imageID)+"/delete", fix.APIKey)
	defer delResp.Body.Close()
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM plant_images WHERE id = $1`, imageID).Scan(&n))
	assert.Zero(t, n, "DB row removed")

	_, err := os.Stat(path)
	assert.Truef(t, os.IsNotExist(err), "file should be deleted from disk; stat err = %v", err)
}

func TestPlantImage_DeleteMissing(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plant/images/9999/delete", fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPlantImage_DeleteBadID(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedPlantImageHTTP(t, db)

	c := server.NewClient(t)
	resp := c.APIDelete(t, "/plant/images/not-a-number/delete", fix.APIKey)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
