package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"

	"isley/chrono"
	"isley/logger"
	"isley/service"
	"isley/utils"
)

// ActivityLogEntry is one row returned from the cross-plant activity query.
type ActivityLogEntry struct {
	ID           uint      `json:"id"`
	Date         time.Time `json:"date"`
	Note         string    `json:"note"`
	ActivityID   int       `json:"activity_id"`
	ActivityName string    `json:"activity_name"`
	PlantID      uint      `json:"plant_id"`
	PlantName    string    `json:"plant_name"`
	StrainName   string    `json:"strain_name"`
	ZoneName     string    `json:"zone_name"`
}

// ActivityLogFilters are the filters the activity log pages accept; see
// service.ActivityFilter.
type ActivityLogFilters = service.ActivityFilter

// ActivityLogPage is the paginated result returned by QueryActivityLog.
type ActivityLogPage struct {
	Entries    []ActivityLogEntry
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

// activityLogPageSize is the default number of rows rendered per page on the
// /activities view.  Exports ignore this.
const activityLogPageSize = 100

// activityLogMaxExport caps the number of rows an export can stream in a
// single request as a safety net.  The UI warns past this point.
const activityLogMaxExport = 100000

// ParseActivityLogFilters reads filter values from the request query string.
// Unrecognised or malformed values are silently ignored (treated as "no
// filter") so that bad bookmarks don't return 400s for an otherwise valid
// page load. from and to are calendar days, both inclusive.
func ParseActivityLogFilters(c *gin.Context) (ActivityLogFilters, error) {
	var f ActivityLogFilters

	if v := strings.TrimSpace(c.Query("plant_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			f.PlantID = id
		}
	}
	for _, raw := range c.QueryArray("activity_id") {
		// Also accept a comma-separated list in a single param for convenience.
		for _, tok := range strings.Split(raw, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(tok)); err == nil && id > 0 {
				f.TypeIDs = append(f.TypeIDs, id)
			}
		}
	}
	if v := strings.TrimSpace(c.Query("zone_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			f.ZoneID = id
		}
	}
	if d, err := chrono.ParseDate(strings.TrimSpace(c.Query("from"))); err == nil {
		f.From = &d
	}
	if d, err := chrono.ParseDate(strings.TrimSpace(c.Query("to"))); err == nil {
		f.To = &d
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		if err := utils.ValidateStringLength("q", q, utils.MaxNameLength); err != nil {
			return f, err
		}
		f.Query = q
	}
	switch c.Query("order") {
	case "date_asc":
		f.Order = service.OldestFirst
	case "plant":
		f.Order = service.ByPlant
	case "activity":
		f.Order = service.ByType
	}
	return f, nil
}

func toLogEntry(e service.ActivityEntry) ActivityLogEntry {
	out := ActivityLogEntry{
		ID: uint(e.ID), Date: e.At, Note: e.Note,
		ActivityID: e.Type.ID, ActivityName: e.Type.Name,
		PlantID: uint(e.Plant.ID), PlantName: e.Plant.Name,
	}
	if e.Zone != nil {
		out.ZoneName = e.Zone.Name
	}
	if e.Strain != nil {
		out.StrainName = e.Strain.Name
	}
	return out
}

// scanActivityLog returns up to limit rows (0 = all) from offset.
func scanActivityLog(ctx context.Context, svc *service.Services, f ActivityLogFilters, limit, offset int) ([]ActivityLogEntry, int, error) {
	f.Offset, f.Limit = offset, limit
	entries, total, err := svc.Activities.Log(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ActivityLogEntry, len(entries))
	for i, e := range entries {
		out[i] = toLogEntry(e)
	}
	return out, total, nil
}

// QueryActivityLog fetches a paginated page of activity log rows.  `page` is
// 1-indexed; pageSize<=0 falls back to activityLogPageSize.
func QueryActivityLog(ctx context.Context, svc *service.Services, f ActivityLogFilters, page, pageSize int) (ActivityLogPage, error) {
	if pageSize <= 0 {
		pageSize = activityLogPageSize
	}
	if page < 1 {
		page = 1
	}
	_, total, err := scanActivityLog(ctx, svc, f, 1, 0)
	if err != nil {
		return ActivityLogPage{}, err
	}
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	entries, total, err := scanActivityLog(ctx, svc, f, pageSize, (page-1)*pageSize)
	if err != nil {
		return ActivityLogPage{}, err
	}
	return ActivityLogPage{
		Entries:    entries,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// ListAllActivities is the public JSON endpoint backing programmatic access
// to the activity log.  Returns a page of entries matching the filters.
func ListAllActivities(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ListAllActivities")

	filters, err := ParseActivityLogFilters(c)
	if err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}

	page := 1
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	pageSize := activityLogPageSize
	if v := strings.TrimSpace(c.Query("page_size")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100000 {
			pageSize = n
		}
	}

	result, err := QueryActivityLog(c.Request.Context(), ServicesFromContext(c), filters, page, pageSize)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activity log")
		apiInternalError(c, "api_database_error")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries":     result.Entries,
		"total":       result.Total,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
	})
}

// formatActivityDate renders an export row's date in the instance zone,
// as the "formatDateTime" template helper does.
func formatActivityDate(t time.Time, zones *chrono.Zones) string {
	return t.In(zones.Instance()).Format(utils.LayoutDateTime)
}

// exportFilenameSuffix returns a short descriptor appended to export filenames
// so the user can distinguish single-plant downloads from full-instance ones.
func exportFilenameSuffix(ctx context.Context, svc *service.Services, f ActivityLogFilters) string {
	now := svc.TimeZones().Now(svc.Clock()).Format("20060102")
	if f.PlantID > 0 {
		if p, err := svc.Plants.Get(ctx, f.PlantID); err == nil && slugify(p.Name) != "" {
			return slugify(p.Name) + "-" + now
		}
		return fmt.Sprintf("plant-%d-%s", f.PlantID, now)
	}
	return "all-" + now
}

// slugify produces a lowercase, filesystem-safe slug from a human name.
func slugify(name string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-', r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "plant"
	}
	return out
}

// ExportActivitiesCSV streams a CSV of all activities matching the filters.
// Auth-gated via AddProtectedRoutes; unauthenticated users are redirected
// to /login by the AuthMiddleware.
func ExportActivitiesCSV(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ExportActivitiesCSV")

	filters, err := ParseActivityLogFilters(c)
	if err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}

	zones := ServicesFromContext(c).TimeZones()
	entries, _, err := scanActivityLog(c.Request.Context(), ServicesFromContext(c), filters, activityLogMaxExport, 0)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities for CSV export")
		apiInternalError(c, "api_database_error")
		return
	}

	filename := "isley-activities-" + exportFilenameSuffix(c.Request.Context(), ServicesFromContext(c), filters) + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	w := csv.NewWriter(c.Writer)
	// Header row
	if err := w.Write([]string{"Date", "Plant", "Strain", "Zone", "Activity", "Note"}); err != nil {
		fieldLogger.WithError(err).Error("Failed to write CSV header")
		return
	}
	for _, e := range entries {
		row := []string{
			formatActivityDate(e.Date, zones),
			e.PlantName,
			e.StrainName,
			e.ZoneName,
			e.ActivityName,
			e.Note,
		}
		if err := w.Write(row); err != nil {
			fieldLogger.WithError(err).Error("Failed to write CSV row")
			return
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		fieldLogger.WithError(err).Error("CSV writer flushed with error")
	}
	fieldLogger.WithFields(logrus.Fields{
		"rows":     len(entries),
		"filename": filename,
	}).Info("Activity log CSV exported")
}

// ExportActivitiesXLSX streams an xlsx workbook of all activities matching
// the filters.  Single "Activities" sheet, frozen header row, autofilter
// over the data range.  Auth-gated via AddProtectedRoutes.
func ExportActivitiesXLSX(c *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "ExportActivitiesXLSX")

	filters, err := ParseActivityLogFilters(c)
	if err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}

	zones := ServicesFromContext(c).TimeZones()
	entries, _, err := scanActivityLog(c.Request.Context(), ServicesFromContext(c), filters, activityLogMaxExport, 0)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities for XLSX export")
		apiInternalError(c, "api_database_error")
		return
	}

	f := excelize.NewFile()
	sheet := "Activities"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create xlsx sheet")
		apiInternalError(c, "api_database_error")
		return
	}
	// Delete the default Sheet1 that NewFile creates.
	if err := f.DeleteSheet("Sheet1"); err != nil {
		fieldLogger.WithError(err).Warn("Failed to delete default Sheet1 (continuing)")
	}
	f.SetActiveSheet(idx)

	headers := []string{"Date", "Plant", "Strain", "Zone", "Activity", "Note"}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to build header style (continuing without)")
	}
	wrapStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
		if headerStyle != 0 {
			_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
		}
	}

	// Column widths tuned for readability; Note gets a wide wrapped column.
	widths := map[string]float64{
		"A": 20, // Date
		"B": 24, // Plant
		"C": 22, // Strain
		"D": 16, // Zone
		"E": 16, // Activity
		"F": 60, // Note
	}
	for col, w := range widths {
		_ = f.SetColWidth(sheet, col, col, w)
	}

	for i, e := range entries {
		row := i + 2
		_ = f.SetCellValue(sheet, cellRef("A", row), formatActivityDate(e.Date, zones))
		_ = f.SetCellValue(sheet, cellRef("B", row), e.PlantName)
		_ = f.SetCellValue(sheet, cellRef("C", row), e.StrainName)
		_ = f.SetCellValue(sheet, cellRef("D", row), e.ZoneName)
		_ = f.SetCellValue(sheet, cellRef("E", row), e.ActivityName)
		_ = f.SetCellValue(sheet, cellRef("F", row), e.Note)
		if wrapStyle != 0 {
			_ = f.SetCellStyle(sheet, cellRef("F", row), cellRef("F", row), wrapStyle)
		}
	}

	// Freeze the header row.
	_ = f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// Autofilter over the data range (only if we actually wrote rows).
	if len(entries) > 0 {
		lastRow := len(entries) + 1
		rangeRef := fmt.Sprintf("A1:F%d", lastRow)
		_ = f.AutoFilter(sheet, rangeRef, nil)
	}

	filename := "isley-activities-" + exportFilenameSuffix(c.Request.Context(), ServicesFromContext(c), filters) + ".xlsx"
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := f.Write(c.Writer); err != nil {
		fieldLogger.WithError(err).Error("Failed to stream XLSX to response")
		return
	}
	fieldLogger.WithFields(logrus.Fields{
		"rows":     len(entries),
		"filename": filename,
	}).Info("Activity log XLSX exported")
}

// cellRef returns a cell reference like "A3" from ("A", 3).  excelize has
// CoordinatesToCellName but that requires integer columns; this wrapper is
// more ergonomic when columns are fixed labels.
func cellRef(col string, row int) string {
	return fmt.Sprintf("%s%d", col, row)
}
