package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"io"
	"isley/logger"
	"isley/model"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plant ID"})
		return
	}
	fileLogger = logger.Log.WithField("plantID", plantID)

	// Parse the multipart form data
	err = c.Request.ParseMultipartForm(10 << 20) // Limit to 10 MB
	if err != nil {
		fileLogger.WithError(err).Error("Failed to parse multipart form data")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Get the uploaded files
	form := c.Request.MultipartForm
	files := form.File["images[]"]
	descriptions := form.Value["descriptions[]"]
	dates := form.Value["dates[]"]

	db, err := model.GetDB()
	if err != nil {
		fileLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

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

		// Generate a unique file path
		timestamp := time.Now().In(time.Local).UnixNano()
		fileName := fmt.Sprintf("plant_%d_image_%d_%d%s", plantID, index, timestamp, filepath.Ext(fileHeader.Filename))
		savePath := filepath.Join("uploads", "plants", fileName)
		fileLogger = fileLogger.WithField("savePath", savePath)

		// Create the uploads directory if it doesn't exist
		err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to create directory")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
			return
		}

		// Save the file to the filesystem
		out, err := os.Create(savePath)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to create file on filesystem")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}
		defer out.Close()
		_, err = io.Copy(out, file)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to save file to disk")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		// Parse description and date
		description := ""
		if index < len(descriptions) {
			description = descriptions[index]
		}
		imageDate := time.Now().In(time.Local)
		if index < len(dates) {
			parsedDate, err := time.Parse("2006-01-02", dates[index])
			if err == nil {
				imageDate = parsedDate
			} else {
				fileLogger.WithError(err).Warn("Failed to parse image date, using current time as fallback")
			}
		}

		// Save image metadata to the database
		_, err = db.Exec(`
            INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
            VALUES (?, ?, ?, 100, ?)`,
			plantID, savePath, description, imageDate)
		if err != nil {
			fileLogger.WithError(err).Error("Failed to save image metadata to database")
			continue
		}

		fileLogger.Info("Successfully processed and saved image")
	}

	c.JSON(http.StatusOK, gin.H{"message": "Images uploaded successfully"})
}

func DeletePlantImage(c *gin.Context) {
	fileLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeletePlantImage",
	})

	imageID, err := strconv.Atoi(c.Param("imageID"))
	if err != nil {
		fileLogger.WithError(err).Error("Invalid image ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}
	fileLogger = logger.Log.WithField("imageID", imageID)

	db, err := model.GetDB()
	if err != nil {
		fileLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Retrieve the image path
	var imagePath string
	err = db.QueryRow("SELECT image_path FROM plant_images WHERE id = ?", imageID).Scan(&imagePath)
	if err != nil {
		if err == sql.ErrNoRows {
			fileLogger.WithError(err).Error("Image not found in database")
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		} else {
			fileLogger.WithError(err).Error("Failed to query database for image")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query error"})
		}
		return
	}
	fileLogger = logger.Log.WithField("imagePath", imagePath)

	// Delete the image from the filesystem
	err = os.Remove(imagePath)
	if err != nil && !os.IsNotExist(err) {
		fileLogger.WithError(err).Error("Failed to delete file from filesystem")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}

	// Delete the image record from the database
	_, err = db.Exec("DELETE FROM plant_images WHERE id = ?", imageID)
	if err != nil {
		fileLogger.WithError(err).Error("Failed to delete image record from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete image record"})
		return
	}

	fileLogger.Info("Image deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}
