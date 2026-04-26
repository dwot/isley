package watcher

// Phase 6a tests for grabber.go. The Grab loop runs forever in
// production; tests drive it with a short-lived context so it executes
// exactly one iteration and then exits via ctx.Done.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/config"
	"isley/logger"
	"isley/model/types"
)

// silenceWatcherLogger swaps the production logger for a discard logger
// once per process so log output does not pollute go test runs.
var grabberLoggerOnce atomic.Bool

func silenceWatcherLogger() {
	if grabberLoggerOnce.Swap(true) {
		return
	}
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.PanicLevel)
	logger.Log = l
}

// snapshotGlobals captures the current values of every config-package
// global Grab reads, plus the streamBackoff map, and returns a
// restorer the caller can defer.
func snapshotGlobals(t *testing.T) func() {
	t.Helper()
	prevStreams := config.Streams
	prevEnabled := config.StreamGrabEnabled
	prevInterval := config.StreamGrabInterval
	prevRestore := config.RestoreInProgress.Load()
	prevBackoff := make(map[uint]int, len(streamBackoff))
	for k, v := range streamBackoff {
		prevBackoff[k] = v
	}
	return func() {
		config.Streams = prevStreams
		config.StreamGrabEnabled = prevEnabled
		config.StreamGrabInterval = prevInterval
		config.RestoreInProgress.Store(prevRestore)
		for k := range streamBackoff {
			delete(streamBackoff, k)
		}
		for k, v := range prevBackoff {
			streamBackoff[k] = v
		}
	}
}

// runGrabOnce calls Grab(ctx) with a short ctx timeout so one iteration
// runs and the function returns cleanly via ctx.Done. Returns when Grab
// returns. The interval is set high enough that a second iteration
// cannot start before the context expires.
func runGrabOnce(t *testing.T) {
	t.Helper()
	config.StreamGrabInterval = 60 // seconds — well past the ctx deadline

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Grab(ctx)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Grab did not return within 10 seconds of ctx cancellation")
	}
}

// pngBytes builds a minimal valid PNG (1×1 white pixel) so the
// downstream GrabWebcamImage validator accepts it.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// chdirToTemp swaps the working directory to a per-test temp dir so
// uploads/streams writes do not leak into the repo. t.Chdir restores
// the prior CWD on cleanup.
func chdirToTemp(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestGrabber_HappyPathWritesFrame exercises the success path: a stream
// pointing at a fake server that returns a PNG should produce a file at
// uploads/streams/stream_<id>_latest.jpg.
func TestGrabber_HappyPathWritesFrame(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGlobals(t)()
	chdirToTemp(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes(t))
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(101)
	config.Streams = []types.Stream{{ID: streamID, Name: "Tent A Cam", URL: srv.URL + "/snap.png"}}
	config.StreamGrabEnabled = 1
	config.RestoreInProgress.Store(false)
	delete(streamBackoff, streamID) // start clean — no skip cycles in effect

	runGrabOnce(t)

	saved := filepath.Join("uploads", "streams", "stream_101_latest.jpg")
	_, err := os.Stat(saved)
	require.NoError(t, err, "frame should be saved at %s", saved)

	// Successful grabs delete the backoff entry (or leave it absent).
	_, stillBackingOff := streamBackoff[streamID]
	assert.False(t, stillBackingOff, "successful grab must clear/leave-empty the backoff counter")
}

// TestGrabber_FailureIncrementsBackoff exercises the failure path: a
// non-image response causes GrabWebcamImage to return an error, which
// should bump the streamBackoff counter for that stream.
func TestGrabber_FailureIncrementsBackoff(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGlobals(t)()
	chdirToTemp(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(102)
	config.Streams = []types.Stream{{ID: streamID, Name: "Bad Cam", URL: srv.URL + "/feed"}}
	config.StreamGrabEnabled = 1
	config.RestoreInProgress.Store(false)
	delete(streamBackoff, streamID)

	runGrabOnce(t)

	got, ok := streamBackoff[streamID]
	require.True(t, ok, "failure should produce a backoff entry")
	assert.Equal(t, 1, got, "first failure should set backoff = 1")
}

// TestGrabber_RespectsRestoreInProgress confirms the entire grab pass
// is skipped when config.RestoreInProgress is true. No streams should be
// fetched (so no upload directory should be created).
func TestGrabber_RespectsRestoreInProgress(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGlobals(t)()
	chdirToTemp(t)

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes(t))
	}))
	t.Cleanup(srv.Close)

	config.Streams = []types.Stream{{ID: 103, Name: "Pause Test", URL: srv.URL + "/snap.png"}}
	config.StreamGrabEnabled = 1
	config.RestoreInProgress.Store(true) // block the iteration body

	runGrabOnce(t)

	assert.Zero(t, atomic.LoadInt32(&hits), "no HTTP hits should occur while RestoreInProgress is true")
}

// TestGrabber_RespectsStreamGrabDisabled confirms the entire grab pass
// is skipped when StreamGrabEnabled is 0.
func TestGrabber_RespectsStreamGrabDisabled(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGlobals(t)()
	chdirToTemp(t)

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	config.Streams = []types.Stream{{ID: 104, Name: "Disabled Test", URL: srv.URL + "/snap.png"}}
	config.StreamGrabEnabled = 0 // disabled
	config.RestoreInProgress.Store(false)

	runGrabOnce(t)

	assert.Zero(t, atomic.LoadInt32(&hits))
}

// TestGrabber_BackoffSkipsCycles exercises the cycle-skip logic: when a
// stream already has a backoff > 1, the iteration should NOT make a new
// HTTP request and should instead decrement the counter.
func TestGrabber_BackoffSkipsCycles(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGlobals(t)()
	chdirToTemp(t)

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(105)
	config.Streams = []types.Stream{{ID: streamID, Name: "Cooldown", URL: srv.URL + "/snap.png"}}
	config.StreamGrabEnabled = 1
	config.RestoreInProgress.Store(false)
	streamBackoff[streamID] = 3 // 3 skip cycles remaining

	runGrabOnce(t)

	assert.Zero(t, atomic.LoadInt32(&hits), "stream with backoff > 1 should not be requested this cycle")
	assert.Equal(t, 2, streamBackoff[streamID], "backoff counter must decrement by 1")
}
