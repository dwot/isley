package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"image/color"
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
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid input"})
		return
	}

	// Prepare paths
	// Split the input image path to get the filename and extension
	fileExtension := filepath.Ext(req.ImagePath)
	fileNameWithoutExt := req.ImagePath[:len(req.ImagePath)-len(fileExtension)]
	outputPath := fmt.Sprintf("%s.processed%s", fileNameWithoutExt, fileExtension)
	//get the logo path from GetSetting("logoPath")
	//append "/uploads/logops" to the logo path
	//if the logo path is empty, use the placeholder image
	logoFile, _ := GetSetting("logo_image")
	logoPath := fmt.Sprintf("./uploads/logos/%s", logoFile)
	if logoPath == "" {
		logoPath = "./web/static/img/placeholder.png"
	}
	fontPath := "./fonts/Anton-Regular.ttf" // Replace with your font path

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

	// Process the image
	if err := utils.ProcessImageWithTextOverlay(overlayReq); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Respond with the path to the new image
	c.JSON(http.StatusOK, gin.H{"success": true, "outputPath": outputPath})
}
