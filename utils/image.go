package utils

import (
	"embed"
	"fmt"
	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image/color"
	"io/fs"
	"isley/logger"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Embed the fonts directory
//
//go:embed fonts/*
var embeddedFonts embed.FS

type TextObject struct {
	Text        string
	Corner      string // "top-left", "top-right", "bottom-left", "bottom-right"
	FontPath    string
	FontColor   color.Color
	ShadowColor color.Color
	FontScale   float64 // Scaling factor for font size, baseline is 1.0
}

type ImageObject struct {
	ImagePath string
	Corner    string  // "top-left", "top-right", "bottom-left", "bottom-right"
	Opacity   float64 // 0.0 (fully transparent) to 1.0 (fully opaque)
}

type TextOverlayRequest struct {
	ImagePath    string
	OutputPath   string
	TextObjects  []TextObject
	ImageObjects []ImageObject
}

func ProcessImageWithTextOverlay(req TextOverlayRequest) error {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"imagePath":  req.ImagePath,
		"outputPath": req.OutputPath,
	})
	fieldLogger.Info("Starting image processing")

	// Load the base image
	img, err := gg.LoadImage(req.ImagePath)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load base image")
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Get image dimensions
	imgWidth := float64(img.Bounds().Dx())
	imgHeight := float64(img.Bounds().Dy())

	// Create a new drawing context
	dc := gg.NewContext(int(imgWidth), int(imgHeight))
	dc.DrawImage(img, 0, 0)

	padding := imgHeight / 100 // Reduce padding for tighter placement
	shadowOffset := imgHeight / 200

	// Helper to calculate scaled dimensions for overlays
	scaleDimension := func(targetWidth, targetHeight, maxWidth, maxHeight float64) (float64, float64) {
		scale := math.Min(maxWidth/targetWidth, maxHeight/targetHeight)
		return targetWidth * scale, targetHeight * scale
	}

	// Process Image Objects
	for _, imgObj := range req.ImageObjects {
		overlayImg, err := gg.LoadImage(imgObj.ImagePath)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to load overlay image")
			return fmt.Errorf("failed to load overlay image: %w", err)
		}

		// Scale the overlay image
		overlayWidth := float64(overlayImg.Bounds().Dx())
		overlayHeight := float64(overlayImg.Bounds().Dy())
		scaledWidth, scaledHeight := scaleDimension(overlayWidth, overlayHeight, imgWidth/6, imgHeight/6) // Increased scale factor for logo

		// Calculate overlay position
		var x, y float64
		switch imgObj.Corner {
		case "top-left":
			x, y = padding, padding
		case "top-right":
			x, y = imgWidth-scaledWidth-padding, padding
		case "bottom-left":
			x, y = padding, imgHeight-scaledHeight-padding
		case "bottom-right":
			x, y = imgWidth-scaledWidth-padding, imgHeight-scaledHeight-padding
		default:
			fieldLogger.WithField("corner", imgObj.Corner).Error("Invalid corner specified")
			return fmt.Errorf("invalid corner specified: %s", imgObj.Corner)
		}

		// Draw the scaled overlay image
		dc.Push()
		dc.ScaleAbout(scaledWidth/overlayWidth, scaledHeight/overlayHeight, x, y)
		dc.DrawImageAnchored(overlayImg, int(x), int(y), 0, 0)
		dc.Pop()
	}

	// Process Text Objects
	for _, textObj := range req.TextObjects {
		// Adjust font size dynamically based on aspect ratio
		aspectRatio := imgWidth / imgHeight
		adjustmentFactor := 1.0
		if aspectRatio < 1.0 { // Portrait
			adjustmentFactor = 0.75
		}
		fontSize := (imgHeight / 20 * textObj.FontScale) * adjustmentFactor

		// Load font from embedded FS
		fontData, err := embeddedFonts.ReadFile(textObj.FontPath)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read embedded font")
			return fmt.Errorf("failed to read embedded font: %w", err)
		}

		parsedFont, err := opentype.Parse(fontData)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to parse font data")
			return fmt.Errorf("failed to parse font data: %w", err)
		}

		fontFace, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
			Size:    fontSize,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create font face")
			return fmt.Errorf("failed to create font face: %w", err)
		}

		dc.SetFontFace(fontFace)

		_, textHeight := dc.MeasureString(textObj.Text)

		// Calculate text position
		var x, y float64
		switch textObj.Corner {
		case "top-left":
			x, y = padding, padding+textHeight/1.6 // Closer to the top
		case "top-right":
			x, y = imgWidth-padding, padding+textHeight/1.6
		case "bottom-left":
			x, y = padding, imgHeight-padding-textHeight*1.1
		case "bottom-right":
			x, y = imgWidth-padding, imgHeight-padding-textHeight*1.1
		default:
			fieldLogger.WithField("corner", textObj.Corner).Error("Invalid corner specified")
			return fmt.Errorf("invalid corner specified: %s", textObj.Corner)
		}

		ax, ay := 0.0, 0.0
		if textObj.Corner == "top-right" || textObj.Corner == "bottom-right" {
			ax = 1.0
		}
		if textObj.Corner == "bottom-left" || textObj.Corner == "bottom-right" {
			ay = 1.0
		}

		// Draw shadow
		dc.SetColor(textObj.ShadowColor)
		dc.DrawStringAnchored(textObj.Text, x+shadowOffset, y+shadowOffset, ax, ay)

		// Draw text
		dc.SetColor(textObj.FontColor)
		dc.DrawStringAnchored(textObj.Text, x, y, ax, ay)
	}

	// Save the output image
	if err := dc.SavePNG(req.OutputPath); err != nil {
		fieldLogger.WithError(err).Error("Failed to save output image")
		return fmt.Errorf("failed to save output image: %w", err)
	}

	fieldLogger.Info("Image processing completed successfully")
	return nil
}

func DecorateImageHandler(c *gin.Context) {
	var req struct {
		ImagePath   string `json:"imagePath"`
		Text1       string `json:"text1"`
		Text2       string `json:"text2"`
		Text1Corner string `json:"text1Corner"`
		Text2Corner string `json:"text2Corner"`
		Logo        string `json:"logo"`
		Font        string `json:"font"`
		TextColor   string `json:"textColor"`
	}

	if err := c.BindJSON(&req); err != nil {
		logger.Log.WithError(err).Error("Failed to bind JSON request")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid input"})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"imagePath":   req.ImagePath,
		"text1":       req.Text1,
		"text2":       req.Text2,
		"text1Corner": req.Text1Corner,
		"text2Corner": req.Text2Corner,
		"logo":        req.Logo,
		"font":        req.Font,
		"textColor":   req.TextColor,
	})

	// Prepare paths
	if req.ImagePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Image path is required"})
		return
	}

	fileExtension := filepath.Ext(req.ImagePath)
	fileNameWithoutExt := req.ImagePath[:len(req.ImagePath)-len(fileExtension)]
	outputPath := fmt.Sprintf("%s.processed%s", fileNameWithoutExt, fileExtension)

	logger.Log.WithFields(logrus.Fields{
		"outputPath": outputPath,
	})

	// Parse text color
	parsedTextColor, err := parseHexColor(req.TextColor)
	if err != nil {
		logger.Log.WithError(err).Error("Invalid text color")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid text color"})
		return
	}

	// Create overlay request
	textObjects := []TextObject{
		{
			Text:        req.Text1,
			Corner:      req.Text1Corner,
			FontPath:    req.Font,
			FontColor:   parsedTextColor,
			ShadowColor: color.Black,
			FontScale:   2.2,
		},
		{
			Text:        req.Text2,
			Corner:      req.Text2Corner,
			FontPath:    req.Font,
			FontColor:   parsedTextColor,
			ShadowColor: color.Black,
			FontScale:   2.2,
		},
	}

	imageObjects := []ImageObject{}
	if req.Logo != "" {
		imageObjects = append(imageObjects, ImageObject{
			ImagePath: req.Logo,
			Corner:    "bottom-left",
			Opacity:   0.8,
		})
	}

	overlayReq := TextOverlayRequest{
		ImagePath:    req.ImagePath,
		OutputPath:   outputPath,
		TextObjects:  textObjects,
		ImageObjects: imageObjects,
	}

	logger.Log.Info("Starting image processing")

	// Process the image
	if err := ProcessImageWithTextOverlay(overlayReq); err != nil {
		logger.Log.WithError(err).Error("Failed to process image with text overlay")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	logger.Log.Info("Image processed successfully")

	// Respond with the path to the new image
	c.JSON(http.StatusOK, gin.H{"success": true, "outputPath": outputPath})
}

func parseHexColor(s string) (color.Color, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return nil, fmt.Errorf("invalid color format")
	}

	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return nil, err
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return nil, err
	}
	b, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return nil, err
	}

	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}

func ListFontsHandler(c *gin.Context) {
	fonts := []string{}
	err := fs.WalkDir(embeddedFonts, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".ttf" {
			fonts = append(fonts, path)
		}
		return nil
	})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to list fonts")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Unable to list fonts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "fonts": fonts})
}

func ListLogosHandler(c *gin.Context) {
	logos := []string{}
	//Load all file names in the local filesystem on path ./uploads/logos/ to the slice  NOT EMBEDDED
	err := filepath.Walk("./uploads/logos/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			logos = append(logos, path)
		}
		return nil
	})

	if err != nil {
		logger.Log.WithError(err).Error("Failed to list logos")
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Unable to list logos"})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "logos": logos})
}
