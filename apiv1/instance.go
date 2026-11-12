package apiv1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type instanceJSON struct {
	Version   string           `json:"version"`
	TimeZone  string           `json:"time_zone"`
	Now       time.Time        `json:"now"`
	GuestMode bool             `json:"guest_mode"`
	Features  instanceFeatures `json:"features"`
	API       instanceAPI      `json:"api"`
}

type instanceFeatures struct {
	ACInfinity    bool `json:"ac_infinity"`
	EcoWitt       bool `json:"ecowitt"`
	SensorIngest  bool `json:"sensor_ingest"`
	CannaDB       bool `json:"cannadb"`
	StreamCapture bool `json:"stream_capture"`
	MCP           bool `json:"mcp"`
}

// instanceAPI tells a client about its own key, so it can see what it is
// allowed to do before trying.
type instanceAPI struct {
	KeyName string   `json:"key_name"`
	Scopes  []string `json:"scopes"`
}

func (a *api) getInstance(c *gin.Context) {
	inst, err := a.svc.Instance.Get(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	key := c.MustGet(contextKeyAuth).(*Key)
	c.JSON(http.StatusOK, instanceJSON{
		Version:   inst.Version,
		TimeZone:  inst.TimeZone,
		Now:       apiTime(inst.Now),
		GuestMode: inst.GuestMode,
		Features: instanceFeatures{
			ACInfinity:    inst.Features.ACInfinity,
			EcoWitt:       inst.Features.EcoWitt,
			SensorIngest:  inst.Features.SensorIngest,
			CannaDB:       inst.Features.CannaDB,
			StreamCapture: inst.Features.StreamCapture,
			MCP:           inst.Features.MCP,
		},
		API: instanceAPI{KeyName: key.Name, Scopes: key.Scopes.List()},
	})
}
