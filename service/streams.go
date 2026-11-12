package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"isley/config"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

// CodeStreamNotFound is the code for a missing stream.
const CodeStreamNotFound = "stream_not_found"

// Stream is a camera feed shown with a zone. The grabber saves its latest
// frame as <StreamDir>/stream_<id>_latest.jpg.
type Stream struct {
	ID      int
	Name    string
	URL     string
	Zone    Ref
	Visible bool
}

// StreamInput is a new stream.
type StreamInput struct {
	Name    string
	URL     string
	ZoneID  int
	Visible bool
}

// StreamUpdate changes the fields that are set.
type StreamUpdate struct {
	Name    Patch[string]
	URL     Patch[string]
	ZoneID  Patch[int]
	Visible Patch[bool]
}

// StreamService owns camera streams.
type StreamService struct {
	deps Deps
}

func streamNotFound(id int) error {
	return NotFound(CodeStreamNotFound, fmt.Sprintf("stream %d does not exist", id))
}

// LatestFramePath is where the grabber keeps a stream's latest frame.
func (s *StreamService) LatestFramePath(id int) string {
	return filepath.Join(s.deps.StreamDir, fmt.Sprintf("stream_%d_latest.jpg", id))
}

const streamSelect = `
	SELECT st.id, st.name, st.url, st.zone_id, COALESCE(z.name, ''), st.visible
	FROM streams st LEFT JOIN zones z ON z.id = st.zone_id`

// List returns streams by zone name then stream name; zoneID 0 means all.
func (s *StreamService) List(ctx context.Context, zoneID int) ([]Stream, error) {
	q, args := streamSelect, []any{}
	if zoneID > 0 {
		q += " WHERE st.zone_id = $1"
		args = append(args, zoneID)
	}
	return s.query(ctx, q+" ORDER BY LOWER(COALESCE(z.name, '')), LOWER(st.name), st.id", args...)
}

// Get returns one stream.
func (s *StreamService) Get(ctx context.Context, id int) (Stream, error) {
	out, err := s.query(ctx, streamSelect+" WHERE st.id = $1", id)
	if err != nil {
		return Stream{}, err
	}
	if len(out) == 0 {
		return Stream{}, streamNotFound(id)
	}
	return out[0], nil
}

func (s *StreamService) query(ctx context.Context, q string, args ...any) ([]Stream, error) {
	return queryStreams(ctx, s.deps.DB, q, args...)
}

func queryStreams(ctx context.Context, db querier, q string, args ...any) ([]Stream, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer rows.Close()
	out := []Stream{}
	for rows.Next() {
		var st Stream
		if err := rows.Scan(&st.ID, &st.Name, &st.URL, &st.Zone.ID, &st.Zone.Name, &st.Visible); err != nil {
			return nil, fmt.Errorf("scan stream: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	return out, nil
}

func (s *StreamService) validate(ctx context.Context, v *Validation, st *Stream) error {
	st.Name = strings.TrimSpace(st.Name)
	st.URL = strings.TrimSpace(st.URL)
	checkName(v, "name", st.Name)
	if err := utils.ValidateStreamURL(st.URL); err != nil {
		v.Add("url", "must be an http, https, rtsp or rtmp address")
	}
	if st.Zone.ID <= 0 {
		v.Add("zone_id", "is required")
		return nil
	}
	found, err := existingIDs(ctx, s.deps.DB, "zones", []int{st.Zone.ID})
	if err != nil {
		return err
	}
	if !found[st.Zone.ID] {
		v.Add("zone_id", "does not exist")
	}
	return nil
}

// Create adds a stream. The grabber fetches its frames.
func (s *StreamService) Create(ctx context.Context, in StreamInput) (Stream, error) {
	st := Stream{Name: in.Name, URL: in.URL, Zone: Ref{ID: in.ZoneID}, Visible: in.Visible}
	var v Validation
	if err := s.validate(ctx, &v, &st); err != nil {
		return Stream{}, err
	}
	if err := v.Err(); err != nil {
		return Stream{}, err
	}
	var id int
	if err := s.deps.DB.QueryRowContext(ctx,
		"INSERT INTO streams (name, url, zone_id, visible) VALUES ($1, $2, $3, $4) RETURNING id",
		st.Name, st.URL, st.Zone.ID, st.Visible).Scan(&id); err != nil {
		return Stream{}, fmt.Errorf("add stream: %w", err)
	}
	return s.afterWrite(ctx, id)
}

// Update changes a stream. Input is checked before the lookup, so a bad
// request is reported as such whether or not the stream exists.
func (s *StreamService) Update(ctx context.Context, id int, upd StreamUpdate) (Stream, error) {
	str := func(p Patch[string]) string {
		if p.Value == nil {
			return ""
		}
		return *p.Value
	}
	var v Validation
	probe := Stream{Name: "-", URL: "http://-", Zone: Ref{ID: -1}}
	if upd.Name.Set {
		probe.Name = str(upd.Name)
	}
	if upd.URL.Set {
		probe.URL = str(upd.URL)
	}
	if upd.ZoneID.Set {
		probe.Zone.ID = 0
		if upd.ZoneID.Value != nil {
			probe.Zone.ID = *upd.ZoneID.Value
		}
	}
	if probe.Zone.ID == -1 {
		// Zone unchanged: only name and URL to check here.
		checkName(&v, "name", strings.TrimSpace(probe.Name))
		if err := utils.ValidateStreamURL(strings.TrimSpace(probe.URL)); err != nil {
			v.Add("url", "must be an http, https, rtsp or rtmp address")
		}
	} else if err := s.validate(ctx, &v, &probe); err != nil {
		return Stream{}, err
	}
	if err := v.Err(); err != nil {
		return Stream{}, err
	}

	st, err := s.Get(ctx, id)
	if err != nil {
		return Stream{}, err
	}
	if upd.Name.Set {
		st.Name = strings.TrimSpace(str(upd.Name))
	}
	if upd.URL.Set {
		st.URL = strings.TrimSpace(str(upd.URL))
	}
	if upd.ZoneID.Set {
		st.Zone.ID = probe.Zone.ID
	}
	if upd.Visible.Set {
		st.Visible = upd.Visible.Value != nil && *upd.Visible.Value
	}
	if _, err := s.deps.DB.ExecContext(ctx,
		"UPDATE streams SET name = $1, url = $2, zone_id = $3, visible = $4 WHERE id = $5",
		st.Name, st.URL, st.Zone.ID, st.Visible, id); err != nil {
		return Stream{}, fmt.Errorf("update stream %d: %w", id, err)
	}
	return s.afterWrite(ctx, id)
}

// Delete deletes a stream and its saved frame.
func (s *StreamService) Delete(ctx context.Context, id int) error {
	res, err := s.deps.DB.ExecContext(ctx, "DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete stream %d: %w", id, err)
	}
	if err := requireRow(res, streamNotFound(id)); err != nil {
		return err
	}
	if err := os.Remove(s.LatestFramePath(id)); err != nil && !os.IsNotExist(err) {
		logger.Log.WithError(err).WithField("stream", id).Warn("Failed to delete stream frame")
	}
	return refreshStreams(ctx, s.deps.DB, s.deps.Store)
}

func (s *StreamService) afterWrite(ctx context.Context, id int) (Stream, error) {
	if err := refreshStreams(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Stream{}, err
	}
	return s.Get(ctx, id)
}

// refreshStreams reloads the stream cache the grabber and pages read.
func refreshStreams(ctx context.Context, q querier, store *config.Store) error {
	streams, err := queryStreams(ctx, q, streamSelect+" ORDER BY st.id")
	if err != nil {
		return err
	}
	out := make([]types.Stream, len(streams))
	for i, st := range streams {
		out[i] = types.Stream{
			ID: uint(st.ID), Name: st.Name, URL: st.URL, ZoneID: uint(st.Zone.ID),
			ZoneName: st.Zone.Name, Visible: st.Visible,
		}
	}
	store.SetStreams(out)
	return nil
}
