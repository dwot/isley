package utils

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"image/color"
	"io"
	"io/fs"
	"isley/logger"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Embed the fonts directory
//
//go:embed fonts/*
var embeddedFonts embed.FS

const (
	// maxImageDimension is the largest width or height (in pixels) we allow
	// before refusing to decode an image.  16384 px covers virtually all
	// consumer cameras while keeping the decoded bitmap under ~1 GB.
	maxImageDimension = 16384

	// maxImageFileSize is the largest file size (in bytes) we'll attempt to
	// process.  50 MB is generous for any reasonable photograph.
	maxImageFileSize = 50 * 1024 * 1024 // 50 MB
)

// validateImageFile checks that the file at path is within acceptable size and
// dimension limits before the caller loads it into memory.  It uses
// image.DecodeConfig which only reads the header, not the full pixel data.
func validateImageFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat image: %w", err)
	}
	if fi.Size() > maxImageFileSize {
		return fmt.Errorf("image file too large: %d bytes (max %d)", fi.Size(), maxImageFileSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open image for validation: %w", err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("cannot decode image header: %w", err)
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return fmt.Errorf("image dimensions %dx%d exceed maximum %d", cfg.Width, cfg.Height, maxImageDimension)
	}
	return nil
}

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

	// Validate image dimensions and file size before loading into memory
	if err := validateImageFile(req.ImagePath); err != nil {
		fieldLogger.WithError(err).Error("Image validation failed")
		return fmt.Errorf("image validation failed: %w", err)
	}

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
		if err := validateImageFile(imgObj.ImagePath); err != nil {
			fieldLogger.WithError(err).Error("Overlay image validation failed")
			return fmt.Errorf("overlay image validation failed: %w", err)
		}
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

// isPathWithinDir checks that resolved falls inside the allowed directory.
// Both paths are cleaned and resolved to absolute form before comparison.
func isPathWithinDir(path, allowedDir string) bool {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(filepath.Clean(allowedDir))
	if err != nil {
		return false
	}
	// Ensure the path starts with the allowed directory (with trailing separator)
	return strings.HasPrefix(absPath, absDir+string(filepath.Separator)) || absPath == absDir
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

	// SECURITY: Validate that ImagePath resolves within the uploads directory
	if !isPathWithinDir(req.ImagePath, "./uploads") {
		logger.Log.WithField("imagePath", req.ImagePath).Warn("Path traversal attempt blocked on ImagePath")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid image path"})
		return
	}

	// SECURITY: Validate that Logo resolves within the uploads/logos directory
	if req.Logo != "" && !isPathWithinDir(req.Logo, "./uploads/logos") {
		logger.Log.WithField("logo", req.Logo).Warn("Path traversal attempt blocked on Logo")
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid logo path"})
		return
	}

	// SECURITY: Validate that Font is a simple embedded path (no traversal)
	if req.Font != "" {
		cleanFont := filepath.Clean(req.Font)
		if strings.Contains(cleanFont, "..") || filepath.IsAbs(cleanFont) {
			logger.Log.WithField("font", req.Font).Warn("Path traversal attempt blocked on Font")
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid font path"})
			return
		}
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
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to process image"})
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
	var logos []string
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
	} else {
		c.JSON(http.StatusOK, gin.H{"success": true, "logos": logos})
	}
}

// maxFrameSize limits the amount of data read for a single image frame
// to prevent memory exhaustion from malicious or malformed responses.
const maxFrameSize = 50 * 1024 * 1024 // 50 MB

func GrabWebcamImage(rawURL string, outputPath string) error {
	logger.Log.WithFields(logrus.Fields{
		"url":        rawURL,
		"outputPath": outputPath,
	}).Info("Capturing image from webcam")

	// Validate URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("invalid webcam URL")
	}
	scheme := strings.ToLower(parsedURL.Scheme)

	switch scheme {
	case "http", "https":
		return grabHTTPFrame(rawURL, outputPath)
	case "rtsp", "rtmp":
		return fmt.Errorf("protocol %s requires ffmpeg which is not installed; use an HTTP/HTTPS snapshot URL instead", scheme)
	default:
		return fmt.Errorf("disallowed URL scheme: %s", scheme)
	}
}

// hlsThumbnailPaths lists well-known thumbnail endpoints served by common
// streaming software (Owncast, etc.).  When the configured stream URL points
// at an HLS playlist we try these paths on the same origin before giving up.
var hlsThumbnailPaths = []string{
	"/thumbnail.jpg",
	"/thumbnail.png",
	"/preview.jpg",
}

// grabHTTPFrame fetches a single image from an HTTP/HTTPS URL.
// It handles three common webcam/stream scenarios:
//   - Direct image URLs (snapshot endpoints that return JPEG/PNG)
//   - MJPEG streams (multipart/x-mixed-replace) where the first frame is extracted
//   - HLS streams (.m3u8) where we fall back to a server thumbnail endpoint
func grabHTTPFrame(rawURL string, outputPath string) error {
	// Detect HLS playlist URLs before making any request — these will never
	// return an image directly, so jump straight to thumbnail fallback.
	if isHLSURL(rawURL) {
		return grabHLSThumbnail(rawURL, outputPath)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("failed to fetch stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream returned HTTP %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")

	// MJPEG streams use multipart/x-mixed-replace with a boundary
	if strings.HasPrefix(contentType, "multipart/x-mixed-replace") {
		return grabMJPEGFrame(resp, outputPath)
	}

	// If the server returned an HLS playlist content type, try thumbnails
	if strings.Contains(contentType, "mpegurl") || strings.Contains(contentType, "apple.mpegurl") {
		resp.Body.Close()
		return grabHLSThumbnail(rawURL, outputPath)
	}

	// Treat everything else as a direct image (snapshot URL)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFrameSize))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Verify it's a valid image before writing to disk
	_, _, err = image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("response is not a valid image: %w", err)
	}

	if err := os.WriteFile(outputPath, body, 0644); err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	logger.Log.WithField("outputPath", outputPath).Info("Image captured successfully")
	return nil
}

// isHLSURL returns true if the URL path looks like an HLS playlist.
func isHLSURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	return strings.Contains(lower, ".m3u8") || strings.Contains(lower, ".m3u")
}

// grabHLSThumbnail attempts to fetch a thumbnail image from the same origin
// as an HLS stream URL. Streaming servers like Owncast serve a preview
// thumbnail at well-known paths alongside the HLS endpoint.
func grabHLSThumbnail(hlsURL string, outputPath string) error {
	parsed, err := url.Parse(hlsURL)
	if err != nil {
		return fmt.Errorf("invalid HLS URL: %w", err)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	baseURL := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	for _, thumbPath := range hlsThumbnailPaths {
		thumbURL := baseURL + thumbPath
		logger.Log.WithField("url", thumbURL).Debug("Trying HLS thumbnail fallback")

		resp, err := client.Get(thumbURL)
		if err != nil {
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxFrameSize))
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Verify it's actually an image
		_, _, err = image.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			continue
		}

		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			return fmt.Errorf("failed to write image: %w", err)
		}

		logger.Log.WithFields(logrus.Fields{
			"outputPath":   outputPath,
			"thumbnailURL": thumbURL,
		}).Info("Image captured from HLS server thumbnail")
		return nil
	}

	return fmt.Errorf("HLS stream detected but no thumbnail endpoint found at %s (tried %v); configure a direct image snapshot URL instead", baseURL, hlsThumbnailPaths)
}

// grabMJPEGFrame extracts the first JPEG frame from an MJPEG
// (multipart/x-mixed-replace) stream.
func grabMJPEGFrame(resp *http.Response, outputPath string) error {
	contentType := resp.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("failed to parse MJPEG content type: %w", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return fmt.Errorf("no boundary found in MJPEG content type")
	}

	reader := multipart.NewReader(resp.Body, boundary)

	// Read just the first part (first frame)
	part, err := reader.NextPart()
	if err != nil {
		return fmt.Errorf("failed to read MJPEG frame: %w", err)
	}
	defer part.Close()

	body, err := io.ReadAll(io.LimitReader(part, maxFrameSize))
	if err != nil {
		return fmt.Errorf("failed to read MJPEG frame data: %w", err)
	}

	if err := os.WriteFile(outputPath, body, 0644); err != nil {
		return fmt.Errorf("failed to write image: %w", err)
	}

	logger.Log.WithField("outputPath", outputPath).Info("Image captured successfully from MJPEG stream")
	return nil
}

func CreateFolderIfNotExists(join string) {
	//Create folder at path from input join argument if not exists
	if _, err := os.Stat(join); os.IsNotExist(err) {
		err := os.MkdirAll(join, os.ModePerm)
		if err != nil {
			logger.Log.WithError(err).Error("Failed to create folder")
		}
	}
}

