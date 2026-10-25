package integration

import (
	"io/fs"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

// TestStatic_CacheHeaders verifies embedded assets are served with the
// validators browsers and CDNs need to cache them, and that a conditional
// request against the ETag is answered with 304 instead of the body.
// Before these headers existed every page navigation refetched every
// script and stylesheet through the reverse proxy.
func TestStatic_CacheHeaders(t *testing.T) {
	t.Parallel()

	db := testutil.NewTestDB(t)
	server := testutil.NewTestServer(t, db)
	c := server.NewClient(t)

	for _, path := range []string{"/static/js/main.js", "/fonts/" + firstFont(t)} {
		resp := c.Get(path)
		resp.Body.Close()
		require.Equalf(t, http.StatusOK, resp.StatusCode, "GET %s", path)

		etag := resp.Header.Get("ETag")
		assert.NotEmptyf(t, etag, "%s: ETag", path)
		assert.Equalf(t, "public, max-age=3600", resp.Header.Get("Cache-Control"), "%s: Cache-Control", path)
		assert.NotEmptyf(t, resp.Header.Get("Last-Modified"), "%s: Last-Modified", path)

		// ETag is stable across requests for the same (immutable) bundle.
		again := c.Get(path)
		again.Body.Close()
		assert.Equalf(t, etag, again.Header.Get("ETag"), "%s: ETag must be stable", path)

		req, err := http.NewRequest(http.MethodGet, c.BaseURL+path, nil)
		require.NoError(t, err)
		req.Header.Set("If-None-Match", etag)
		cond, err := c.Do(req)
		require.NoError(t, err)
		cond.Body.Close()
		assert.Equalf(t, http.StatusNotModified, cond.StatusCode, "%s: If-None-Match should 304", path)
	}

	// Missing assets still 404 and never advertise a cache policy.
	missing := c.Get("/static/js/does-not-exist.js")
	missing.Body.Close()
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)
	assert.Empty(t, missing.Header.Get("ETag"))
}

// firstFont returns the basename of one bundled font so the /fonts route
// is exercised without hard-coding a filename that may be renamed.
func firstFont(t *testing.T) string {
	t.Helper()
	entries, err := fs.ReadDir(testutil.RepoFS(t), "utils/fonts")
	require.NoError(t, err)
	for _, e := range entries {
		if !e.IsDir() {
			return e.Name()
		}
	}
	t.Fatal("no bundled fonts found under utils/fonts")
	return ""
}
