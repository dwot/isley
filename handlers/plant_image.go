package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"io"
	"isley/logger"
	"isley/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func UploadPlantImages(c *gin.Context) {
	fileLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "UploadPlantImages",
	})

	// Get the plant ID from the URL parameter
	plantID, err := strconv.Atoi(c.Param("plantID"))
	if err != nil {
		fileLogger.WithError(err).Error("Invalid plant ID")
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}
	fileLogger = logger.Log.WithField("plantID", plantID)

	// Parse the multipart form data
	err = c.Request.ParseMultipartForm(10 << 20) // Limit to 10 MB
	if err != nil {
		fileLogger.WithError(err).Error("Failed to parse multipart form data")
		apiBadRequest(c, "api_failed_to_parse_form")
		return
	}

	// Get the uploaded files
	form := c.Request.MultipartForm
	files := form.File["images[]"]
	descriptions := form.Value["descriptions[]"]
	dates := form.Value["dates[]"]

	db := DBFromContext(c)
	imageIDs := make([]int, 0)
	// Process each uploaded file
	for index, fileHeader := range files {
		fileLogger := logger.Log.WithField("fileIndex", index)

		// Open the uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			fileLogger.WithError(err).Error("Failed to open uploaded file")
			continue
		}
		defer file.Close()

		// Validate MIME type
		sniff := make([]byte, 512)
		n, _ := file.Read(sniff)
		mimeType := http.DetectContentType(sniff[:n])
		allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}
		if !allowed[mimeType] {
			fileLogger.WithField("mimeType", mimeType).Warn("Rejected upload with disallowed MIME type")
			apiBadRequest(c, "api_invalid_file_type")
			return
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			fileLogger.WithError(err).Error("Failed to seek uploaded file")
			apiInternalError(c, "api_failed_to_process_file")
			return
		}

		// Generate a unique file path
		timestamp := time.Now().UnixNano()
		fileName := fmt.Sprintf("plant_%d_image_%d_%d%s", plantID, index, timestamp, filepath.Ext(fileHeader.Filename))
		savePath := filepath.Join("uploads", "plants", fileName)
		fileLogger = fileLogger.WithField("savePath", savePath)

		// Create the uploads directory if it doesn't exist
		err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to create directory")
			apiInternalError(c, "api_failed_to_create_directory")
			return
		}

		// Save the file to the filesystem
		out, err := os.Create(savePath)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to create file on filesystem")
			apiInternalError(c, "api_failed_to_save_file")
			return
		}
		defer out.Close()
		_, err = io.Copy(out, file)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to save file to disk")
			apiInternalError(c, "api_failed_to_save_file")
			return
		}

		// Parse description and date
		description := ""
		if index < len(descriptions) {
			description = descriptions[index]
		}
		imageDate := time.Now()
		if index < len(dates) {
			parsedDate, err := time.Parse(utils.LayoutDate, dates[index])
			if err == nil {
				imageDate = parsedDate
			} else {
				fileLogger.WithError(err).Warn("Failed to parse image date, using current time as fallback")
			}
		}

		// Save image metadata to the database

		var imageID int
		err = db.QueryRow(`
            INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
            VALUES ($1, $2, $3, 100, $4) returning id`,
			plantID, savePath, description, imageDate).Scan(&imageID)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to save image metadata to database")
			continue
		}
		imageIDs = append(imageIDs, imageID)

		fileLogger.Info("Successfully processed and saved image")
	}

	c.JSON(http.StatusOK, gin.H{"ids": imageIDs, "message": T(c, "api_images_uploaded")})
}

func DeletePlantImage(c *gin.Context) {
	fileLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeletePlantImage",
	})

	imageID, err := strconv.Atoi(c.Param("imageID"))
	if err != nil {
		fileLogger.WithError(err).Error("Invalid image ID")
		apiBadRequest(c, "api_invalid_image_id")
		return
	}
	fileLogger = logger.Log.WithField("imageID", imageID)

	db := DBFromContext(c)

	// Retrieve the image path
	var imagePath string
	err = db.QueryRow("SELECT image_path FROM plant_images WHERE id = $1", imageID).Scan(&imagePath)
	if err != nil {
		if err == sql.ErrNoRows {
			fileLogger.WithError(err).Error("Image not found in database")
			apiNotFound(c, "api_image_not_found")
		} else {
			fileLogger.WithError(err).Error("Failed to query database for image")
			apiInternalError(c, "api_database_query_error")
		}
		return
	}
	fileLogger = logger.Log.WithField("imagePath", imagePath)

	// Delete the image from the filesystem
	err = os.Remove(imagePath)
	if err != nil && !os.IsNotExist(err) {
		fileLogger.WithError(err).Error("Failed to delete file from filesystem")
		apiInternalError(c, "api_failed_to_delete_file")
		return
	}

	// Delete the image record from the database
	_, err = db.Exec("DELETE FROM plant_images WHERE id = $1", imageID)
	if err != nil {
		fileLogger.WithError(err).Error("Failed to delete image record from database")
		apiInternalError(c, "api_failed_to_delete_image_record")
		return
	}

	fileLogger.Info("Image deleted successfully")
	apiOK(c, "api_image_deleted")
}
