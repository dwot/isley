package apiv1

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
)

// maxUploadBytes caps an image upload request: one image plus its form
// fields.
const maxUploadBytes = service.MaxImageBytes + 1<<20

type imageJSON struct {
	ID      int    `json:"id"`
	PlantID int    `json:"plant_id"`
	URL     string `json:"url"`
	// Description is "" when there is none.
	Description string      `json:"description"`
	TakenOn     chrono.Date `json:"taken_on"`
}

func toImageJSON(img service.PlantImage) imageJSON {
	return imageJSON{
		ID: img.ID, PlantID: img.PlantID, URL: "/" + strings.TrimPrefix(img.Path, "/"),
		Description: img.Description, TakenOn: img.TakenOn,
	}
}

func (a *api) listPlantImages(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	imgs, err := a.svc.Images.ForPlant(c.Request.Context(), plantID)
	if err != nil {
		writeError(c, err)
		return
	}
	// A plant has a handful of images; paging the full list is simpler
	// than paging in SQL.
	end := min(offset+page.Limit, len(imgs))
	start := min(offset, end)
	out := make([]imageJSON, 0, end-start)
	for _, img := range imgs[start:end] {
		out = append(out, toImageJSON(img))
	}
	next, ok := nextOffset(c, offset, len(out), len(imgs))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) getImage(c *gin.Context) {
	id, ok := pathID(c, service.CodeImageNotFound, "image")
	if !ok {
		return
	}
	img, err := a.svc.Images.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toImageJSON(img))
}

// createImage takes a multipart form: one file in "image", an optional
// "description" and an optional "taken_on" day.
func (a *api) createImage(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	// Files over 1 MB spill to temporary files rather than memory.
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			abortWith(c, http.StatusRequestEntityTooLarge, CodePayloadTooLarge, "the request is larger than 51 MB", nil)
			return
		}
		invalid(c, map[string]string{"body": "must be multipart/form-data"})
		return
	}
	defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	form := c.Request.MultipartForm

	problems := map[string]string{}
	for name := range form.Value {
		if name != "description" && name != "taken_on" {
			problems[name] = "is not a field of this request"
		}
	}
	for name := range form.File {
		if name != "image" {
			problems[name] = "is not a field of this request"
		}
	}
	files := form.File["image"]
	switch {
	case len(files) == 0:
		problems["image"] = "is required"
	case len(files) > 1:
		problems["image"] = "must be a single file; upload images one per request"
	}
	upload := service.ImageUpload{Size: -1}
	if v, ok := single(form.Value, "description", problems); ok {
		upload.Description = v
	}
	if v, ok := single(form.Value, "taken_on", problems); ok {
		d, err := chrono.ParseDate(v)
		if err != nil {
			problems["taken_on"] = "must be a date (YYYY-MM-DD)"
		}
		upload.TakenOn = d
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}

	f, err := files[0].Open()
	if err != nil {
		writeError(c, err)
		return
	}
	defer f.Close()
	upload.Content, upload.Size = f, files[0].Size
	imgs, err := a.svc.Images.Add(c.Request.Context(), plantID, []service.ImageUpload{upload})
	if err != nil {
		writeError(c, renameImageFields(err))
		return
	}
	c.Header("Location", Prefix+"/images/"+itoa(imgs[0].ID))
	c.JSON(http.StatusCreated, toImageJSON(imgs[0]))
}

// single reads a form value that may appear at most once.
func single(values map[string][]string, name string, problems map[string]string) (string, bool) {
	vs := values[name]
	switch len(vs) {
	case 0:
		return "", false
	case 1:
		return vs[0], true
	default:
		problems[name] = "must be given once"
		return "", false
	}
}

// renameImageFields maps the service's images[0] field names onto this
// request's form fields.
func renameImageFields(err error) error {
	e, ok := service.AsError(err)
	if !ok || len(e.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(e.Fields))
	for name, problem := range e.Fields {
		switch name {
		case "images[0]":
			name = "image"
		case "images[0].description":
			name = "description"
		}
		fields[name] = problem
	}
	return service.Invalid(fields)
}

func (a *api) deleteImage(c *gin.Context) {
	id, ok := pathID(c, service.CodeImageNotFound, "image")
	if !ok {
		return
	}
	if err := a.svc.Images.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
