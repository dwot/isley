package watcher

import (
	"fmt"
	"isley/config"
	"isley/logger"
	"isley/utils"
	"path/filepath"
	"time"
)

func Grab() {
	logger.Log.Info("Started Stream Grabber")
	interval := 60

	for {
		if config.StreamGrabEnabled == 1 {
			for _, stream := range config.Streams {
				logger.Log.WithField("stream", stream.Name).Debug("Grabbing stream image")
				latestFileName := fmt.Sprintf("stream_%d_latest%s", stream.ID, filepath.Ext(".jpg"))
				latestSavePath := filepath.Join("uploads", "streams", latestFileName)
				utils.CreateFolderIfNotExists(filepath.Join("uploads", "streams"))
				utils.GrabWebcamImage(stream.URL, latestSavePath)
				logger.Log.WithField("stream", stream.Name).Debug("Stream image saved")
			}
		}
		if config.StreamGrabInterval > 0 {
			interval = config.StreamGrabInterval
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
