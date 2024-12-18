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

	padding := imgHeight / 50
	shadowOffset := imgHeight / 200

	// Process Image Objects
	for _, imgObj := range req.ImageObjects {
		fieldLogger.WithFields(logrus.Fields{
			"imagePath": imgObj.ImagePath,
			"corner":    imgObj.Corner,
			"opacity":   imgObj.Opacity,
		}).Info("Processing image overlay")

		overlayImg, err := gg.LoadImage(imgObj.ImagePath)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to load overlay image")
			return fmt.Errorf("failed to load overlay image: %w", err)
		}

		overlayWidth := float64(overlayImg.Bounds().Dx())
		overlayHeight := float64(overlayImg.Bounds().Dy())

		// Calculate overlay position
		var x, y float64
		switch imgObj.Corner {
		case "top-left":
			x, y = padding, padding
		case "top-right":
			x, y = imgWidth-overlayWidth-padding, padding
		case "bottom-left":
			x, y = padding, imgHeight-overlayHeight-padding
		case "bottom-right":
			x, y = imgWidth-overlayWidth-padding, imgHeight-overlayHeight-padding
		default:
			fieldLogger.WithField("corner", imgObj.Corner).Error("Invalid corner specified")
			return fmt.Errorf("invalid corner specified: %s", imgObj.Corner)
		}

		// Apply transparency and draw the image
		dc.Push()
		dc.DrawImageAnchored(overlayImg, int(x), int(y), 0, 0)
		dc.Pop()
		dc.SetRGBA(1, 1, 1, imgObj.Opacity)
	}

	// Process Text Objects
	for _, textObj := range req.TextObjects {
		textLogger := logger.Log.WithFields(logrus.Fields{
			"text":     textObj.Text,
			"corner":   textObj.Corner,
			"fontPath": textObj.FontPath,
		})
		textLogger.Info("Processing text overlay")

		// Calculate scaled font size
		fontSize := (imgHeight / 20) * textObj.FontScale

		// Load font from the embedded FS
		fontData, err := embeddedFonts.ReadFile(textObj.FontPath)
		if err != nil {
			textLogger.WithError(err).Error("Failed to read embedded font")
			return fmt.Errorf("failed to read embedded font: %w", err)
		}

		// Parse the font using opentype
		parsedFont, err := opentype.Parse(fontData)
		if err != nil {
			textLogger.WithError(err).Error("Failed to parse font data")
			return fmt.Errorf("failed to parse font data: %w", err)
		}

		// Create a font face with the desired size
		fontFace, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
			Size:    fontSize,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			textLogger.WithError(err).Error("Failed to create font face")
			return fmt.Errorf("failed to create font face: %w", err)
		}

		dc.SetFontFace(fontFace)

		// Measure text dimensions
		_, textHeight := dc.MeasureString(textObj.Text)

		// Calculate text position
		var x, y float64
		paddingAdjustment := textHeight / 2

		switch textObj.Corner {
		case "top-left":
			x, y = padding, padding+textHeight
		case "top-right":
			x, y = imgWidth-padding, padding+textHeight
		case "bottom-left":
			x, y = padding, imgHeight-padding
		case "bottom-right":
			x, y = imgWidth-padding, imgHeight-padding
		default:
			logger.Log.WithField("corner", textObj.Corner).Error("Invalid corner specified")
			return fmt.Errorf("invalid corner specified: %s", textObj.Corner)
		}

		if textObj.Corner == "bottom-left" || textObj.Corner == "bottom-right" {
			y -= textHeight + paddingAdjustment
		} else if textObj.Corner == "top-left" || textObj.Corner == "top-right" {
			y = padding + textHeight
		}

		// Adjust text alignment for corners
		var ax, ay float64
		switch textObj.Corner {
		case "top-right":
			ax, ay = 1.0, 0.0
		case "top-left":
			ax, ay = 0.0, 0.0
		case "bottom-right":
			ax, ay = 1.0, 1.0
		case "bottom-left":
			ax, ay = 0.0, 1.0
		}

		// Draw drop shadow
		dc.SetColor(textObj.ShadowColor)
		dc.DrawStringAnchored(textObj.Text, x+shadowOffset, y+shadowOffset, ax, ay)

		// Draw main text
		dc.SetColor(textObj.FontColor)
		dc.DrawStringAnchored(textObj.Text, x, y, ax, ay)
	}

	// Save the output image
	if err := dc.SavePNG(req.OutputPath); err != nil {
		fieldLogger.WithError(err).Error("Failed to save output image")
		return fmt.Errorf("failed to save output image: %w", err)
	}

	fieldLogger.WithFields(logrus.Fields{
		"outputPath": req.OutputPath,
	}).Info("Image processing completed successfully")

	return nil
}
