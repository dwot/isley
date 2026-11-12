package apiv1

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/service"
)

type streamJSON struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	URL     string  `json:"url"`
	Zone    refJSON `json:"zone"`
	Visible bool    `json:"visible"`
	// FrameURL is the latest saved frame on this instance.
	FrameURL string `json:"frame_url"`
}

func (a *api) toStreamJSON(st service.Stream) streamJSON {
	return streamJSON{
		ID: st.ID, Name: st.Name, URL: st.URL, Zone: refJSON{ID: st.Zone.ID, Name: st.Zone.Name},
		Visible:  st.Visible,
		FrameURL: "/" + strings.TrimPrefix(filepath.ToSlash(a.svc.Streams.LatestFramePath(st.ID)), "/"),
	}
}

func (a *api) listStreams(c *gin.Context) {
	problems := map[string]string{}
	zoneID := queryID(c, "zone_id", problems)
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	streams, err := a.svc.Streams.List(c.Request.Context(), zoneID)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]streamJSON, len(streams))
	for i, st := range streams {
		out[i] = a.toStreamJSON(st)
	}
	// A handful per instance: one page.
	c.JSON(http.StatusOK, NewList(out, nil))
}

func (a *api) getStream(c *gin.Context) {
	id, ok := pathID(c, service.CodeStreamNotFound, "stream")
	if !ok {
		return
	}
	st, err := a.svc.Streams.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, a.toStreamJSON(st))
}

func (a *api) createStream(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		ZoneID  int    `json:"zone_id"`
		Visible *bool  `json:"visible"`
	}
	if !bindJSON(c, &req) {
		return
	}
	in := service.StreamInput{Name: req.Name, URL: req.URL, ZoneID: req.ZoneID, Visible: true}
	if req.Visible != nil {
		in.Visible = *req.Visible
	}
	st, err := a.svc.Streams.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/streams/"+itoa(st.ID))
	c.JSON(http.StatusCreated, a.toStreamJSON(st))
}

func (a *api) updateStream(c *gin.Context) {
	id, ok := pathID(c, service.CodeStreamNotFound, "stream")
	if !ok {
		return
	}
	var req struct {
		Name    optional[string] `json:"name"`
		URL     optional[string] `json:"url"`
		ZoneID  optional[int]    `json:"zone_id"`
		Visible optional[bool]   `json:"visible"`
	}
	if !bindJSON(c, &req) {
		return
	}
	st, err := a.svc.Streams.Update(c.Request.Context(), id, service.StreamUpdate{
		Name: req.Name.patch(), URL: req.URL.patch(), ZoneID: req.ZoneID.patch(), Visible: req.Visible.patch(),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, a.toStreamJSON(st))
}

func (a *api) deleteStream(c *gin.Context) {
	id, ok := pathID(c, service.CodeStreamNotFound, "stream")
	if !ok {
		return
	}
	if err := a.svc.Streams.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
