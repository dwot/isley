package apiv1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"isley/service"
)

type zoneJSON struct {
	ID                  int       `json:"id"`
	Name                string    `json:"name"`
	LeafTempOffsetF     *float64  `json:"leaf_temp_offset_f"`
	VPDTempSensorID     *int      `json:"vpd_temp_sensor_id"`
	VPDHumiditySensorID *int      `json:"vpd_humidity_sensor_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type zoneDetailJSON struct {
	zoneJSON
	Sensors []zoneSensorJSON `json:"sensors"`
}

type zoneSensorJSON struct {
	ID         int          `json:"id"`
	Name       string       `json:"name"`
	Source     string       `json:"source"`
	Device     string       `json:"device"`
	Type       string       `json:"type"`
	Unit       string       `json:"unit"`
	Visibility string       `json:"visibility"`
	Latest     *readingJSON `json:"latest"`
}

type readingJSON struct {
	Value float64   `json:"value"`
	At    time.Time `json:"at"`
}

func toZoneJSON(z service.Zone) zoneJSON {
	return zoneJSON{
		ID: z.ID, Name: z.Name, LeafTempOffsetF: z.LeafTempOffsetF,
		VPDTempSensorID: z.VPDTempSensorID, VPDHumiditySensorID: z.VPDHumiditySensorID,
		CreatedAt: apiTime(z.CreatedAt), UpdatedAt: apiTime(z.UpdatedAt),
	}
}

type zoneCursor struct {
	AfterID int `json:"after_id"`
}

func (a *api) listZones(c *gin.Context) {
	page, ok := parsePage(c)
	if !ok {
		return
	}
	var cur zoneCursor
	if !decodeCursor(c, page, &cur) {
		return
	}
	zones, more, err := a.svc.Zones.Page(c.Request.Context(), cur.AfterID, page.Limit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]zoneJSON, len(zones))
	for i, z := range zones {
		out[i] = toZoneJSON(z)
	}
	var next *string
	if more {
		if next, err = encodeCursor(zoneCursor{AfterID: zones[len(zones)-1].ID}); err != nil {
			writeError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) getZone(c *gin.Context) {
	id, ok := pathID(c, service.CodeZoneNotFound, "zone")
	if !ok {
		return
	}
	d, err := a.svc.Zones.Detail(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	a.writeZoneDetail(c, http.StatusOK, d)
}

func (a *api) writeZoneDetail(c *gin.Context, status int, d service.ZoneDetail) {
	sensors := make([]zoneSensorJSON, len(d.Sensors))
	for i, s := range d.Sensors {
		sensors[i] = zoneSensorJSON{
			ID: s.ID, Name: s.Name, Source: s.Source, Device: s.Device, Type: s.Type,
			Unit: s.Unit, Visibility: s.Visibility,
		}
		if s.Latest != nil {
			sensors[i].Latest = &readingJSON{Value: s.Latest.Value, At: apiTime(s.Latest.At)}
		}
	}
	c.JSON(status, zoneDetailJSON{zoneJSON: toZoneJSON(d.Zone), Sensors: sensors})
}

type zoneCreateJSON struct {
	Name                string   `json:"name"`
	LeafTempOffsetF     *float64 `json:"leaf_temp_offset_f"`
	VPDTempSensorID     *int     `json:"vpd_temp_sensor_id"`
	VPDHumiditySensorID *int     `json:"vpd_humidity_sensor_id"`
}

func (a *api) createZone(c *gin.Context) {
	var req zoneCreateJSON
	if !bindJSON(c, &req) {
		return
	}
	ctx := c.Request.Context()
	z, err := a.svc.Zones.Create(ctx, service.ZoneInput{
		Name: req.Name, LeafTempOffsetF: req.LeafTempOffsetF,
		VPDTempSensorID: req.VPDTempSensorID, VPDHumiditySensorID: req.VPDHumiditySensorID,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	d, err := a.svc.Zones.Detail(ctx, z.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/zones/"+itoa(z.ID))
	a.writeZoneDetail(c, http.StatusCreated, d)
}

type zonePatchJSON struct {
	Name                optional[string]  `json:"name"`
	LeafTempOffsetF     optional[float64] `json:"leaf_temp_offset_f"`
	VPDTempSensorID     optional[int]     `json:"vpd_temp_sensor_id"`
	VPDHumiditySensorID optional[int]     `json:"vpd_humidity_sensor_id"`
}

func (a *api) updateZone(c *gin.Context) {
	id, ok := pathID(c, service.CodeZoneNotFound, "zone")
	if !ok {
		return
	}
	var req zonePatchJSON
	if !bindJSON(c, &req) {
		return
	}
	ctx := c.Request.Context()
	if _, err := a.svc.Zones.Update(ctx, id, service.ZoneUpdate{
		Name:                req.Name.patch(),
		LeafTempOffsetF:     req.LeafTempOffsetF.patch(),
		VPDTempSensorID:     req.VPDTempSensorID.patch(),
		VPDHumiditySensorID: req.VPDHumiditySensorID.patch(),
	}); err != nil {
		writeError(c, err)
		return
	}
	d, err := a.svc.Zones.Detail(ctx, id)
	if err != nil {
		writeError(c, err)
		return
	}
	a.writeZoneDetail(c, http.StatusOK, d)
}

func (a *api) deleteZone(c *gin.Context) {
	id, ok := pathID(c, service.CodeZoneNotFound, "zone")
	if !ok {
		return
	}
	if err := a.svc.Zones.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
