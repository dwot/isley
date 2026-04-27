package watcher

// +parallel:serial — streamBackoff package-global map and
// config.RestoreInProgress atomic
//
// Phase 6a tests for grabber.go. The Grab loop runs forever in
// production; tests drive it with a short-lived context so it executes
// exactly one iteration and then exits via ctx.Done.
//
// These tests cannot call t.Parallel(): every test mutates the
// package-level streamBackoff map (snapshotGrabState's
// snapshot/restore would race two parallel tests' writes) and the
// config.RestoreInProgress atomic (a process-global the grabber reads
// each iteration). Phase 3 of TEST_PLAN_2.md cleared the t.Chdir
// blocker by passing frameDir into Grab; the remaining serial cause
// is documented above for the audit Phase 4 will run. Lifting it
// would require extracting streamBackoff into a per-grabber instance
// — out of scope for Phase 3.

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

// snapshotGrabState captures the streamBackoff map and the
// RestoreInProgress flag (the only process-globals Grab still touches
// after the Phase 2 ConfigStore refactor) and returns a restorer the
// caller can defer. Stream/enabled/interval state lives on a per-test
// *config.Store now, so each test owns its own snapshot.
func snapshotGrabState(t *testing.T) func() {
	t.Helper()
	prevRestore := config.RestoreInProgress.Load()
	prevBackoff := make(map[uint]int, len(streamBackoff))
	for k, v := range streamBackoff {
		prevBackoff[k] = v
	}
	return func() {
		config.RestoreInProgress.Store(prevRestore)
		for k := range streamBackoff {
			delete(streamBackoff, k)
		}
		for k, v := range prevBackoff {
			streamBackoff[k] = v
		}
	}
}

// runGrabOnce calls Grab(ctx, store, frameDir) with a short ctx
// timeout so one iteration runs and the function returns cleanly via
// ctx.Done. The supplied store carries enabled/interval/streams; we
// force the interval to 60s so the second iteration cannot start
// before the ctx expires. frameDir is the per-test directory the
// grabber writes frames into; tests pass t.TempDir() to keep writes
// isolated from each other and from the repo's uploads/ tree.
func runGrabOnce(t *testing.T, store *config.Store, frameDir string) {
	t.Helper()
	store.SetStreamGrabInterval(60) // seconds — well past the ctx deadline

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Grab(ctx, store, frameDir)
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestGrabber_HappyPathWritesFrame exercises the success path: a stream
// pointing at a fake server that returns a PNG should produce a file at
// <frameDir>/stream_<id>_latest.jpg.
func TestGrabber_HappyPathWritesFrame(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGrabState(t)()
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
	config.RestoreInProgress.Store(false)
	delete(streamBackoff, streamID) // start clean — no skip cycles in effect

	runGrabOnce(t, store, frameDir)

	saved := filepath.Join(frameDir, "stream_101_latest.jpg")
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
	defer snapshotGrabState(t)()
	frameDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	const streamID = uint(102)
	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: streamID, Name: "Bad Cam", URL: srv.URL + "/feed"}})
	store.SetStreamGrabEnabled(1)
	config.RestoreInProgress.Store(false)
	delete(streamBackoff, streamID)

	runGrabOnce(t, store, frameDir)

	got, ok := streamBackoff[streamID]
	require.True(t, ok, "failure should produce a backoff entry")
	assert.Equal(t, 1, got, "first failure should set backoff = 1")
}

// TestGrabber_RespectsRestoreInProgress confirms the entire grab pass
// is skipped when config.RestoreInProgress is true. No streams should be
// fetched (so no frames should be written).
func TestGrabber_RespectsRestoreInProgress(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGrabState(t)()
	frameDir := t.TempDir()

	hits := int32(0)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pngBytes(t))
	}))
	t.Cleanup(srv.Close)

	store := config.NewStore()
	store.SetStreams([]types.Stream{{ID: 103, Name: "Pause Test", URL: srv.URL + "/snap.png"}})
	store.SetStreamGrabEnabled(1)
	config.RestoreInProgress.Store(true) // block the iteration body

	runGrabOnce(t, store, frameDir)

	assert.Zero(t, atomic.LoadInt32(&hits), "no HTTP hits should occur while RestoreInProgress is true")
}

// TestGrabber_RespectsStreamGrabDisabled confirms the entire grab pass
// is skipped when StreamGrabEnabled is 0.
func TestGrabber_RespectsStreamGrabDisabled(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGrabState(t)()
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
	config.RestoreInProgress.Store(false)

	runGrabOnce(t, store, frameDir)

	assert.Zero(t, atomic.LoadInt32(&hits))
}

// TestGrabber_BackoffSkipsCycles exercises the cycle-skip logic: when a
// stream already has a backoff > 1, the iteration should NOT make a new
// HTTP request and should instead decrement the counter.
func TestGrabber_BackoffSkipsCycles(t *testing.T) {
	silenceWatcherLogger()
	defer snapshotGrabState(t)()
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
	config.RestoreInProgress.Store(false)
	streamBackoff[streamID] = 3 // 3 skip cycles remaining

	runGrabOnce(t, store, frameDir)

	assert.Zero(t, atomic.LoadInt32(&hits), "stream with backoff > 1 should not be requested this cycle")
	assert.Equal(t, 2, streamBackoff[streamID], "backoff counter must decrement by 1")
}
