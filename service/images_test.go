package service_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/chrono"
	"isley/service"
	"isley/tests/testutil"
)

var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00}

func TestImages_AllOrNone(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	svc := testutil.NewServices(t, db)
	plant := testutil.SeedPlant(t, db, "A",
		testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))
	ctx := context.Background()

	_, err := svc.Images.Add(ctx, plant, []service.ImageUpload{
		{Content: bytes.NewReader(jpegBytes), Size: -1},
		{Content: bytes.NewReader([]byte("not a picture")), Size: -1},
	})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, map[string]string{"images[1]": service.ProblemImageType}, e.Fields)

	imgs, err := svc.Images.ForPlant(ctx, plant)
	require.NoError(t, err)
	assert.Empty(t, imgs)
	entries, _ := os.ReadDir(filepath.Join(svc.UploadDir(), "plants"))
	assert.Empty(t, entries, "nothing is left on disk")

	_, err = svc.Images.Add(ctx, plant, []service.ImageUpload{{Content: bytes.NewReader(jpegBytes), Size: service.MaxImageBytes + 1}})
	e, ok = service.AsError(err)
	require.True(t, ok)
	assert.Equal(t, service.ProblemImageTooLarge, e.Fields["images[0]"])
}

// Rows written by older versions hold timestamps, not days, and may point
// anywhere; deleting one never removes a file outside the image directory.
func TestImages_OldRows(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	svc := testutil.NewServices(t, db)
	plant := testutil.SeedPlant(t, db, "A",
		testutil.SeedStrain(t, db, testutil.SeedBreeder(t, db, "B"), "S"), testutil.SeedZone(t, db, "Z"))
	ctx := context.Background()

	outside := filepath.Join(t.TempDir(), "keep.jpg")
	require.NoError(t, os.WriteFile(outside, jpegBytes, 0o600))
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
		VALUES ($1, $2, 'old', 100, '2026-04-25 00:00:00+00:00')`, plant, outside)
	testutil.MustExec(t, db, `INSERT INTO plant_images (plant_id, image_path, image_description, image_order, image_date)
		VALUES ($1, 'uploads\plants\win.jpg', 'windows', 100, '2026-05-01 13:45:12.123-04:00')`, plant)

	imgs, err := svc.Images.ForPlant(ctx, plant)
	require.NoError(t, err)
	require.Len(t, imgs, 2)
	assert.Equal(t, "windows", imgs[0].Description)
	assert.Equal(t, chrono.Date{Year: 2026, Month: 5, Day: 1}, imgs[0].TakenOn)
	assert.Equal(t, "uploads/plants/win.jpg", imgs[0].Path)
	assert.Equal(t, chrono.Date{Year: 2026, Month: 4, Day: 25}, imgs[1].TakenOn)

	require.NoError(t, svc.Images.Delete(ctx, imgs[1].ID))
	assert.FileExists(t, outside)
	_, err = svc.Images.Get(ctx, imgs[1].ID)
	assert.True(t, service.IsKind(err, service.KindNotFound))
}
