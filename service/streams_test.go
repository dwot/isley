package service_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/service"
	"isley/tests/testutil"
)

func TestStreams(t *testing.T) {
	t.Parallel()
	db := testutil.NewTestDB(t)
	checkStreams(t, db, testutil.NewServices(t, db))
}

func checkStreams(t *testing.T, db *sql.DB, svc *service.Services) {
	ctx := context.Background()
	tent := testutil.SeedZone(t, db, "Tent")
	closet := testutil.SeedZone(t, db, "Closet")

	cam, err := svc.Streams.Create(ctx, service.StreamInput{Name: " Tent cam ", URL: "http://cam.local/snap.jpg", ZoneID: tent, Visible: true})
	require.NoError(t, err)
	assert.Equal(t, "Tent cam", cam.Name)
	assert.Equal(t, service.Ref{ID: tent, Name: "Tent"}, cam.Zone)
	_, err = svc.Streams.Create(ctx, service.StreamInput{Name: "Closet cam", URL: "rtsp://cam.local/live", ZoneID: closet})
	require.NoError(t, err)

	_, err = svc.Streams.Create(ctx, service.StreamInput{Name: "", URL: "file:///etc/passwd", ZoneID: 9999})
	e, ok := service.AsError(err)
	require.True(t, ok, "%v", err)
	for _, f := range []string{"name", "url", "zone_id"} {
		assert.Contains(t, e.Fields, f)
	}

	all, err := svc.Streams.List(ctx, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "Closet cam", all[0].Name, "by zone name")
	inTent, err := svc.Streams.List(ctx, tent)
	require.NoError(t, err)
	require.Len(t, inTent, 1)

	hidden := false
	got, err := svc.Streams.Update(ctx, cam.ID, service.StreamUpdate{Visible: service.Put(&hidden), ZoneID: service.Put(&closet)})
	require.NoError(t, err)
	assert.False(t, got.Visible)
	assert.Equal(t, closet, got.Zone.ID)
	assert.Equal(t, "http://cam.local/snap.jpg", got.URL, "unset fields stay")
	_, err = svc.Streams.Update(ctx, 9999, service.StreamUpdate{})
	assert.True(t, service.IsKind(err, service.KindNotFound))

	// Delete takes the saved frame too.
	frame := svc.Streams.LatestFramePath(cam.ID)
	require.NoError(t, os.MkdirAll(svc.UploadDir()+"/streams", 0o755))
	require.NoError(t, os.WriteFile(frame, []byte("jpeg"), 0o600))
	require.NoError(t, svc.Streams.Delete(ctx, cam.ID))
	assert.NoFileExists(t, frame)
	assert.True(t, service.IsKind(svc.Streams.Delete(ctx, cam.ID), service.KindNotFound))
}
