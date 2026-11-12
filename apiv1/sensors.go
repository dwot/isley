package apiv1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"isley/service"
)

type sensorJSON struct {
	ID         int          `json:"id"`
	Name       string       `json:"name"`
	Zone       *refJSON     `json:"zone"`
	Source     string       `json:"source"`
	Device     string       `json:"device"`
	Type       string       `json:"type"`
	Unit       string       `json:"unit"`
	Visibility string       `json:"visibility"`
	CreatedAt  time.Time    `json:"created_at"`
	Latest     *readingJSON `json:"latest"`
}

func toSensorJSON(sn service.Sensor) sensorJSON {
	out := sensorJSON{
		ID: sn.ID, Name: sn.Name, Source: sn.Source, Device: sn.Device, Type: sn.Type,
		Unit: sn.Unit, Visibility: sn.Visibility, CreatedAt: apiTime(sn.CreatedAt),
	}
	if sn.Zone != nil {
		out.Zone = &refJSON{ID: sn.Zone.ID, Name: sn.Zone.Name}
	}
	if sn.Latest != nil {
		out.Latest = &readingJSON{Value: sn.Latest.Value, At: apiTime(sn.Latest.At)}
	}
	return out
}

func (a *api) listSensors(c *gin.Context) {
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	problems := map[string]string{}
	f := service.SensorFilter{
		ZoneID: queryID(c, "zone_id", problems),
		Source: c.Query("source"),
		Offset: offset,
		Limit:  page.Limit,
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	sensors, total, err := a.svc.Sensors.List(c.Request.Context(), f)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]sensorJSON, len(sensors))
	for i, sn := range sensors {
		out[i] = toSensorJSON(sn)
	}
	next, ok := nextOffset(c, offset, len(out), total)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) getSensor(c *gin.Context) {
	id, ok := pathID(c, service.CodeSensorNotFound, "sensor")
	if !ok {
		return
	}
	sn, err := a.svc.Sensors.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSensorJSON(sn))
}

func (a *api) updateSensor(c *gin.Context) {
	id, ok := pathID(c, service.CodeSensorNotFound, "sensor")
	if !ok {
		return
	}
	var req struct {
		Name       optional[string] `json:"name"`
		ZoneID     optional[int]    `json:"zone_id"`
		Unit       optional[string] `json:"unit"`
		Visibility optional[string] `json:"visibility"`
	}
	if !bindJSON(c, &req) {
		return
	}
	sn, err := a.svc.Sensors.Update(c.Request.Context(), id, service.SensorUpdate{
		Name: req.Name.patch(), ZoneID: req.ZoneID.patch(), Unit: req.Unit.patch(), Visibility: req.Visibility.patch(),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSensorJSON(sn))
}

func (a *api) deleteSensor(c *gin.Context) {
	id, ok := pathID(c, service.CodeSensorNotFound, "sensor")
	if !ok {
		return
	}
	if err := a.svc.Sensors.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
