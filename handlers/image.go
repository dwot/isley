package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"image/color"
	"isley/logger"
	"isley/utils"
	"net/http"
	"path/filepath"
)

func DecorateImageHandler(c *gin.Context) {

	var req struct {
		ImagePath  string `json:"imagePath"`
		StrainName string `json:"strainName"`
		PlantAge   string `json:"plantAge"`
	}

	if err := c.BindJSON(&req); err != nil {
		logger.Log.WithError(err).Error("Failed to bind JSON request")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid input"})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"imagePath":  req.ImagePath,
		"strainName": req.StrainName,
		"plantAge":   req.PlantAge,
	})

	// Prepare paths
	fileExtension := filepath.Ext(req.ImagePath)
	fileNameWithoutExt := req.ImagePath[:len(req.ImagePath)-len(fileExtension)]
	outputPath := fmt.Sprintf("%s.processed%s", fileNameWithoutExt, fileExtension)

	logoFile, _ := GetSetting("logo_image")
	logoPath := fmt.Sprintf("./uploads/logos/%s", logoFile)
	if logoPath == "" {
		logoPath = "web/static/img/placeholder.png"
	}

	logger.Log.WithFields(logrus.Fields{
		"outputPath": outputPath,
		"logoPath":   logoPath,
	})

	fontPath := "fonts/Anton-Regular.ttf" // Replace with your font path
	logger.Log.WithField("fontPath", fontPath)

	// Create overlay request
	overlayReq := utils.TextOverlayRequest{
		ImagePath:  req.ImagePath,
		OutputPath: outputPath,
		TextObjects: []utils.TextObject{
			{
				Text:        req.StrainName,
				Corner:      "top-left",
				FontPath:    fontPath,
				FontColor:   color.White,
				ShadowColor: color.Black,
				FontScale:   2.2,
			},
			{
				Text:        fmt.Sprintf("Day %s", req.PlantAge),
				Corner:      "bottom-right",
				FontPath:    fontPath,
				FontColor:   color.White,
				ShadowColor: color.Black,
				FontScale:   2.2,
			},
		},
		ImageObjects: []utils.ImageObject{
			{
				ImagePath: logoPath,
				Corner:    "bottom-left",
				Opacity:   0.8,
			},
		},
	}

	logger.Log.Info("Starting image processing")

	// Process the image
	if err := utils.ProcessImageWithTextOverlay(overlayReq); err != nil {
		logger.Log.WithError(err).Error("Failed to process image with text overlay")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logger.Log.Info("Image processed successfully")

	// Respond with the path to the new image
	c.JSON(http.StatusOK, gin.H{"success": true, "outputPath": outputPath})
}
