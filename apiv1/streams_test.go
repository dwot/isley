package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type streamBody struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Zone struct {
		ID int `json:"id"`
	} `json:"zone"`
	Visible  bool   `json:"visible"`
	FrameURL string `json:"frame_url"`
}

func TestStreamsAPI(t *testing.T) {
	t.Parallel()
	checkStreamsAPI(t, newFixture(t))
}

func checkStreamsAPI(t *testing.T, f fixture) {
	read, write, admin := f.key(t, "read"), f.key(t, "write"), f.key(t, "admin")
	tent := testutil.SeedZone(t, f.server.DB, "Tent")

	body := []byte(`{"name":"Tent cam","url":"http://cam.local/snap.jpg","zone_id":` + itoa(tent) + `}`)
	assert.Equal(t, http.StatusForbidden, f.cc.do(http.MethodPost, "/streams", bearer(write), body).StatusCode,
		"the server fetches stream URLs, so adding one needs admin")
	resp := f.cc.do(http.MethodPost, "/streams", bearer(admin), body)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	cam := decode[streamBody](t, resp.Body)
	assert.True(t, cam.Visible, "visible by default")
	assert.Contains(t, cam.FrameURL, "stream_"+itoa(cam.ID)+"_latest.jpg")
	assert.Equal(t, "/api/v1/streams/"+itoa(cam.ID), resp.Header.Get("Location"))

	resp = f.cc.do(http.MethodPost, "/streams", bearer(admin), []byte(`{"name":"x","url":"ftp://x","zone_id":9999}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	resp = f.cc.get("/streams?zone_id="+itoa(tent), read)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	list := decode[struct {
		Items []streamBody `json:"items"`
	}](t, resp.Body)
	require.Len(t, list.Items, 1)

	resp = f.cc.do(http.MethodPatch, "/streams/"+itoa(cam.ID), bearer(admin), []byte(`{"visible":false}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.False(t, decode[streamBody](t, resp.Body).Visible)

	assert.Equal(t, http.StatusNoContent, f.cc.do(http.MethodDelete, "/streams/"+itoa(cam.ID), bearer(admin), nil).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.get("/streams/"+itoa(cam.ID), read).StatusCode)
}
