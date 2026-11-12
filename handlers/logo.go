package handlers

// Custom logo upload (Settings › Customization).

import (
	"fmt"
	"io"
	"isley/logger"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
)

func UploadLogo(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UploadLogo")
	// Parse the multipart form data
	err := c.Request.ParseMultipartForm(MaxMultipartFormSize) // Limit to 10 MB
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to parse form data")
		apiBadRequest(c, "api_failed_to_parse_form")
		return
	}

	// Retrieve the file from the "logo" field
	fileHeader, err := c.FormFile("logo")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to retrieve file")
		apiBadRequest(c, "api_failed_to_retrieve_file")
		return
	}

	// Open the uploaded file
	file, err := fileHeader.Open()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open file")
		apiInternalError(c, "api_failed_to_open_file")
		return
	}
	defer file.Close()

	// Validate MIME type
	sniff := make([]byte, 512)
	n, _ := file.Read(sniff)
	mimeType := http.DetectContentType(sniff[:n])
	allowedMIME := map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}
	if !allowedMIME[mimeType] {
		fieldLogger.WithField("mimeType", mimeType).Warn("Rejected logo upload with disallowed MIME type")
		apiBadRequest(c, "api_invalid_file_type")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		fieldLogger.WithError(err).Error("Failed to seek logo file")
		apiInternalError(c, "api_failed_to_process_file")
		return
	}

	// Generate a unique file path under the per-engine upload root.
	timestamp := time.Now().UnixNano()
	fileName := fmt.Sprintf("logo_image_%d%s", timestamp, filepath.Ext(fileHeader.Filename))
	savePath := filepath.Join(UploadDirFromContext(c), "logos", fileName)

	// Create the uploads/logos directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create directory")
		apiInternalError(c, "api_failed_to_create_directory")
		return
	}

	// Save the file to the filesystem
	out, err := os.Create(savePath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		apiInternalError(c, "api_failed_to_save_file")
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save file")
		apiInternalError(c, "api_failed_to_save_file")
		return
	}

	// Update the database with the new logo path
	db := DBFromContext(c)
	err = UpdateSetting(db, ConfigStoreFromContext(c), "logo_image", fileName)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update logo setting")
		apiInternalError(c, "api_failed_to_update_logo")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_logo_uploaded"), "path": savePath})
}
