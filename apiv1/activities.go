package apiv1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
)

func toActivityJSON(a service.PlantActivity, plant refJSON) activityJSON {
	out := activityJSON{
		ID: a.ID, Plant: plant, ActivityType: refJSON{ID: a.Type.ID, Name: a.Type.Name}, Note: a.Note,
		At: apiTime(a.At), IsWatering: a.IsWatering, IsFeeding: a.IsFeeding,
		Measurements: make([]measurementJSON, len(a.Measurements)),
	}
	for i, m := range a.Measurements {
		out.Measurements[i] = toMeasurementJSON(m)
	}
	return out
}

func toEntryJSON(e service.ActivityEntry) activityJSON {
	return toActivityJSON(e.PlantActivity, refJSON{ID: e.Plant.ID, Name: e.Plant.Name})
}

// offsetCursor pages lists whose order isn't a unique key.
type offsetCursor struct {
	Offset int `json:"offset"`
}

func readOffset(c *gin.Context) (Page, int, bool) {
	page, ok := parsePage(c)
	if !ok {
		return page, 0, false
	}
	var cur offsetCursor
	if !decodeCursor(c, page, &cur) {
		return page, 0, false
	}
	if cur.Offset < 0 {
		invalid(c, map[string]string{"cursor": "is not a cursor this API returned"})
		return page, 0, false
	}
	return page, cur.Offset, true
}

func nextOffset(c *gin.Context, offset, got, total int) (*string, bool) {
	if offset+got >= total || got == 0 {
		return nil, true
	}
	next, err := encodeCursor(offsetCursor{Offset: offset + got})
	if err != nil {
		writeError(c, err)
		return nil, false
	}
	return next, true
}

type measurementValueJSON struct {
	MetricID int     `json:"metric_id"`
	Value    float64 `json:"value"`
}

func toValues(in []measurementValueJSON) []service.MeasurementValue {
	out := make([]service.MeasurementValue, len(in))
	for i, m := range in {
		out[i] = service.MeasurementValue{MetricID: m.MetricID, Value: m.Value}
	}
	return out
}

type activityCreateJSON struct {
	ActivityTypeID int                    `json:"activity_type_id"`
	Note           string                 `json:"note"`
	At             *inputTime             `json:"at"`
	Measurements   []measurementValueJSON `json:"measurements"`
}

func (a *api) activityInput(c *gin.Context, req activityCreateJSON) (service.ActivityInput, bool) {
	problems := map[string]string{}
	in := service.ActivityInput{TypeID: req.ActivityTypeID, Note: req.Note, Measurements: toValues(req.Measurements)}
	if at := a.parseTime("at", req.At, problems); at != nil {
		in.At = *at
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return in, false
	}
	return in, true
}

func (a *api) createActivity(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req activityCreateJSON
	if !bindJSON(c, &req) {
		return
	}
	in, ok := a.activityInput(c, req)
	if !ok {
		return
	}
	e, err := a.svc.Activities.Create(c.Request.Context(), plantID, in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/activities/"+itoa(e.ID))
	c.JSON(http.StatusCreated, toEntryJSON(e))
}

func (a *api) createActivitiesBulk(c *gin.Context) {
	var req struct {
		activityCreateJSON
		PlantIDs []int `json:"plant_ids"`
	}
	if !bindJSON(c, &req) {
		return
	}
	in, ok := a.activityInput(c, req.activityCreateJSON)
	if !ok {
		return
	}
	entries, err := a.svc.Activities.CreateBulk(c.Request.Context(), req.PlantIDs, in)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]activityJSON, len(entries))
	for i, e := range entries {
		out[i] = toEntryJSON(e)
	}
	c.JSON(http.StatusCreated, NewList(out, nil))
}

func (a *api) getActivity(c *gin.Context) {
	id, ok := pathID(c, service.CodeActivityNotFound, "activity")
	if !ok {
		return
	}
	e, err := a.svc.Activities.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toEntryJSON(e))
}

func (a *api) updateActivity(c *gin.Context) {
	id, ok := pathID(c, service.CodeActivityNotFound, "activity")
	if !ok {
		return
	}
	var req struct {
		ActivityTypeID optional[int]                    `json:"activity_type_id"`
		Note           optional[string]                 `json:"note"`
		At             optional[inputTime]              `json:"at"`
		Measurements   optional[[]measurementValueJSON] `json:"measurements"`
	}
	if !bindJSON(c, &req) {
		return
	}
	upd := service.ActivityUpdate{TypeID: req.ActivityTypeID.patch(), Note: req.Note.patch()}
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
	if req.Measurements.set {
		values := []service.MeasurementValue{}
		if req.Measurements.value != nil {
			values = toValues(*req.Measurements.value)
		}
		upd.Measurements = service.Put(&values)
	}
	e, err := a.svc.Activities.Update(c.Request.Context(), id, upd)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toEntryJSON(e))
}

func (a *api) deleteActivity(c *gin.Context) {
	id, ok := pathID(c, service.CodeActivityNotFound, "activity")
	if !ok {
		return
	}
	if err := a.svc.Activities.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) writeActivities(c *gin.Context, entries []service.ActivityEntry, offset, total int) {
	out := make([]activityJSON, len(entries))
	for i, e := range entries {
		out[i] = toEntryJSON(e)
	}
	next, ok := nextOffset(c, offset, len(entries), total)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) listPlantActivities(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	entries, total, err := a.svc.Activities.ForPlant(c.Request.Context(), plantID, offset, page.Limit)
	if err != nil {
		writeError(c, err)
		return
	}
	a.writeActivities(c, entries, offset, total)
}

func queryDay(c *gin.Context, name string, problems map[string]string) *chrono.Date {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	d, err := chrono.ParseDate(raw)
	if err != nil {
		problems[name] = "must be a day, YYYY-MM-DD"
		return nil
	}
	return &d
}

func (a *api) listActivities(c *gin.Context) {
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	problems := map[string]string{}
	f := service.ActivityFilter{
		PlantID: queryID(c, "plant_id", problems),
		ZoneID:  queryID(c, "zone_id", problems),
		From:    queryDay(c, "from", problems),
		To:      queryDay(c, "to", problems),
		Query:   c.Query("q"),
		Offset:  offset,
		Limit:   page.Limit,
	}
	for _, raw := range c.QueryArray("activity_type_id") {
		for _, tok := range strings.Split(raw, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(tok))
			if err != nil || id <= 0 {
				problems["activity_type_id"] = "must be positive whole numbers"
				continue
			}
			f.TypeIDs = append(f.TypeIDs, id)
		}
	}
	switch order := c.Query("order"); order {
	case "", "newest":
	case "oldest":
		f.Order = service.OldestFirst
	default:
		problems["order"] = "must be newest or oldest"
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	entries, total, err := a.svc.Activities.Log(c.Request.Context(), f)
	if err != nil {
		writeError(c, err)
		return
	}
	a.writeActivities(c, entries, offset, total)
}

func (a *api) listMeasurements(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	problems := map[string]string{}
	metricID := queryID(c, "metric_id", problems)
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	ms, total, err := a.svc.Measurements.ForPlant(c.Request.Context(), plantID, metricID, offset, page.Limit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]measurementJSON, len(ms))
	for i, m := range ms {
		out[i] = toMeasurementJSON(m)
	}
	next, ok := nextOffset(c, offset, len(ms), total)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) createMeasurement(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req struct {
		MetricID int        `json:"metric_id"`
		Value    *float64   `json:"value"`
		At       *inputTime `json:"at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	problems := map[string]string{}
	in := service.MeasurementInput{MetricID: req.MetricID}
	if req.Value == nil {
		problems["value"] = "is required"
	} else {
		in.Value = *req.Value
	}
	if at := a.parseTime("at", req.At, problems); at != nil {
		in.At = *at
	}
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	m, err := a.svc.Measurements.Create(c.Request.Context(), plantID, in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/measurements/"+itoa(m.ID))
	c.JSON(http.StatusCreated, toMeasurementJSON(m))
}

func (a *api) updateMeasurement(c *gin.Context) {
	id, ok := pathID(c, service.CodeMeasurementNotFound, "measurement")
	if !ok {
		return
	}
	var req struct {
		Value optional[float64]   `json:"value"`
		At    optional[inputTime] `json:"at"`
	}
	if !bindJSON(c, &req) {
		return
	}
	upd := service.MeasurementUpdate{Value: req.Value.patch()}
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
	m, err := a.svc.Measurements.Update(c.Request.Context(), id, upd)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toMeasurementJSON(m))
}

func (a *api) deleteMeasurement(c *gin.Context) {
	id, ok := pathID(c, service.CodeMeasurementNotFound, "measurement")
	if !ok {
		return
	}
	if err := a.svc.Measurements.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
