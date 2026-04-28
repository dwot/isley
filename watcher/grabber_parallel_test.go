package watcher

// Parallel-safe grabber tests. Phase 4.3 of TEST_PLAN_2.md lifted the
// package-global streamBackoff map into the Grabber struct, so each
// test owns its own *Grabber and per-stream backoff counter. None of
// these tests mutate config.RestoreInProgress to true; they all read
// the default-false flag, so they can run concurrently with each other
// (Go pauses parallel tests until the package's serial tests finish,
// so the sibling TestGrabber_RespectsRestoreInProgress can never
// overlap them).

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/model/types"
)

// TestGrabber_HappyPathWritesFrame exercises the success path: a
// stream pointing at a fake server that returns a PNG should produce
// a file at <frameDir>/stream_<id>_latest.jpg.
func TestGrabber_HappyPathWritesFrame(t *testing.T) {
	t.Parallel()
	silenceWatcherLogger()
	frameDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes(t))
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(101)
	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: streamID, Name: "Tent A Cam", URL: srv.URL + "/snap.png"}})
	store.SetStreamGrabEnabled(1)

	g := NewGrabber(store, frameDir)
	runGrabOnce(t, g)

	saved := filepath.Join(frameDir, "stream_101_latest.jpg")
	_, err := os.Stat(saved)
	require.NoError(t, err, "frame should be saved at %s", saved)

	// Successful grabs delete the backoff entry (or leave it absent).
	_, stillBackingOff := g.backoff[streamID]
	assert.False(t, stillBackingOff, "successful grab must clear/leave-empty the backoff counter")
}

// TestGrabber_FailureIncrementsBackoff exercises the failure path: a
// non-image response causes GrabWebcamImage to return an error, which
// should bump the per-grabber backoff counter for that stream.
func TestGrabber_FailureIncrementsBackoff(t *testing.T) {
	t.Parallel()
	silenceWatcherLogger()
	frameDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(102)
	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: streamID, Name: "Bad Cam", URL: srv.URL + "/feed"}})
	store.SetStreamGrabEnabled(1)

	g := NewGrabber(store, frameDir)
	runGrabOnce(t, g)

	got, ok := g.backoff[streamID]
	require.True(t, ok, "failure should produce a backoff entry")
	assert.Equal(t, 1, got, "first failure should set backoff = 1")
}

// TestGrabber_RespectsStreamGrabDisabled confirms the entire grab pass
// is skipped when StreamGrabEnabled is 0.
func TestGrabber_RespectsStreamGrabDisabled(t *testing.T) {
	t.Parallel()
	silenceWatcherLogger()
	frameDir := t.TempDir()

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: 104, Name: "Disabled Test", URL: srv.URL + "/snap.png"}})
	store.SetStreamGrabEnabled(0) // disabled

	g := NewGrabber(store, frameDir)
	runGrabOnce(t, g)

	assert.Zero(t, atomic.LoadInt32(&hits))
}

// TestGrabber_BackoffSkipsCycles exercises the cycle-skip logic: when
// a stream already has a backoff > 1, the iteration should NOT make a
// new HTTP request and should instead decrement the counter.
func TestGrabber_BackoffSkipsCycles(t *testing.T) {
	t.Parallel()
	silenceWatcherLogger()
	frameDir := t.TempDir()

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(105)
	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: streamID, Name: "Cooldown", URL: srv.URL + "/snap.png"}})
	store.SetStreamGrabEnabled(1)

	g := NewGrabber(store, frameDir)
	g.backoff[streamID] = 3 // 3 skip cycles remaining
	runGrabOnce(t, g)

	assert.Zero(t, atomic.LoadInt32(&hits), "stream with backoff > 1 should not be requested this cycle")
	assert.Equal(t, 2, g.backoff[streamID], "backoff counter must decrement by 1")
}
