package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/logger"
	"isley/service"
)

// UploadPlantImages adds the images[] files of a multipart form to a plant,
// with descriptions[] and dates[] (YYYY-MM-DD) matched by position. All
// are stored or none are.
func UploadPlantImages(c *gin.Context) {
	plantID, err := strconv.Atoi(c.Param("plantID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}

	// Cap the request body before ParseMultipartForm — without this large
	// files spill to disk unbounded once they exceed the in-memory threshold.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxPlantImageRequestSize)
	if err := c.Request.ParseMultipartForm(MaxMultipartFormSize); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			apiError(c, http.StatusRequestEntityTooLarge, "api_image_too_large")
		} else {
			apiBadRequest(c, "api_failed_to_parse_form")
		}
		return
	}

	form := c.Request.MultipartForm
	files := form.File["images[]"]
	if len(files) == 0 {
		c.JSON(http.StatusOK, gin.H{"ids": []int{}, "message": T(c, "api_images_uploaded")})
		return
	}
	descriptions := form.Value["descriptions[]"]
	dates := form.Value["dates[]"]

	uploads := make([]service.ImageUpload, len(files))
	for i, fh := range files {
		f, err := fh.Open()
		if err != nil {
			logger.Log.WithError(err).WithField("plantID", plantID).Error("Failed to open uploaded file")
			apiInternalError(c, "api_failed_to_process_file")
			return
		}
		defer f.Close()
		u := service.ImageUpload{Content: f, Size: fh.Size}
		if i < len(descriptions) {
			u.Description = descriptions[i]
		}
		// An unreadable date falls back to today, as it always has.
		if i < len(dates) {
			if d, err := chrono.ParseDate(dates[i]); err == nil {
				u.TakenOn = d
			}
		}
		uploads[i] = u
	}

	images, err := ServicesFromContext(c).Images.Add(c.Request.Context(), plantID, uploads)
	if err != nil {
		imageError(c, err, "api_failed_to_save_file")
		return
	}
	ids := make([]int, len(images))
	for i, img := range images {
		ids[i] = img.ID
	}
	c.JSON(http.StatusOK, gin.H{"ids": ids, "message": T(c, "api_images_uploaded")})
}

// imageError keeps the upload form's two image errors: 413 for a file
// that is too large, 400 api_invalid_file_type for one that isn't a
// picture.
func imageError(c *gin.Context, err error, fallbackKey string) {
	if e, ok := service.AsError(err); ok && e.Kind == service.KindInvalid {
		for _, problem := range e.Fields {
			switch problem {
			case service.ProblemImageTooLarge:
				apiError(c, http.StatusRequestEntityTooLarge, "api_image_too_large")
				return
			case service.ProblemImageType:
				apiBadRequest(c, "api_invalid_file_type")
				return
			}
		}
	}
	apiServiceError(c, err, fallbackKey)
}

// DeletePlantImage deletes an image and its files.
func DeletePlantImage(c *gin.Context) {
	imageID, err := strconv.Atoi(c.Param("imageID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_image_id")
		return
	}
	if err := ServicesFromContext(c).Images.Delete(c.Request.Context(), imageID); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_image_record")
		return
	}
	apiOK(c, "api_image_deleted")
}
