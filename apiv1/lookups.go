package apiv1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/service"
)

type statusJSON struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
	// Phase groups statuses: growing, harvested or dead.
	Phase      string   `json:"phase"`
	VPDLowKPa  *float64 `json:"vpd_low_kpa"`
	VPDHighKPa *float64 `json:"vpd_high_kpa"`
}

func statusPhase(n service.StatusName) string {
	switch {
	case n.IsDead():
		return "dead"
	case n.IsHarvested():
		return "harvested"
	default:
		return "growing"
	}
}

type metricJSON struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Unit    string `json:"unit"`
	BuiltIn bool   `json:"built_in"`
}

type activityTypeJSON struct {
	ID         int                  `json:"id"`
	Name       string               `json:"name"`
	IsWatering bool                 `json:"is_watering"`
	IsFeeding  bool                 `json:"is_feeding"`
	BuiltIn    bool                 `json:"built_in"`
	Metrics    []activityMetricJSON `json:"metrics"`
}

type activityMetricJSON struct {
	MetricID int    `json:"metric_id"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Required bool   `json:"required"`
}

// The lookup lists are short and fixed-ish, so they come back whole in the
// list envelope with next_cursor null.

func (a *api) listStatuses(c *gin.Context) {
	all, err := a.svc.Lookups.Statuses(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]statusJSON, len(all))
	for i, s := range all {
		out[i] = statusJSON{
			ID: s.ID, Name: string(s.Name), Order: s.Order, Phase: statusPhase(s.Name),
			VPDLowKPa: s.VPDLowKPa, VPDHighKPa: s.VPDHighKPa,
		}
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}

func (a *api) listMetrics(c *gin.Context) {
	all, err := a.svc.Lookups.Metrics(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]metricJSON, len(all))
	for i, m := range all {
		out[i] = metricJSON{ID: m.ID, Name: m.Name, Unit: m.Unit, BuiltIn: m.BuiltIn}
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}

func (a *api) listActivityTypes(c *gin.Context) {
	all, err := a.svc.Lookups.ActivityTypes(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]activityTypeJSON, len(all))
	for i, t := range all {
		metrics := make([]activityMetricJSON, len(t.Metrics))
		for j, m := range t.Metrics {
			metrics[j] = activityMetricJSON{MetricID: m.MetricID, Name: m.Name, Unit: m.Unit, Required: m.Required}
		}
		out[i] = activityTypeJSON{
			ID: t.ID, Name: t.Name, IsWatering: t.IsWatering, IsFeeding: t.IsFeeding,
			BuiltIn: t.BuiltIn, Metrics: metrics,
		}
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}
