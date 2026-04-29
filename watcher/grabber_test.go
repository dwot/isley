package watcher

// +parallel:serial — config.RestoreInProgress mutation
//
// This file holds grabber tests that mutate the process-global
// config.RestoreInProgress atomic to true. Phase 4.3 of TEST_PLAN_2.md
// lifted the streamBackoff package-global into the new Grabber struct,
// so the only remaining cliff is the RestoreInProgress flag — a
// process-wide toggle the audit decided to leave as-is. Sibling file
// grabber_parallel_test.go holds the parallel-safe tests.
//
// Tests in this file cannot call t.Parallel(): they flip
// RestoreInProgress to true and would race the parallel-safe tests'
// implicit assumption that the flag is false during their iteration.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
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

// runGrabOnce drives a *Grabber's Run loop with a short ctx timeout so
// one iteration runs and the function returns cleanly via ctx.Done.
// The supplied grabber's store is forced to a 60s grab interval so the
// second iteration cannot start before the ctx expires.
func runGrabOnce(t *testing.T, g *Grabber) {
	t.Helper()
	g.Store.SetStreamGrabInterval(60) // seconds — well past the ctx deadline

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		g.Run(ctx)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Grabber.Run did not return within 10 seconds of ctx cancellation")
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

// TestGrabber_RespectsRestoreInProgress confirms the entire grab pass
// is skipped when config.RestoreInProgress is true. No streams should
// be fetched (so no frames should be written).
func TestGrabber_RespectsRestoreInProgress(t *testing.T) {
	silenceWatcherLogger()
	prev := config.RestoreInProgress.Load()
	t.Cleanup(func() { config.RestoreInProgress.Store(prev) })

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

	g := NewGrabber(store, t.TempDir())
	runGrabOnce(t, g)

	assert.Zero(t, atomic.LoadInt32(&hits), "no HTTP hits should occur while RestoreInProgress is true")
}
