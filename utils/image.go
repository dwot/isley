package utils

import (
	"embed"
	"fmt"
	"github.com/fogleman/gg"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"image/color"
	"isley/logger"
	"math"
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
