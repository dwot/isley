package apiv1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/service"
)

type smokeCriterionJSON struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
}

type smokeScoreJSON struct {
	Criterion refJSON `json:"criterion"`
	Value     int     `json:"value"`
}

type smokeReportJSON struct {
	ID                int              `json:"id"`
	PlantID           int              `json:"plant_id"`
	ReportDate        chrono.Date      `json:"report_date"`
	ConsumptionMethod string           `json:"consumption_method"`
	Rating            int              `json:"rating"`
	Description       string           `json:"description"`
	CureDays          *int             `json:"cure_days"`
	Scores            []smokeScoreJSON `json:"scores"`
}

func toSmokeReportJSON(r service.SmokeReport) smokeReportJSON {
	out := smokeReportJSON{
		ID: r.ID, PlantID: r.PlantID, ReportDate: r.ReportDate, ConsumptionMethod: r.ConsumptionMethod,
		Rating: r.Rating, Description: r.Description, CureDays: r.CureDays,
		Scores: make([]smokeScoreJSON, len(r.Scores)),
	}
	for i, s := range r.Scores {
		out.Scores[i] = smokeScoreJSON{Criterion: refJSON{ID: s.Criterion.ID, Name: s.Criterion.Name}, Value: s.Value}
	}
	return out
}

type smokeScoreInputJSON struct {
	CriterionID int `json:"criterion_id"`
	Value       int `json:"value"`
}

func toScoreInputs(in []smokeScoreInputJSON) []service.SmokeScoreInput {
	out := make([]service.SmokeScoreInput, len(in))
	for i, s := range in {
		out[i] = service.SmokeScoreInput{CriterionID: s.CriterionID, Value: s.Value}
	}
	return out
}

func (a *api) listSmokeCriteria(c *gin.Context) {
	all, err := a.svc.SmokeReports.Criteria(c.Request.Context(), false)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]smokeCriterionJSON, len(all))
	for i, cr := range all {
		out[i] = smokeCriterionJSON{ID: cr.ID, Name: cr.Name, SortOrder: cr.SortOrder, Active: cr.Active}
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}

func (a *api) listPlantSmokeReports(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	reports, err := a.svc.SmokeReports.ForPlant(c.Request.Context(), plantID)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]smokeReportJSON, len(reports))
	for i, r := range reports {
		out[i] = toSmokeReportJSON(r)
	}
	c.JSON(http.StatusOK, NewList(out, nil))
}

func (a *api) getSmokeReport(c *gin.Context) {
	id, ok := pathID(c, service.CodeSmokeReportNotFound, "smoke report")
	if !ok {
		return
	}
	r, err := a.svc.SmokeReports.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSmokeReportJSON(r))
}

func (a *api) createSmokeReport(c *gin.Context) {
	plantID, ok := pathID(c, service.CodePlantNotFound, "plant")
	if !ok {
		return
	}
	var req struct {
		ReportDate        chrono.Date           `json:"report_date"`
		ConsumptionMethod string                `json:"consumption_method"`
		Rating            int                   `json:"rating"`
		Description       string                `json:"description"`
		Scores            []smokeScoreInputJSON `json:"scores"`
	}
	if !bindJSON(c, &req) {
		return
	}
	r, err := a.svc.SmokeReports.Create(c.Request.Context(), plantID, service.SmokeReportInput{
		ReportDate: req.ReportDate, ConsumptionMethod: req.ConsumptionMethod, Rating: req.Rating,
		Description: req.Description, Scores: toScoreInputs(req.Scores),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/smoke-reports/"+itoa(r.ID))
	c.JSON(http.StatusCreated, toSmokeReportJSON(r))
}

// updateSmokeReport changes the fields given. scores, when given, replace
// the scores on active criteria; scores on archived criteria are kept.
func (a *api) updateSmokeReport(c *gin.Context) {
	id, ok := pathID(c, service.CodeSmokeReportNotFound, "smoke report")
	if !ok {
		return
	}
	var req struct {
		ReportDate        optional[chrono.Date]           `json:"report_date"`
		ConsumptionMethod optional[string]                `json:"consumption_method"`
		Rating            optional[int]                   `json:"rating"`
		Description       optional[string]                `json:"description"`
		Scores            optional[[]smokeScoreInputJSON] `json:"scores"`
	}
	if !bindJSON(c, &req) {
		return
	}
	ctx := c.Request.Context()
	cur, err := a.svc.SmokeReports.Get(ctx, id)
	if err != nil {
		writeError(c, err)
		return
	}
	in := service.SmokeReportInput{
		ReportDate: cur.ReportDate, ConsumptionMethod: cur.ConsumptionMethod, Rating: cur.Rating,
		Description: cur.Description,
	}
	if req.ReportDate.set && req.ReportDate.value != nil {
		in.ReportDate = *req.ReportDate.value
	}
	if req.ConsumptionMethod.set {
		in.ConsumptionMethod = deref(req.ConsumptionMethod.value)
	}
	if req.Rating.set {
		in.Rating = deref(req.Rating.value)
	}
	if req.Description.set {
		in.Description = deref(req.Description.value)
	}
	if req.Scores.set {
		in.Scores = toScoreInputs(deref(req.Scores.value))
	} else {
		// Keep the current scores: resend those on active criteria (the
		// service leaves archived ones alone).
		active, err := a.svc.SmokeReports.Criteria(ctx, true)
		if err != nil {
			writeError(c, err)
			return
		}
		isActive := map[int]bool{}
		for _, cr := range active {
			isActive[cr.ID] = true
		}
		for _, s := range cur.Scores {
			if isActive[s.Criterion.ID] {
				in.Scores = append(in.Scores, service.SmokeScoreInput{CriterionID: s.Criterion.ID, Value: s.Value})
			}
		}
	}
	r, err := a.svc.SmokeReports.Update(ctx, id, in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toSmokeReportJSON(r))
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func (a *api) deleteSmokeReport(c *gin.Context) {
	id, ok := pathID(c, service.CodeSmokeReportNotFound, "smoke report")
	if !ok {
		return
	}
	if err := a.svc.SmokeReports.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) getStrainSmokeSummary(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	sum, err := a.svc.SmokeReports.StrainSummary(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	type entry struct {
		Plant      refJSON     `json:"plant"`
		Rating     int         `json:"rating"`
		ReportDate chrono.Date `json:"report_date"`
	}
	entries := make([]entry, len(sum.Entries))
	for i, e := range sum.Entries {
		entries[i] = entry{Plant: refJSON{ID: e.Plant.ID, Name: e.Plant.Name}, Rating: e.Rating, ReportDate: e.ReportDate}
	}
	var avg *float64
	if len(entries) > 0 {
		avg = &sum.Average
	}
	c.JSON(http.StatusOK, gin.H{"average_rating": avg, "harvests": len(entries), "entries": entries})
}
