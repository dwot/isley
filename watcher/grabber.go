package watcher

import (
	"context"
	"fmt"
	"isley/config"
	"isley/logger"
	"isley/utils"
	"path/filepath"
	"time"
)

// streamBackoff tracks consecutive failures per stream ID so we can delay
// retries for unresponsive streams instead of hammering them every interval.
var streamBackoff = map[uint]int{}

const (
	maxBackoffMultiplier      = 10 // cap at 10× the normal interval
	defaultStreamGrabInterval = 60 // seconds between stream image captures
)

// Grab runs the stream-image capture loop until ctx is cancelled. It
// reads enabled/interval/streams from the supplied per-engine
// *config.Store rather than package globals so multiple engines (e.g.
// parallel tests) can run side-by-side without colliding on config.
//
// frameDir is the on-disk directory frame snapshots are written into.
// Production main.go threads the engine's resolved StreamDir/FrameDir
// here; tests pass t.TempDir() so per-test grabber writes stay
// isolated. An empty frameDir falls back to "uploads/streams" — the
// historical default — so callers (e.g. legacy entry points) still
// behave the same.
func Grab(ctx context.Context, store *config.Store, frameDir string) {
	logger.Log.Info("Started Stream Grabber")
	if frameDir == "" {
		frameDir = filepath.Join("uploads", "streams")
	}
	interval := defaultStreamGrabInterval

	for {
		if !config.RestoreInProgress.Load() && store.StreamGrabEnabled() == 1 {
			for _, stream := range store.Streams() {
				// If this stream has consecutive failures, skip cycles proportionally
				if failures, ok := streamBackoff[stream.ID]; ok && failures > 0 {
					backoff := failures
					if backoff > maxBackoffMultiplier {
						backoff = maxBackoffMultiplier
					}
					// Decrement the skip counter each cycle; when it reaches 0 we retry
					streamBackoff[stream.ID] = failures - 1
					if failures > 1 {
						logger.Log.WithField("stream", stream.Name).Debugf(
							"Backing off stream grab (%d cycles remaining)", failures-1)
						continue
					}
				}

				logger.Log.WithField("stream", stream.Name).Debug("Grabbing stream image")
				latestFileName := fmt.Sprintf("stream_%d_latest%s", stream.ID, filepath.Ext(".jpg"))
				latestSavePath := filepath.Join(frameDir, latestFileName)
				utils.CreateFolderIfNotExists(frameDir)

				if err := utils.GrabWebcamImage(stream.URL, latestSavePath); err != nil {
					// Record failure and set skip cycles for exponential backoff
					prev := streamBackoff[stream.ID]
					next := prev + 1
					if next > maxBackoffMultiplier {
						next = maxBackoffMultiplier
					}
					streamBackoff[stream.ID] = next
					logger.Log.WithField("stream", stream.Name).WithError(err).Warnf(
						"Stream grab failed, backing off %d cycles", next)
				} else {
					// Success — reset backoff
					delete(streamBackoff, stream.ID)
					logger.Log.WithField("stream", stream.Name).Debug("Stream image saved")
				}
			}
		}
		if grab := store.StreamGrabInterval(); grab > 0 {
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
