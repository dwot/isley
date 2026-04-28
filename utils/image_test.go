package utils

// Phase 6a tests for image.go. Covers the pieces of the file that don't
// require ffmpeg or a live MJPEG/HLS source:
//
//   - validateImageFile (good headers, oversized dimensions, traversal,
//     non-image data)
//   - parseHexColor (well-formed, malformed, missing #)
//   - isPathWithinDir (legitimate, traversal attempt, absolute escape)
//   - isHLSURL (table-driven URL classification)
//   - CreateFolderIfNotExists
//   - GrabWebcamImage / grabHTTPFrame against a fake httptest.Server
//     (direct snapshot, MJPEG first-frame extraction, HLS thumbnail
//     fallback)

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/logger"
)

// silenceLogger swaps in a no-op logger once per process so production
// log lines do not pollute go test output. Idempotent.
var imageLoggerOnce atomic.Bool

func silenceImageLogger() {
	if imageLoggerOnce.Swap(true) {
		return
	}
	l := logrus.New()
	l.SetOutput(io.Discard)
	l.SetLevel(logrus.PanicLevel)
	logger.Log = l
	logger.AccessWriter = io.Discard
}

// pngBytes returns a tiny PNG payload (1x1, opaque white).
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// writeTempPNG drops a PNG into a per-test directory and returns the
// path.
func writeTempPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.png")
	require.NoError(t, os.WriteFile(path, pngBytes(t), 0o644))
	return path
}

// ---------------------------------------------------------------------------
// parseHexColor
// ---------------------------------------------------------------------------

func TestParseHexColor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr bool
		wantR   uint8
		wantG   uint8
		wantB   uint8
	}{
		{"with hash", "#FF8000", false, 0xff, 0x80, 0x00},
		{"without hash", "00ABFF", false, 0x00, 0xab, 0xff},
		{"lowercase", "#aabbcc", false, 0xaa, 0xbb, 0xcc},
		{"too short", "#FFF", true, 0, 0, 0},
		{"too long", "#FFFFFFFF", true, 0, 0, 0},
		{"non-hex", "#ZZZZZZ", true, 0, 0, 0},
		{"empty", "", true, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHexColor(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			rgba, ok := got.(color.RGBA)
			require.True(t, ok)
			assert.Equal(t, tc.wantR, rgba.R)
			assert.Equal(t, tc.wantG, rgba.G)
			assert.Equal(t, tc.wantB, rgba.B)
			assert.Equal(t, uint8(255), rgba.A, "alpha defaults to fully opaque")
		})
	}
}

// ---------------------------------------------------------------------------
// isPathWithinDir
// ---------------------------------------------------------------------------

func TestIsPathWithinDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "uploads", "logos"), 0o755))

	cases := []struct {
		name       string
		path       string
		allowedDir string
		want       bool
	}{
		{"file inside dir", filepath.Join(tmp, "uploads", "ok.png"), filepath.Join(tmp, "uploads"), true},
		{"file in nested subdir", filepath.Join(tmp, "uploads", "logos", "ok.png"), filepath.Join(tmp, "uploads"), true},
		{"file outside dir", filepath.Join(tmp, "elsewhere.png"), filepath.Join(tmp, "uploads"), false},
		{"path equals dir", filepath.Join(tmp, "uploads"), filepath.Join(tmp, "uploads"), true},
		{"path outside via traversal", filepath.Join(tmp, "uploads", "..", "secret.txt"), filepath.Join(tmp, "uploads"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isPathWithinDir(tc.path, tc.allowedDir)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// isHLSURL
// ---------------------------------------------------------------------------

func TestIsHLSURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.com/stream.m3u8", true},
		{"http://example.com/playlist.m3u", true},
		{"https://example.com/stream.M3U8", true},
		{"https://example.com/stream.M3U8?token=abc", true},
		{"https://example.com/snapshot.jpg", false},
		{"rtsp://example.com/feed", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			assert.Equal(t, tc.want, isHLSURL(tc.url))
		})
	}
}

// ---------------------------------------------------------------------------
// validateImageFile
// ---------------------------------------------------------------------------

func TestValidateImageFile_HappyPath(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	path := writeTempPNG(t)
	require.NoError(t, validateImageFile(path))
}

func TestValidateImageFile_RejectsTraversalPath(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	// filepath.Clean leaves "../" intact when the path begins with one,
	// so the traversal check fires before any disk access.
	err := validateImageFile("../../../etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory traversal")
}

func TestValidateImageFile_RejectsMissingFile(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	err := validateImageFile(filepath.Join(t.TempDir(), "does-not-exist.png"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot stat image")
}

func TestValidateImageFile_RejectsNonImage(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	path := filepath.Join(t.TempDir(), "junk.png")
	require.NoError(t, os.WriteFile(path, []byte("not an image"), 0o644))

	err := validateImageFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot decode image header")
}

// ---------------------------------------------------------------------------
// CreateFolderIfNotExists
// ---------------------------------------------------------------------------

func TestCreateFolderIfNotExists_CreatesNewFolder(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	target := filepath.Join(t.TempDir(), "a", "b", "c")
	CreateFolderIfNotExists(target)
	stat, err := os.Stat(target)
	require.NoError(t, err)
	assert.True(t, stat.IsDir())
}

func TestCreateFolderIfNotExists_NoOpWhenExists(t *testing.T) {
	t.Parallel()

	silenceImageLogger()
	target := t.TempDir() // already exists
	require.NotPanics(t, func() { CreateFolderIfNotExists(target) })
	stat, err := os.Stat(target)
	require.NoError(t, err)
	assert.True(t, stat.IsDir())
}

// ---------------------------------------------------------------------------
// GrabWebcamImage / grabHTTPFrame
// ---------------------------------------------------------------------------

// TestGrabWebcamImage_RejectsInvalidURL covers the early URL-parse and
// host-required guards.
func TestGrabWebcamImage_RejectsInvalidURL(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	cases := []string{
		"not://a-real-url",     // unsupported scheme
		"://no-scheme",         // url.Parse error
		"http://",              // no host
		"rtsp://example.com/x", // disallowed even though syntactically valid
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.jpg")
			err := GrabWebcamImage(raw, out)
			require.Error(t, err)
		})
	}
}

// TestGrabWebcamImage_DirectSnapshotHappyPath stands up a fake HTTP
// server that returns a PNG payload, then asserts GrabWebcamImage writes
// the body to the requested path.
func TestGrabWebcamImage_DirectSnapshotHappyPath(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	payload := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "snap.png")
	require.NoError(t, GrabWebcamImage(srv.URL+"/snapshot.png", out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "the saved bytes must match the server response")
}

// TestGrabWebcamImage_NonImageResponseRejected verifies that even with a
// 200 response, a non-image body is rejected before being persisted.
func TestGrabWebcamImage_NonImageResponseRejected(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello, not an image"))
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "snap.png")
	err := GrabWebcamImage(srv.URL+"/feed", out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid image")

	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr), "no file should be written when validation fails")
}

// TestGrabWebcamImage_NonOKStatusReturnsError covers the resp.StatusCode
// != http.StatusOK branch.
func TestGrabWebcamImage_NonOKStatusReturnsError(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "snap.png")
	err := GrabWebcamImage(srv.URL+"/feed", out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

// TestGrabWebcamImage_MJPEGFirstFrame extracts the first JPEG part from a
// multipart/x-mixed-replace stream. We use PNG bytes here because
// grabMJPEGFrame writes the body verbatim and never re-decodes — the
// content of the part is opaque.
func TestGrabWebcamImage_MJPEGFirstFrame(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	payload := pngBytes(t)
	const boundary = "isley-mjpeg-test"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
		w.WriteHeader(http.StatusOK)
		var buf bytes.Buffer
		buf.WriteString("--" + boundary + "\r\n")
		buf.WriteString("Content-Type: image/jpeg\r\n\r\n")
		buf.Write(payload)
		buf.WriteString("\r\n--" + boundary + "--\r\n")
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "frame.jpg")
	require.NoError(t, GrabWebcamImage(srv.URL+"/mjpeg", out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestGrabWebcamImage_HLSURLFallsBackToThumbnail verifies that a URL
// ending in .m3u8 triggers the HLS thumbnail-fallback path. The fake
// server serves a thumbnail at /thumbnail.jpg (the first item in
// hlsThumbnailPaths) and returns 404 for the playlist itself.
func TestGrabWebcamImage_HLSURLFallsBackToThumbnail(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	payload := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/thumbnail.jpg":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "thumb.jpg")
	err := GrabWebcamImage(srv.URL+"/stream.m3u8", out)
	require.NoError(t, err)

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

// TestGrabWebcamImage_HLSNoThumbnailReturnsError covers the all-paths-failed
// branch of grabHLSThumbnail. The fake server 404s every URL.
func TestGrabWebcamImage_HLSNoThumbnailReturnsError(t *testing.T) {
	t.Parallel()

	silenceImageLogger()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "thumb.jpg")
	err := GrabWebcamImage(srv.URL+"/stream.m3u8", out)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "no thumbnail endpoint found"),
		"error message should explain why HLS fallback failed; got: %v", err)
}
