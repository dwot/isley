package apiv1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
)

// inputTime is a timestamp a client sends: RFC 3339, or without an offset
// to mean the instance zone. It is parsed once the instance zone is known.
type inputTime string

func (a *api) parseTime(field string, raw *inputTime, problems map[string]string) *time.Time {
	if raw == nil {
		return nil
	}
	t, err := a.svc.TimeZones().ParseInput(string(*raw))
	if err != nil {
		problems[field] = "must be a date-time such as 2026-11-11T09:30:00-05:00"
		return nil
	}
	return &t
}

type plantCreateJSON struct {
	Name               string       `json:"name"`
	Description        string       `json:"description"`
	StrainID           int          `json:"strain_id"`
	ZoneID             *int         `json:"zone_id"`
	StatusID           int          `json:"status_id"`
	StartDate          *chrono.Date `json:"start_date"`
	StatusAt           *inputTime   `json:"status_at"`
	Clone              bool         `json:"clone"`
	ParentID           *int         `json:"parent_id"`
	SensorIDs          []int        `json:"sensor_ids"`
	DecrementSeedCount bool         `json:"decrement_seed_count"`
}

func (a *api) createPlant(c *gin.Context) {
	var req plantCreateJSON
	if !bindJSON(c, &req) {
		return
	}
	problems := map[string]string{}
	statusAt := a.parseTime("status_at", req.StatusAt, problems)
	if req.StartDate == nil {
		problems["start_date"] = "is required"
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	ctx := c.Request.Context()
	p, err := a.svc.Plants.Create(ctx, service.PlantInput{
		Name: req.Name, Description: req.Description, StrainID: req.StrainID, ZoneID: req.ZoneID,
		StatusID: req.StatusID, Start: service.OnDay(*req.StartDate), StatusAt: statusAt,
		Clone: req.Clone, ParentID: req.ParentID, SensorIDs: req.SensorIDs,
		DecrementSeedCount: req.DecrementSeedCount,
	})
	if err != nil {
		writeError(c, renameStart(err))
		return
	}
	c.Header("Location", Prefix+"/plants/"+itoa(p.ID))
	a.writePlantDetail(c, http.StatusCreated, p.ID)
}

// renameStart reports the service's "start" field as the API's start_date.
func renameStart(err error) error {
	if e, ok := service.AsError(err); ok {
		if problem, has := e.Fields["start"]; has {
			fields := map[string]string{}
			for k, v := range e.Fields {
				fields[k] = v
			}
			delete(fields, "start")
			fields["start_date"] = problem
			return service.Invalid(fields)
		}
	}
	return err
}

type plantPatchJSON struct {
	Name           optional[string]      `json:"name"`
	Description    optional[string]      `json:"description"`
	StrainID       optional[int]         `json:"strain_id"`
	ZoneID         optional[int]         `json:"zone_id"`
	StartDate      optional[chrono.Date] `json:"start_date"`
	HarvestWeightG optional[float64]     `json:"harvest_weight_g"`
	Clone          optional[bool]        `json:"clone"`
	ParentID       optional[int]         `json:"parent_id"`
}

func (a *api) updatePlant(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req plantPatchJSON
	if !bindJSON(c, &req) {
		return
	}
	upd := service.PlantUpdate{
		Name: req.Name.patch(), Description: req.Description.patch(),
		StrainID: req.StrainID.patch(), ZoneID: req.ZoneID.patch(),
		HarvestWeightG: req.HarvestWeightG.patch(), Clone: req.Clone.patch(),
		ParentID: req.ParentID.patch(),
	}
	if req.StartDate.set {
		upd.Start.Set = true
		if req.StartDate.value != nil {
			w := service.OnDay(*req.StartDate.value)
			upd.Start.Value = &w
		}
	}
	if _, err := a.svc.Plants.Update(c.Request.Context(), id, upd); err != nil {
		writeError(c, renameStart(err))
		return
	}
	a.writePlantDetail(c, http.StatusOK, id)
}

func (a *api) deletePlant(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	if err := a.svc.Plants.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) setPlantSensors(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req struct {
		SensorIDs *[]int `json:"sensor_ids"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.SensorIDs == nil {
		invalid(c, map[string]string{"sensor_ids": "is required (send [] to unlink all)"})
		return
	}
	if err := a.svc.Plants.SetSensors(c.Request.Context(), id, *req.SensorIDs); err != nil {
		writeError(c, err)
		return
	}
	a.writePlantDetail(c, http.StatusOK, id)
}

func toStatusEntryJSON(e service.StatusEntry) statusEntryJSON {
	return statusEntryJSON{ID: e.ID, StatusID: e.StatusID, Name: string(e.Status), At: apiTime(e.At)}
}

func (a *api) listStatusHistory(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	h, err := a.svc.Plants.StatusHistory(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]statusEntryJSON, len(h))
	for i, e := range h {
		out[i] = toStatusEntryJSON(e)
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}

type statusChangeJSON struct {
	Changed bool            `json:"changed"`
	Entry   statusEntryJSON `json:"entry"`
}

func (a *api) changePlantStatus(c *gin.Context) {
	id, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req struct {
		StatusID int        `json:"status_id"`
		At       *inputTime `json:"at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	problems := map[string]string{}
	at := a.parseTime("at", req.At, problems)
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	entry, changed, err := a.svc.Plants.ChangeStatus(c.Request.Context(), id, req.StatusID, at)
	if err != nil {
		writeError(c, err)
		return
	}
	status := http.StatusOK
	if changed {
		status = http.StatusCreated
	}
	c.JSON(status, statusChangeJSON{Changed: changed, Entry: toStatusEntryJSON(entry)})
}

func (a *api) updateStatusEntry(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	entryID, ok := pathParamID(c, "entry_id", service.CodeStatusEntryNotFound, "status entry")
	if !ok {
		return
	}
	var req struct {
		StatusID optional[int]       `json:"status_id"`
		At       optional[inputTime] `json:"at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	upd := service.StatusEntryUpdate{StatusID: req.StatusID.patch()}
	if req.At.set {
		problems := map[string]string{}
		upd.At.Set = true
		if req.At.value != nil {
			upd.At.Value = a.parseTime("at", req.At.value, problems)
		}
		if len(problems) > 0 {
			invalid(c, problems)
			return
		}
	}
	if _, err := a.svc.Plants.Get(c.Request.Context(), plantID); err != nil {
		writeError(c, err)
		return
	}
	e, err := a.svc.Plants.UpdateStatusEntry(c.Request.Context(), plantID, entryID, upd)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStatusEntryJSON(e))
}

func (a *api) deleteStatusEntry(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	entryID, ok := pathParamID(c, "entry_id", service.CodeStatusEntryNotFound, "status entry")
	if !ok {
		return
	}
	if _, err := a.svc.Plants.Get(c.Request.Context(), plantID); err != nil {
		writeError(c, err)
		return
	}
	if err := a.svc.Plants.DeleteStatusEntry(c.Request.Context(), plantID, entryID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writePlantDetail answers with the plant as it now stands.
func (a *api) writePlantDetail(c *gin.Context, status int, id int) {
	d, err := a.svc.Plants.Detail(c.Request.Context(), id, RecentLimit)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(status, toPlantDetailJSON(d))
}
