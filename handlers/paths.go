package handlers

import (
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// Context keys for the per-engine on-disk path roots that handlers
// touch at request time. The engine middleware sets these once per
// request; handlers reach for them via the *FromContext accessors so
// production paths and per-test tempdirs flow through a single
// indirection. Mirrors the shape of contextKeyConfigStore /
// contextKeyBackupService — same dependency-injection pattern.
const (
	contextKeyUploadDir = "uploadDir"
	contextKeyStreamDir = "streamDir"
	contextKeyLogsDir   = "logsDir"
)

// Default path roots used when the engine constructs middleware without
// explicit overrides. Exported so production main and tests can compose
// derived paths (e.g. <UploadDir>/plants) the same way without
// duplicating the literals.
const (
	DefaultUploadDir = "uploads"
	DefaultLogsDir   = "logs"
)

// DefaultStreamDir returns the conventional stream-snapshot path
// derived from an upload root. Used by both the engine middleware and
// the watcher's grabber so a single UploadDir flip cascades.
func DefaultStreamDir(uploadDir string) string {
	if uploadDir == "" {
		uploadDir = DefaultUploadDir
	}
	return filepath.Join(uploadDir, "streams")
}

// UploadDirFromContext returns the per-engine upload root the
// middleware injected. Handlers compose subdirectories
// (<UploadDir>/plants, <UploadDir>/logos) off the returned path.
func UploadDirFromContext(c *gin.Context) string {
	v, _ := c.Get(contextKeyUploadDir)
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return DefaultUploadDir
}

// StreamDirFromContext returns the per-engine stream-snapshot root
// AddStreamHandler writes its initial GrabWebcamImage output into.
func StreamDirFromContext(c *gin.Context) string {
	v, _ := c.Get(contextKeyStreamDir)
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return DefaultStreamDir(UploadDirFromContext(c))
}

// LogsDirFromContext returns the per-engine logs root GetLogs and
// DownloadLogs read app.log / access.log from.
func LogsDirFromContext(c *gin.Context) string {
	v, _ := c.Get(contextKeyLogsDir)
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return DefaultLogsDir
}

// SetUploadDirOnContext, SetStreamDirOnContext, SetLogsDirOnContext are
// the small helpers the engine middleware uses to bind the configured
// directories to a request context.
func SetUploadDirOnContext(c *gin.Context, dir string) { c.Set(contextKeyUploadDir, dir) }
func SetStreamDirOnContext(c *gin.Context, dir string) { c.Set(contextKeyStreamDir, dir) }
func SetLogsDirOnContext(c *gin.Context, dir string)   { c.Set(contextKeyLogsDir, dir) }
