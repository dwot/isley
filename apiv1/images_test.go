package apiv1_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// jpeg is enough of a JPEG for content sniffing.
var jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00}

type imageBody struct {
	ID          int    `json:"id"`
	PlantID     int    `json:"plant_id"`
	URL         string `json:"url"`
	Description string `json:"description"`
	TakenOn     string `json:"taken_on"`
}

type formPart struct {
	name, filename string
	data           []byte
}

func multipartBody(t *testing.T, parts ...formPart) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.filename != "" {
			w, err := mw.CreateFormFile(p.name, p.filename)
			require.NoError(t, err)
			_, err = w.Write(p.data)
			require.NoError(t, err)
			continue
		}
		require.NoError(t, mw.WriteField(p.name, string(p.data)))
	}
	require.NoError(t, mw.Close())
	return buf.Bytes(), mw.FormDataContentType()
}

func (f fixture) upload(t *testing.T, key string, plantID int, extra map[string]string, parts ...formPart) response {
	t.Helper()
	body, ctype := multipartBody(t, parts...)
	headers := bearer(key)
	headers["Content-Type"] = ctype
	for k, v := range extra {
		headers[k] = v
	}
	return f.cc.do(http.MethodPost, "/plants/"+itoa(plantID)+"/images", headers, body)
}

// storedFile maps an image URL back to its file on disk.
func storedFile(f fixture, img imageBody) string {
	return filepath.Join(f.server.UploadDir, "plants", filepath.Base(img.URL))
}

func TestImages(t *testing.T) {
	t.Parallel()
	checkImages(t, newFixture(t))
}

func checkImages(t *testing.T, f fixture) {
	db := f.server.DB
	strain := testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S")
	zone := testutil.SeedZone(t, db, "Tent")
	plant := testutil.SeedPlant(t, db, "A", strain, zone)
	read, write := f.key(t, "read"), f.key(t, "write")

	// The stored extension comes from the content, never the filename.
	resp := f.upload(t, write, plant, nil,
		formPart{name: "image", filename: "leaf.html", data: jpeg},
		formPart{name: "description", data: []byte("week 3")},
		formPart{name: "taken_on", data: []byte("2026-11-01")})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	first := decode[imageBody](t, resp.Body)
	assert.Equal(t, "/api/v1/images/"+itoa(first.ID), resp.Header.Get("Location"))
	assert.Equal(t, plant, first.PlantID)
	assert.Equal(t, "week 3", first.Description)
	assert.Equal(t, "2026-11-01", first.TakenOn)
	assert.True(t, strings.HasSuffix(first.URL, ".jpg"), first.URL)
	assert.FileExists(t, storedFile(f, first))

	// No day given: today in the instance zone (the fixed clock is
	// 2026-11-10 22:00 in New York).
	resp = f.upload(t, write, plant, nil, formPart{name: "image", filename: "b.jpg", data: jpeg})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	second := decode[imageBody](t, resp.Body)
	assert.Equal(t, "2026-11-10", second.TakenOn)
	assert.Equal(t, "", second.Description)

	// Newest first, paged.
	resp = f.cc.get("/plants/"+itoa(plant)+"/images?limit=1", read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	page := decode[struct {
		Items      []imageBody `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, second.ID, page.Items[0].ID)
	require.NotNil(t, page.NextCursor)
	resp = f.cc.get("/plants/"+itoa(plant)+"/images?limit=1&cursor="+*page.NextCursor, read)
	page = decode[struct {
		Items      []imageBody `json:"items"`
		NextCursor *string     `json:"next_cursor"`
	}](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, first.ID, page.Items[0].ID)
	assert.Nil(t, page.NextCursor)

	resp = f.cc.get("/images/"+itoa(first.ID), read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, first, decode[imageBody](t, resp.Body))

	// Bad uploads keep nothing.
	before := filesIn(t, f)
	for name, parts := range map[string][]formPart{
		"not an image":  {{name: "image", filename: "a.jpg", data: []byte("plain text, not a picture")}},
		"no image":      {{name: "description", data: []byte("x")}},
		"two images":    {{name: "image", filename: "a.jpg", data: jpeg}, {name: "image", filename: "b.jpg", data: jpeg}},
		"unknown field": {{name: "image", filename: "a.jpg", data: jpeg}, {name: "caption", data: []byte("x")}},
		"bad day":       {{name: "image", filename: "a.jpg", data: jpeg}, {name: "taken_on", data: []byte("Nov 1")}},
	} {
		resp = f.upload(t, write, plant, nil, parts...)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, resp.Body)
	}
	resp = f.upload(t, write, plant, nil, formPart{name: "image", filename: "a.jpg", data: []byte("text")})
	env := requireErrorEnvelope(t, resp.Body)
	assert.Contains(t, env.Error.Details["fields"], "image")
	assert.Equal(t, before, filesIn(t, f), "a rejected upload leaves no file behind")

	resp = f.upload(t, write, 9999, nil, formPart{name: "image", filename: "a.jpg", data: jpeg})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp = f.upload(t, read, plant, nil, formPart{name: "image", filename: "a.jpg", data: jpeg})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Delete takes the file and any decorated copy with it.
	file := storedFile(f, first)
	processed := strings.TrimSuffix(file, ".jpg") + ".processed.jpg"
	require.NoError(t, os.WriteFile(processed, jpeg, 0o600))
	resp = f.cc.do(http.MethodDelete, "/images/"+itoa(first.ID), bearer(write), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	assert.NoFileExists(t, file)
	assert.NoFileExists(t, processed)
	assert.Equal(t, http.StatusNotFound, f.cc.get("/images/"+itoa(first.ID), read).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.do(http.MethodDelete, "/images/"+itoa(first.ID), bearer(write), nil).StatusCode)

	// Deleting the plant deletes its image files.
	resp = f.cc.do(http.MethodDelete, "/plants/"+itoa(plant), bearer(f.key(t, "admin")), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "%s", resp.Body)
	assert.NoFileExists(t, storedFile(f, second))
}

// An upload retried with the same Idempotency-Key stores one image, and
// the key's body handling must not cut a large upload short.
func TestImages_IdempotentLargeUpload(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	db := f.server.DB
	plant := testutil.SeedPlant(t, db, "A",
		testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Tent"))
	write := f.key(t, "write")

	big := append(append([]byte{}, jpeg...), bytes.Repeat([]byte{0}, 3<<20)...)
	idem := map[string]string{"Idempotency-Key": "photo-1"}
	resp := f.upload(t, write, plant, idem, formPart{name: "image", filename: "a.jpg", data: big})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	img := decode[imageBody](t, resp.Body)
	info, err := os.Stat(storedFile(f, img))
	require.NoError(t, err)
	assert.Equal(t, int64(len(big)), info.Size())

	resp = f.upload(t, write, plant, idem, formPart{name: "image", filename: "a.jpg", data: big})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "true", resp.Header.Get("Idempotent-Replayed"))
	assert.Equal(t, img.ID, decode[imageBody](t, resp.Body).ID)
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM plant_images").Scan(&n))
	assert.Equal(t, 1, n)

	// The same key with a different file is a different request.
	resp = f.upload(t, write, plant, idem, formPart{name: "image", filename: "a.jpg", data: jpeg})
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "%s", resp.Body)
}

func filesIn(t *testing.T, f fixture) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.server.UploadDir, "plants"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}
