package utils

import (
	"fmt"
	"github.com/fogleman/gg"
	"image/color"
)

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
	// Load the base image
	img, err := gg.LoadImage(req.ImagePath)
	if err != nil {
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
		overlayImg, err := gg.LoadImage(imgObj.ImagePath)
		if err != nil {
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
		// Calculate scaled font size
		fontSize := (imgHeight / 20) * textObj.FontScale
		if err := dc.LoadFontFace(textObj.FontPath, fontSize); err != nil {
			return fmt.Errorf("failed to load font: %w", err)
		}

		// Measure text dimensions
		_, textHeight := dc.MeasureString(textObj.Text)

		// Calculate text position with adjustments to ensure visibility
		var x, y float64
		paddingAdjustment := textHeight / 2 // Dynamically adjust padding based on text height

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
			return fmt.Errorf("invalid corner specified: %s", textObj.Corner)
		}

		// Prevent text from bleeding off the edges
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
		return fmt.Errorf("failed to save output image: %w", err)
	}

	return nil
}
