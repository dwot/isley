package watcher

import (
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

const maxBackoffMultiplier = 10 // cap at 10× the normal interval

func Grab() {
	logger.Log.Info("Started Stream Grabber")
	interval := 60

	for {
		if config.StreamGrabEnabled == 1 {
			for _, stream := range config.Streams {
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
				latestSavePath := filepath.Join("uploads", "streams", latestFileName)
				utils.CreateFolderIfNotExists(filepath.Join("uploads", "streams"))

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
		if config.StreamGrabInterval > 0 {
			interval = config.StreamGrabInterval
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
