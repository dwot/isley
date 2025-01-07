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
		logger.Log.Info("Checking for stream grab")
		if config.StreamGrabEnabled == 1 {
			logger.Log.Info("Stream grab enabled")
			// Iterate config.Streams
			for _, stream := range config.Streams {
				logger.Log.WithField("stream", stream.Name).Info("Checking stream")
				// Grab stream data
				logger.Log.WithField("stream", stream.Name).Info("Grabbing stream data")
				// Generate a unique file path
				timestamp := time.Now().In(time.Local).UnixNano()
				fileName := fmt.Sprintf("stream_%s_image_%d_%s", stream.Name, timestamp, filepath.Ext(".jpg"))
				savePath := filepath.Join("uploads", "streams", fileName)
				// Create the folder if it doesn't exist
				utils.CreateFolderIfNotExists(filepath.Join("uploads", "streams"))
				logger.Log.WithField("stream", stream.Name).Info("Saving stream image")
				utils.GrabWebcamImage(stream.URL, savePath)
				logger.Log.WithField("stream", stream.Name).Info("Stream image saved")
				//Copy the image to stream_<id>_latest.jpg
				latestFileName := fmt.Sprintf("stream_%d_latest%s", stream.ID, filepath.Ext(".jpg"))
				latestSavePath := filepath.Join("uploads", "streams", latestFileName)
				utils.CopyFile(savePath, latestSavePath)
			}
		}
		logger.Log.Info("Stream grab complete")
		if config.StreamGrabInterval > 0 {
			interval = config.StreamGrabInterval
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}
