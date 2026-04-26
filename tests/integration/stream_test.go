package integration

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// streamFixture sets up a zone, an httptest fake stream server, and an
// API key. The fake server responds 200 OK to any GET so that
// AddStreamHandler's call to utils.GrabWebcamImage succeeds without
// hitting the network.
type streamFixture struct {
	APIKey string
	URL    string
	server *httptest.Server
}

func seedStreamHTTP(t *testing.T, db *sql.DB) streamFixture {
	t.Helper()
	testutil.SeedZone(t, db, "Z")

	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-bytes"))
	}))
	t.Cleanup(fake.Close)

	return streamFixture{
		APIKey: testutil.SeedAPIKey(t, db, "test-stream-key"),
		URL:    fake.URL,
		server: fake,
	}
}

// ---------------------------------------------------------------------------
// POST /streams
// ---------------------------------------------------------------------------

func TestStream_AddHappyPath(t *testing.T) {
	resetRateLimit(t)
	// Scope filesystem writes from AddStreamHandler's GrabWebcamImage
	// call to a tempdir so we don't pollute the repo's uploads/.
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/streams", fix.APIKey, map[string]interface{}{
		"stream_name": "Tent Cam",
		"url":         fix.URL,
		"zone_id":     "1",
		"visible":     true,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var got struct {
		ID int `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.NotZero(t, got.ID)

	var name, url string
	require.NoError(t, db.QueryRow(
		`SELECT name, url FROM streams WHERE id = $1`, got.ID,
	).Scan(&name, &url))
	assert.Equal(t, "Tent Cam", name)
	assert.Equal(t, fix.URL, url)
}

func TestStream_AddRejectsBadURL(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	c := server.NewClient(t)
	cases := []struct {
		name, url string
	}{
		{"empty", ""},
		{"missing scheme", "no-scheme"},
		{"unsupported scheme", "ftp://server/stream"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := apiPostJSON(t, c, "/streams", fix.APIKey, map[string]interface{}{
				"stream_name": "Bad URL Stream",
				"url":         tc.url,
				"zone_id":     "1",
				"visible":     true,
			})
			resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestStream_AddRejectsEmptyName(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	c := server.NewClient(t)
	resp := apiPostJSON(t, c, "/streams", fix.APIKey, map[string]interface{}{
		"stream_name": "",
		"url":         fix.URL,
		"zone_id":     "1",
		"visible":     true,
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// ---------------------------------------------------------------------------
// PUT /streams/:id
// ---------------------------------------------------------------------------

func TestStream_UpdateRenames(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO streams (name, url, zone_id, visible) VALUES ('Old Name', $1, 1, 1)`,
		fix.URL,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiPutJSON(t, c, "/streams/"+strconv.FormatInt(id, 10), fix.APIKey, map[string]interface{}{
		"stream_name": "New Name",
		"url":         fix.URL,
		"zone_id":     1,
		"visible":     false,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var name string
	var visible int
	require.NoError(t, db.QueryRow(
		`SELECT name, visible FROM streams WHERE id = $1`, id,
	).Scan(&name, &visible))
	assert.Equal(t, "New Name", name)
	assert.Equal(t, 0, visible, "visible=false should persist as 0")
}

// ---------------------------------------------------------------------------
// DELETE /streams/:id
// ---------------------------------------------------------------------------

func TestStream_DeleteRemovesRow(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	res, err := db.Exec(
		`INSERT INTO streams (name, url, zone_id, visible) VALUES ('Doomed', $1, 1, 1)`,
		fix.URL,
	)
	require.NoError(t, err)
	id, _ := res.LastInsertId()

	c := server.NewClient(t)
	resp := apiDelete(t, c, "/streams/"+strconv.FormatInt(id, 10), fix.APIKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM streams WHERE id = $1`, id).Scan(&n))
	assert.Zero(t, n)
}

// ---------------------------------------------------------------------------
// GET /streams (basic route — session-gated, returns map keyed by zone)
// ---------------------------------------------------------------------------

func TestStream_GetByZoneGroupsByZone(t *testing.T) {
	resetRateLimit(t)
	t.Chdir(t.TempDir())

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	fix := seedStreamHTTP(t, db)

	// Add a second zone + a stream in each.
	mustExecRow(t, db, `INSERT INTO zones (id, name) VALUES (2, 'Zone B')`)
	mustExecRow(t, db, `INSERT INTO streams (name, url, zone_id, visible) VALUES ('Cam A', $1, 1, 1)`, fix.URL)
	mustExecRow(t, db, `INSERT INTO streams (name, url, zone_id, visible) VALUES ('Cam B', $1, 2, 1)`, fix.URL)

	testutil.SeedAdmin(t, db, "stream-pw")
	c := server.LoginAsAdmin(t, "stream-pw")

	resp := c.Get("/streams")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string][]map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got["Z"], 1)
	require.Len(t, got["Zone B"], 1)
	assert.Equal(t, "Cam A", got["Z"][0]["name"])
	assert.Equal(t, "Cam B", got["Zone B"][0]["name"])
}
