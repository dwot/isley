package watcher

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"isley/config"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

const (
	maxBackoffMultiplier      = 10 // cap at 10× the normal interval
	defaultStreamGrabInterval = 60 // seconds between stream image captures
)

// Grabber runs the stream-image capture loop. One instance per running
// app; constructed by main alongside the Watcher and driven from a
// goroutine. Tests construct their own per-test instance so the
// per-stream backoff counter and other per-iteration state stay
// isolated.
//
// Production callers must resolve frameDir via app.ResolvePathDefaults
// before calling NewGrabber; an empty frameDir is a programmer error
// and Run panics fast so the bug surfaces at the call site rather than
// after the goroutine has been running for hours writing to the
// process CWD.
type Grabber struct {
	Store    *config.Store
	FrameDir string

	mu      sync.Mutex
	backoff map[uint]int
}

// NewGrabber returns a Grabber wired to the supplied per-engine
// *config.Store and resolved frame directory.
func NewGrabber(store *config.Store, frameDir string) *Grabber {
	return &Grabber{
		Store:    store,
		FrameDir: frameDir,
		backoff:  map[uint]int{},
	}
}

// Run drives the stream-image capture loop until ctx is cancelled. It
// reads enabled/interval/streams from the per-engine *config.Store on
// every iteration so runtime settings changes take effect without
// restarting the goroutine.
func (g *Grabber) Run(ctx context.Context) {
	if g.FrameDir == "" {
		panic("watcher.Grabber.Run: FrameDir must be non-empty (resolve via app.ResolvePathDefaults)")
	}
	logger.Log.Info("Started Stream Grabber")
	interval := defaultStreamGrabInterval

	for {
		if !config.RestoreInProgress.Load() && g.Store.StreamGrabEnabled() == 1 {
			for _, stream := range g.Store.Streams() {
				g.processStream(stream)
			}
		}
		if grab := g.Store.StreamGrabInterval(); grab > 0 {
			interval = grab
		}

		// Wait for either the grab interval or context cancellation
		select {
		case <-ctx.Done():
			logger.Log.Info("Stream Grabber shutting down")
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// processStream handles one stream's grab attempt, reading and
// writing the per-stream backoff counter under g.mu. The mutex
// acquisition is short because each stream is independent;
// serialising across streams within one iteration is fine.
func (g *Grabber) processStream(stream types.Stream) {
	// If this stream has consecutive failures, skip cycles proportionally
	g.mu.Lock()
	failures, ok := g.backoff[stream.ID]
	if ok && failures > 0 {
		g.backoff[stream.ID] = failures - 1
	}
	g.mu.Unlock()

	if ok && failures > 0 {
		if failures > 1 {
			logger.Log.WithField("stream", stream.Name).Debugf(
				"Backing off stream grab (%d cycles remaining)", failures-1)
			return
		}
	}

	logger.Log.WithField("stream", stream.Name).Debug("Grabbing stream image")
	latestFileName := fmt.Sprintf("stream_%d_latest%s", stream.ID, filepath.Ext(".jpg"))
	latestSavePath := filepath.Join(g.FrameDir, latestFileName)
	utils.CreateFolderIfNotExists(g.FrameDir)

	if err := utils.GrabWebcamImage(stream.URL, latestSavePath); err != nil {
		// Record failure and set skip cycles for exponential backoff
		g.mu.Lock()
		next := g.backoff[stream.ID] + 1
		if next > maxBackoffMultiplier {
			next = maxBackoffMultiplier
		}
		g.backoff[stream.ID] = next
		g.mu.Unlock()
		logger.Log.WithField("stream", stream.Name).WithError(err).Warnf(
			"Stream grab failed, backing off %d cycles", next)
	} else {
		// Success — reset backoff
		g.mu.Lock()
		delete(g.backoff, stream.ID)
		g.mu.Unlock()
		logger.Log.WithField("stream", stream.Name).Debug("Stream image saved")
	}
}
