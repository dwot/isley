package apiv1

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"isley/service"
)

const (
	defaultRawLimit = 1000
	maxRawLimit     = 5000
)

type sensorBriefJSON struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Unit string `json:"unit"`
}

type rawPointJSON struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

type bucketJSON struct {
	Start      time.Time `json:"start"`
	Min        float64   `json:"min"`
	Avg        float64   `json:"avg"`
	Max        float64   `json:"max"`
	Count      int       `json:"count"`
	OnFraction *float64  `json:"on_fraction,omitempty"`
	Starts     *int      `json:"starts,omitempty"`
}

type sensorDataJSON struct {
	Sensor     sensorBriefJSON `json:"sensor"`
	From       time.Time       `json:"from"`
	To         time.Time       `json:"to"`
	Bucket     string          `json:"bucket"`
	Items      any             `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

type rawCursor struct {
	After int `json:"after"`
}

// getSensorData returns a sensor's readings over [from, to): raw, or
// summarised per bucket.
func (a *api) getSensorData(c *gin.Context) {
	id, ok := pathID(c, service.CodeSensorNotFound, "sensor")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	problems := map[string]string{}
	to := a.svc.TimeZones().Now(a.svc.Clock())
	if raw := c.Query("to"); raw != "" {
		v := inputTime(raw)
		if t := a.parseTime("to", &v, problems); t != nil {
			to = *t
		}
	}
	from := to.Add(-24 * time.Hour)
	if raw := c.Query("from"); raw != "" {
		v := inputTime(raw)
		if t := a.parseTime("from", &v, problems); t != nil {
			from = *t
		}
	}
	bucketParam := c.Query("bucket")
	isRaw := bucketParam == "raw"
	var size time.Duration
	if !isRaw {
		if bucketParam == "" {
			size = service.DefaultBucket(from, to)
		} else {
			var err error
			if size, err = service.ParseBucket(bucketParam); err != nil {
				problems["bucket"] = `must be "raw" or a size such as 5m, 1h or 1d`
			}
		}
	}
	limit := defaultRawLimit
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxRawLimit {
			problems["limit"] = "must be a whole number from 1 to " + strconv.Itoa(maxRawLimit)
		}
		limit = n
	}
	var cur rawCursor
	if s := c.Query("cursor"); s != "" {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil || json.Unmarshal(b, &cur) != nil || cur.After <= 0 {
			problems["cursor"] = "is not a cursor this API returned"
		}
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}

	sn, err := a.svc.Sensors.Get(ctx, id)
	if err != nil {
		writeError(c, err)
		return
	}
	out := sensorDataJSON{
		Sensor: sensorBriefJSON{ID: sn.ID, Name: sn.Name, Type: sn.Type, Unit: sn.Unit},
		From:   apiTime(from.In(a.svc.TimeZones().Instance())),
		To:     apiTime(to.In(a.svc.TimeZones().Instance())),
	}
	if isRaw {
		points, more, err := a.svc.Sensors.Raw(ctx, id, from, to, cur.After, limit)
		if err != nil {
			writeError(c, err)
			return
		}
		items := make([]rawPointJSON, len(points))
		for i, p := range points {
			items[i] = rawPointJSON{At: apiTime(p.At), Value: p.Value}
		}
		if more {
			if out.NextCursor, err = encodeCursor(rawCursor{After: points[len(points)-1].ID}); err != nil {
				writeError(c, err)
				return
			}
		}
		out.Bucket, out.Items = "raw", items
		c.JSON(http.StatusOK, out)
		return
	}
	buckets, err := a.svc.Sensors.Buckets(ctx, id, from, to, size)
	if err != nil {
		writeError(c, err)
		return
	}
	items := make([]bucketJSON, len(buckets))
	for i, b := range buckets {
		items[i] = bucketJSON{Start: apiTime(b.Start), Min: b.Min, Avg: b.Avg, Max: b.Max, Count: b.Count,
			OnFraction: b.OnFraction, Starts: b.Starts}
	}
	out.Bucket, out.Items = service.FormatBucket(size), items
	c.JSON(http.StatusOK, out)
}
