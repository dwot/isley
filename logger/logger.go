package logger

import (
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *logrus.Logger

func InitLogger() {
	// Create logs directory if it doesn't exist
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		os.Mkdir("logs", 0755)
	}

	Log = logrus.New()

	// Configure lumberjack for log rotation
	logFile := &lumberjack.Logger{
		Filename:   "logs/app.log",
		MaxSize:    10, // Max size in MB
		MaxBackups: 5,  // Number of old log files to keep
		MaxAge:     30, // Max age in days
		Compress:   true,
	}

	// Multi-writer for logging to both file and console
	multiWriter := io.MultiWriter(logFile, os.Stdout)

	// Set log output to multi-writer
	Log.SetOutput(multiWriter)

	// Set log format
	Log.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339,
		FullTimestamp:   true,
	})

	// Set log level
	Log.SetLevel(logrus.InfoLevel)
}
