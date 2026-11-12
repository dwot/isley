package handlers

// Viewing and downloading the application log (Settings › Logs).

import (
	"isley/logger"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetLogs(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetLogs")

	linesParam := c.DefaultQuery("lines", strconv.Itoa(DefaultLogLines))
	n, err := strconv.Atoi(linesParam)
	if err != nil || n < MinLogLines {
		n = DefaultLogLines
	}
	if n > MaxLogLines {
		n = MaxLogLines
	}

	fileParam := c.DefaultQuery("file", "app")
	logsDir := LogsDirFromContext(c)
	var logPath string
	switch fileParam {
	case "access":
		logPath = filepath.Join(logsDir, "access.log")
	default:
		logPath = filepath.Join(logsDir, "app.log")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read log file")
		apiInternalError(c, "api_failed_to_read_log_file")
		return
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	c.JSON(http.StatusOK, gin.H{
		"lines": strings.Join(lines, "\n"),
		"total": len(lines),
	})
}

func DownloadLogs(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DownloadLogs")

	fileParam := c.DefaultQuery("file", "app")
	logsDir := LogsDirFromContext(c)
	var filePath, fileName string
	switch fileParam {
	case "access":
		filePath = filepath.Join(logsDir, "access.log")
		fileName = "access.log"
	default:
		filePath = filepath.Join(logsDir, "app.log")
		fileName = "app.log"
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fieldLogger.WithError(err).Error("Log file not found")
		apiNotFound(c, "api_log_file_not_found")
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+fileName)
	c.Header("Content-Type", "text/plain")
	c.File(filePath)
}
