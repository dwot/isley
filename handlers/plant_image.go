package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"isley/model"
	"log"
	_ "mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func UploadPlantImages(c *gin.Context) {
	// Get the plant ID from the URL parameter
	plantID, err := strconv.Atoi(c.Param("plantID"))
	if err != nil {
		log.Println("Error parsing plant ID:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plant ID"})
		return
	}

	// Parse the multipart form data
	err = c.Request.ParseMultipartForm(10 << 20) // Limit to 10 MB
	if err != nil {
		log.Println("Error parsing form data:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Get the uploaded files
	form := c.Request.MultipartForm
	files := form.File["images[]"]
	descriptions := form.Value["descriptions[]"]
	dates := form.Value["dates[]"]

	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	// Process each uploaded file
	for index, fileHeader := range files {
		// Open the uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			log.Println("Error opening file:", err)
			continue
		}
		defer file.Close()

		// Generate a unique file path
		timestamp := time.Now().UnixNano()
		fileName := fmt.Sprintf("plant_%d_image_%d_%d%s", plantID, index, timestamp, filepath.Ext(fileHeader.Filename))
		savePath := filepath.Join("uploads", "plants", fileName)

		// Create the uploads directory if it doesn't exist
		err = os.MkdirAll(filepath.Dir(savePath), os.ModePerm)
		if err != nil {
			log.Println("Error creating directory:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create directory"})
			return
		}

		// Save the file to the filesystem
		out, err := os.Create(savePath)
		if err != nil {
			log.Println("Error creating file:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}
		defer out.Close()
		_, err = io.Copy(out, file)
		if err != nil {
			log.Println("Error saving file:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		// Parse description and date
		description := ""
		if index < len(descriptions) {
			description = descriptions[index]
		}
		imageDate := time.Now() // Default to now if date is invalid
		if index < len(dates) {
			parsedDate, err := time.Parse("2006-01-02", dates[index])
			if err == nil {
				imageDate = parsedDate
			}
		}

		// Save image metadata to the database
		_, err = db.Exec(`
            INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
            VALUES (?, ?, ?, 100, ?)`,
			plantID, savePath, description, imageDate)
		if err != nil {
			log.Println("Error saving metadata:", err)
			continue
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Images uploaded successfully"})
}

func DeletePlantImage(c *gin.Context) {
	// Get the image ID from the URL
	imageID, err := strconv.Atoi(c.Param("imageID"))
	if err != nil {
		log.Println("Error parsing image ID:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image ID"})
		return
	}

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	// Retrieve the image path
	var imagePath string
	err = db.QueryRow("SELECT image_path FROM plant_images WHERE id = ?", imageID).Scan(&imagePath)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		} else {
			log.Println("Error querying image:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query error"})
		}
		return
	}

	// Delete the image from the filesystem
	err = os.Remove(imagePath)
	if err != nil && !os.IsNotExist(err) {
		log.Println("Error deleting file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}

	// Delete the image record from the database
	_, err = db.Exec("DELETE FROM plant_images WHERE id = ?", imageID)
	if err != nil {
		log.Println("Error deleting image record:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete image record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Image deleted successfully"})
}
