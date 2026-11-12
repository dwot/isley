package service

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"isley/chrono"
	"isley/logger"
)

// CodeImageNotFound is the code for a missing image.
const CodeImageNotFound = "image_not_found"

const (
	// MaxImageBytes caps one uploaded image.
	MaxImageBytes = 50 << 20
	// MaxImageDescription caps an image description, in characters.
	MaxImageDescription = 5000

	// ProblemImageTooLarge and ProblemImageType are the field problems
	// for an image that is too big or not a supported picture.
	ProblemImageTooLarge = "must be at most 50 MB"
	ProblemImageType     = "must be a JPEG, PNG, GIF or WebP image"
)

// imageExt maps the sniffed content type to the stored file's extension.
// The extension never comes from the client's filename, so a file is
// always served as the picture it was checked to be.
var imageExt = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// ImageUpload is one image to add to a plant.
type ImageUpload struct {
	Content io.Reader
	// Size is the content length when known (it is checked before
	// reading), or -1.
	Size        int64
	Description string
	// TakenOn defaults to today.
	TakenOn chrono.Date
}

// ImageService stores plant photos: files under <UploadDir>/plants, a row
// each in plant_images.
type ImageService struct {
	deps Deps
}

func (s *ImageService) dir() string { return filepath.Join(s.deps.UploadDir, "plants") }

func errImageNotFound(id int) error {
	return NotFound(CodeImageNotFound, fmt.Sprintf("image %d does not exist", id))
}

// Get returns one image.
func (s *ImageService) Get(ctx context.Context, id int) (PlantImage, error) {
	imgs, err := queryImages(ctx, s.deps, "id = $1", id)
	if err != nil {
		return PlantImage{}, err
	}
	if len(imgs) == 0 {
		return PlantImage{}, errImageNotFound(id)
	}
	return imgs[0], nil
}

// ForPlant returns a plant's images, newest first.
func (s *ImageService) ForPlant(ctx context.Context, plantID int) ([]PlantImage, error) {
	if err := s.requirePlant(ctx, plantID); err != nil {
		return nil, err
	}
	return queryImages(ctx, s.deps, "plant_id = $1", plantID)
}

func (s *ImageService) requirePlant(ctx context.Context, plantID int) error {
	return requirePlant(ctx, s.deps.DB, plantID)
}

// requirePlant returns plant_not_found unless the plant exists.
func requirePlant(ctx context.Context, q querier, plantID int) error {
	found, err := existingIDs(ctx, q, "plant", []int{plantID})
	if err != nil {
		return err
	}
	if !found[plantID] {
		return NotFound(CodePlantNotFound, fmt.Sprintf("plant %d does not exist", plantID))
	}
	return nil
}

// Add stores images for a plant, all or none: a bad image means nothing is
// kept. Field names in a validation error are images[i].
func (s *ImageService) Add(ctx context.Context, plantID int, uploads []ImageUpload) ([]PlantImage, error) {
	var v Validation
	if len(uploads) == 0 {
		v.Add("images", "at least one image is required")
	}
	for i, u := range uploads {
		if utf8.RuneCountInString(u.Description) > MaxImageDescription {
			v.Add(fmt.Sprintf("images[%d].description", i), fmt.Sprintf("must be at most %d characters", MaxImageDescription))
		}
		if u.Size > MaxImageBytes {
			v.Add(fmt.Sprintf("images[%d]", i), ProblemImageTooLarge)
		}
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	if err := s.requirePlant(ctx, plantID); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return nil, fmt.Errorf("create image directory: %w", err)
	}
	// Each file lands under a temporary name first, so nothing half
	// written is ever visible at a real image path.
	var written []string
	cleanup := func() {
		for _, p := range written {
			_ = os.Remove(p)
		}
	}
	// The random part keeps names unique even when the clock repeats.
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("image name: %w", err)
	}
	stamp := fmt.Sprintf("%d_%x", s.deps.Clock.Now().UnixNano(), nonce)
	paths := make([]string, len(uploads))
	for i, u := range uploads {
		tmp, ext, err := s.writeTemp(u.Content)
		if tmp != "" {
			written = append(written, tmp)
		}
		var problem *imageProblem
		if errors.As(err, &problem) {
			v.Add(fmt.Sprintf("images[%d]", i), problem.text)
			continue
		}
		if err != nil {
			cleanup()
			return nil, err
		}
		paths[i] = filepath.ToSlash(filepath.Join(s.dir(), fmt.Sprintf("plant_%d_image_%d_%s%s", plantID, i, stamp, ext)))
	}
	if err := v.Err(); err != nil {
		cleanup()
		return nil, err
	}
	for i := range paths {
		if err := os.Rename(written[i], paths[i]); err != nil {
			cleanup()
			return nil, fmt.Errorf("store image: %w", err)
		}
		written[i] = paths[i]
	}

	today := s.deps.Zones.Today(s.deps.Clock)
	ids := make([]int, len(uploads))
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for i, u := range uploads {
			day := u.TakenOn
			if day.IsZero() {
				day = today
			}
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
				VALUES ($1, $2, $3, $4, $5) RETURNING id`,
				plantID, paths[i], u.Description, DefaultImageOrder, day).Scan(&ids[i]); err != nil {
				return fmt.Errorf("save image: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		cleanup()
		return nil, err
	}

	out := make([]PlantImage, len(ids))
	for i, id := range ids {
		if out[i], err = s.Get(ctx, id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DefaultImageOrder is the image_order new images get.
const DefaultImageOrder = 100

type imageProblem struct{ text string }

func (p *imageProblem) Error() string { return p.text }

// writeTemp copies one image into a temporary file in the image directory
// and returns its path and the extension its content calls for. A file
// that is not a supported picture or is too large is an *imageProblem.
func (s *ImageService) writeTemp(content io.Reader) (string, string, error) {
	br := bufio.NewReaderSize(content, 512)
	head, err := br.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return "", "", fmt.Errorf("read image: %w", err)
	}
	ext, ok := imageExt[http.DetectContentType(head)]
	if !ok {
		return "", "", &imageProblem{ProblemImageType}
	}
	f, err := os.CreateTemp(s.dir(), ".upload-*")
	if err != nil {
		return "", "", fmt.Errorf("create image file: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(br, MaxImageBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return f.Name(), "", fmt.Errorf("write image file: %w", err)
	}
	if n > MaxImageBytes {
		return f.Name(), "", &imageProblem{ProblemImageTooLarge}
	}
	return f.Name(), ext, nil
}

// Delete deletes an image and its files.
func (s *ImageService) Delete(ctx context.Context, id int) error {
	img, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	res, err := s.deps.DB.ExecContext(ctx, "DELETE FROM plant_images WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete image %d: %w", id, err)
	}
	if err := requireRow(res, errImageNotFound(id)); err != nil {
		return err
	}
	s.removeFiles([]string{img.Path})
	return nil
}

// removeFiles deletes stored image files and their decorated copies
// (<name>.processed<ext>). The row is already gone, so a file that can't
// be removed is logged, not returned. Paths outside the image directory
// (say, from an edited backup) are never touched.
func (s *ImageService) removeFiles(paths []string) {
	root, err := filepath.Abs(s.dir())
	if err != nil {
		logger.Log.WithError(err).Warn("Cannot resolve the image directory; image files left in place")
		return
	}
	for _, p := range paths {
		abs, err := filepath.Abs(filepath.FromSlash(p))
		if err != nil || !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			logger.Log.WithField("path", p).Warn("Image path is outside the image directory; file left in place")
			continue
		}
		ext := filepath.Ext(abs)
		for _, f := range []string{abs, strings.TrimSuffix(abs, ext) + ".processed" + ext} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				logger.Log.WithError(err).WithField("path", f).Warn("Failed to delete image file")
			}
		}
	}
}

// queryImages returns the images matching where, newest first.
func queryImages(ctx context.Context, deps Deps, where string, args ...any) ([]PlantImage, error) {
	rows, err := deps.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, plant_id, image_path, COALESCE(image_description, ''), image_order, image_date
		FROM plant_images WHERE %s
		ORDER BY %s DESC, id DESC`, where, deps.Dialect.DayOf("image_date")), args...)
	if err != nil {
		return nil, fmt.Errorf("plant images: %w", err)
	}
	defer rows.Close()
	out := []PlantImage{}
	for rows.Next() {
		var img PlantImage
		if err := rows.Scan(&img.ID, &img.PlantID, &img.Path, &img.Description, &img.Order, &img.TakenOn); err != nil {
			return nil, fmt.Errorf("scan image: %w", err)
		}
		img.Path = strings.ReplaceAll(img.Path, "\\", "/")
		out = append(out, img)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plant images: %w", err)
	}
	return out, nil
}
