package handlers

// Smoke reports: post-harvest tasting notes attached to a plant. Reads
// follow the plant page's visibility (public under guest mode); writes are
// session-gated. The service keeps a report and its scores one record.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/chrono"
	"isley/logger"
	"isley/model/types"
	"isley/service"
)

// SmokeRatingScale returns the selectable ratings in ascending order, for
// the report modals' star groups.
func SmokeRatingScale() []int {
	scale := make([]int, 0, service.SmokeRatingMax-service.SmokeRatingMin+1)
	for v := service.SmokeRatingMin; v <= service.SmokeRatingMax; v++ {
		scale = append(scale, v)
	}
	return scale
}

// SmokeEligibility tells the plant page whether to offer "+ Add"
// (harvested) and whether to show the reports card (harvested or has
// reports: a plant whose harvest entry was deleted keeps its reports
// reachable). A failed lookup hides the card rather than the page.
func SmokeEligibility(ctx context.Context, svc *service.Services, plantID int) (harvested, hasReports bool) {
	harvested, hasReports, err := svc.SmokeReports.Eligibility(ctx, plantID)
	if err != nil {
		logger.Log.WithError(err).WithField("plant", plantID).Error("Failed to check smoke report eligibility")
		return false, false
	}
	return harvested, hasReports
}

// dayTime is a calendar day as the instance zone's midnight, the shape the
// report JSON has always had (the pages read its first ten characters).
func dayTime(svc *service.Services, d chrono.Date) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, svc.TimeZones().Instance())
}

func toTypeSmokeReport(svc *service.Services, r service.SmokeReport) types.SmokeReport {
	out := types.SmokeReport{
		ID: r.ID, PlantID: r.PlantID, ReportDate: dayTime(svc, r.ReportDate),
		ConsumptionMethod: r.ConsumptionMethod, Rating: r.Rating, Description: r.Description,
		CureDays: r.CureDays, Scores: make([]types.SmokeReportScore, len(r.Scores)),
	}
	for i, s := range r.Scores {
		out.Scores[i] = types.SmokeReportScore{CriterionID: s.Criterion.ID, Name: s.Criterion.Name, Value: s.Value}
	}
	return out
}

// GetSmokeReportsHandler returns a plant's reports, latest first.
func GetSmokeReportsHandler(c *gin.Context) {
	plantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}
	svc := ServicesFromContext(c)
	reports, err := svc.SmokeReports.ForPlant(c.Request.Context(), plantID)
	if err != nil {
		if service.IsKind(err, service.KindNotFound) {
			c.JSON(http.StatusOK, []types.SmokeReport{})
			return
		}
		apiServiceError(c, err, "api_internal_error")
		return
	}
	out := make([]types.SmokeReport, len(reports))
	for i, r := range reports {
		out[i] = toTypeSmokeReport(svc, r)
	}
	c.JSON(http.StatusOK, out)
}

// smokeReportRequest is the report form.
type smokeReportRequest struct {
	ReportDate        string `json:"report_date"`
	ConsumptionMethod string `json:"consumption_method"`
	Rating            int    `json:"rating"`
	Description       string `json:"description"`
	Scores            []struct {
		CriterionID int `json:"criterion_id"`
		Value       int `json:"value"`
	} `json:"scores"`
}

// input reads the form; the date is a day (a date-time sent by an older
// page is cut to its day).
func (r smokeReportRequest) input() (service.SmokeReportInput, bool) {
	raw := strings.TrimSpace(r.ReportDate)
	if len(raw) < len(chrono.DateLayout) {
		return service.SmokeReportInput{}, false
	}
	day, err := chrono.ParseDate(raw[:len(chrono.DateLayout)])
	if err != nil {
		return service.SmokeReportInput{}, false
	}
	in := service.SmokeReportInput{
		ReportDate: day, ConsumptionMethod: r.ConsumptionMethod, Rating: r.Rating, Description: r.Description,
		Scores: make([]service.SmokeScoreInput, len(r.Scores)),
	}
	for i, s := range r.Scores {
		in.Scores[i] = service.SmokeScoreInput{CriterionID: s.CriterionID, Value: s.Value}
	}
	return in, true
}

// smokeReportError keeps the report form's translated messages.
func smokeReportError(c *gin.Context, err error, fallbackKey string) {
	if e, ok := service.AsError(err); ok {
		if e.Code == service.CodePlantNotHarvested {
			apiBadRequest(c, "api_plant_not_harvested")
			return
		}
		for field, problem := range e.Fields {
			switch {
			case field == "rating" || strings.HasSuffix(field, ".value"):
				apiBadRequest(c, "api_invalid_smoke_rating")
			case strings.HasSuffix(field, ".criterion_id") && strings.Contains(problem, "twice"):
				apiBadRequest(c, "api_duplicate_smoke_score")
			case strings.HasSuffix(field, ".criterion_id"):
				apiBadRequest(c, "api_invalid_smoke_criterion_id")
			case field == "consumption_method":
				apiBadRequest(c, "api_invalid_smoke_consumption_method")
			case field == "description":
				apiBadRequest(c, "api_invalid_smoke_description")
			default:
				continue
			}
			return
		}
	}
	apiServiceError(c, err, fallbackKey)
}

// AddSmokeReportHandler creates a report for a harvested plant.
func AddSmokeReportHandler(c *gin.Context) {
	plantID, err := strconv.Atoi(c.Param("plantID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}
	var req smokeReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	in, ok := req.input()
	if !ok {
		apiBadRequest(c, "api_invalid_smoke_report_date")
		return
	}
	r, err := ServicesFromContext(c).SmokeReports.Create(c.Request.Context(), plantID, in)
	if err != nil {
		smokeReportError(c, err, "api_failed_to_add_smoke_report")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": r.ID})
}

// UpdateSmokeReportHandler rewrites a report and its scores.
func UpdateSmokeReportHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_report_id")
		return
	}
	var req smokeReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	in, ok := req.input()
	if !ok {
		apiBadRequest(c, "api_invalid_smoke_report_date")
		return
	}
	if _, err := ServicesFromContext(c).SmokeReports.Update(c.Request.Context(), id, in); err != nil {
		smokeReportError(c, err, "api_failed_to_update_smoke_report")
		return
	}
	apiOK(c, "api_smoke_report_updated")
}

// DeleteSmokeReportHandler deletes a report.
func DeleteSmokeReportHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_report_id")
		return
	}
	if err := ServicesFromContext(c).SmokeReports.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_smoke_report")
		return
	}
	apiOK(c, "api_smoke_report_deleted")
}

// GetStrainSmokeSummaryHandler serves the strain page's tasting card.
func GetStrainSmokeSummaryHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	svc := ServicesFromContext(c)
	sum, err := svc.SmokeReports.StrainSummary(c.Request.Context(), strainID)
	out := types.SmokeReportSummary{Entries: []types.SmokeReportSummaryRow{}}
	if err != nil {
		if !service.IsKind(err, service.KindNotFound) {
			apiServiceError(c, err, "api_internal_error")
			return
		}
		c.JSON(http.StatusOK, out)
		return
	}
	out.Average, out.Count = sum.Average, len(sum.Entries)
	for _, e := range sum.Entries {
		out.Entries = append(out.Entries, types.SmokeReportSummaryRow{
			PlantID: e.Plant.ID, PlantName: e.Plant.Name, Rating: e.Rating, ReportDate: dayTime(svc, e.ReportDate),
		})
	}
	c.JSON(http.StatusOK, out)
}
