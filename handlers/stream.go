package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/model/types"
	"isley/service"
	"isley/utils"
)

// GetStreams lists every stream for the settings page.
func GetStreams(ctx context.Context, svc *service.Services) []types.Stream {
	streams, err := svc.Streams.List(ctx, 0)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to list streams")
		return []types.Stream{}
	}
	return toTypeStreams(streams)
}

func toTypeStreams(streams []service.Stream) []types.Stream {
	out := make([]types.Stream, len(streams))
	for i, st := range streams {
		out[i] = types.Stream{
			ID: uint(st.ID), Name: st.Name, URL: st.URL, ZoneID: uint(st.Zone.ID),
			ZoneName: st.Zone.Name, Visible: st.Visible,
		}
	}
	return out
}

// formInt and formBool read a JSON number or boolean, or the string a
// form select sends ("3", "true").
type formInt int

func (f *formInt) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*f = formInt(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	*f = formInt(n)
	return nil
}

type formBool bool

func (f *formBool) UnmarshalJSON(b []byte) error {
	var v bool
	if err := json.Unmarshal(b, &v); err == nil {
		*f = formBool(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return err
	}
	*f = formBool(v)
	return nil
}

type streamRequest struct {
	Name    string   `json:"stream_name"`
	URL     string   `json:"url"`
	ZoneID  formInt  `json:"zone_id"`
	Visible formBool `json:"visible"`
}

// streamsResponse answers with the whole list, which the settings page
// redraws from.
func streamsResponse(c *gin.Context) []types.Stream {
	return GetStreams(c.Request.Context(), ServicesFromContext(c))
}

func AddStreamHandler(c *gin.Context) {
	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	svc := ServicesFromContext(c)
	st, err := svc.Streams.Create(c.Request.Context(), service.StreamInput{
		Name: req.Name, URL: req.URL, ZoneID: int(req.ZoneID), Visible: bool(req.Visible),
	})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_stream")
		return
	}
	// A first frame now, so the dashboard has something before the
	// grabber's next round.
	if err := utils.GrabWebcamImage(st.URL, svc.Streams.LatestFramePath(st.ID)); err != nil {
		logger.Log.WithError(err).WithField("stream", st.ID).Warn("Failed to grab first stream frame")
	}
	c.JSON(http.StatusCreated, gin.H{"id": st.ID, "streams": streamsResponse(c)})
}

func UpdateStreamHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	var req streamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	zoneID, visible := int(req.ZoneID), bool(req.Visible)
	if _, err := ServicesFromContext(c).Streams.Update(c.Request.Context(), id, service.StreamUpdate{
		Name: service.Put(&req.Name), URL: service.Put(&req.URL),
		ZoneID: service.Put(&zoneID), Visible: service.Put(&visible),
	}); err != nil {
		apiServiceError(c, err, "api_failed_to_update_stream")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_updated"), "streams": streamsResponse(c)})
}

func DeleteStreamHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := ServicesFromContext(c).Streams.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_stream")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_deleted"), "streams": streamsResponse(c)})
}

// GetStreamsByZoneHandler serves the dashboard's streams keyed by zone name.
func GetStreamsByZoneHandler(c *gin.Context) {
	streams, err := ServicesFromContext(c).Streams.List(c.Request.Context(), 0)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_read_streams")
		return
	}
	byZone := make(map[string][]types.Stream)
	for _, st := range toTypeStreams(streams) {
		// The dashboard never had zone ids here.
		st.ZoneID = 0
		byZone[st.ZoneName] = append(byZone[st.ZoneName], st)
	}
	c.JSON(http.StatusOK, byZone)
}
