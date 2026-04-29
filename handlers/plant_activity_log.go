package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"

	"isley/logger"
	model "isley/model"
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

// ActivityLogFilters captures all optional filter params the activity log
// endpoints accept.  All fields default to "no filter".
type ActivityLogFilters struct {
	PlantID     *int
	ActivityIDs []int
	ZoneID      *int
	From        *time.Time
	To          *time.Time
	Query       string
	Order       string
}

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
// page load.
func ParseActivityLogFilters(c *gin.Context) (ActivityLogFilters, error) {
	var f ActivityLogFilters

	if v := strings.TrimSpace(c.Query("plant_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			f.PlantID = &id
		}
	}

	for _, raw := range c.QueryArray("activity_id") {
		// Also accept a comma-separated list in a single param for convenience.
		for _, tok := range strings.Split(raw, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			if id, err := strconv.Atoi(tok); err == nil && id > 0 {
				f.ActivityIDs = append(f.ActivityIDs, id)
			}
		}
	}

	if v := strings.TrimSpace(c.Query("zone_id")); v != "" {
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			f.ZoneID = &id
		}
	}

	loc := appTimeLocation(ConfigStoreFromContext(c).Timezone())
	if v := strings.TrimSpace(c.Query("from")); v != "" {
		if t, err := time.ParseInLocation(utils.LayoutDate, v, loc); err == nil {
			f.From = &t
		}
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		if t, err := time.ParseInLocation(utils.LayoutDate, v, loc); err == nil {
			// inclusive end-of-day
			end := t.Add(24*time.Hour - time.Second)
			f.To = &end
		}
	}

	if q := strings.TrimSpace(c.Query("q")); q != "" {
		if err := utils.ValidateStringLength("q", q, utils.MaxNameLength); err != nil {
			return f, err
		}
		f.Query = q
	}

	switch c.Query("order") {
	case "date_asc", "plant", "activity":
		f.Order = c.Query("order")
	default:
		f.Order = "date_desc"
	}

	return f, nil
}

// buildActivityLogQuery assembles the SQL and argument slice for the given
// filters.  Uses `$N` placeholders, which both the SQLite and PostgreSQL
// drivers accept.  Pagination is applied by the caller.
func buildActivityLogQuery(f ActivityLogFilters) (string, []interface{}) {
	var where []string
	var args []interface{}
	idx := 0
	next := func() string { idx++; return fmt.Sprintf("$%d", idx) }

	base := `
SELECT pa.id, pa.date, pa.note,
       a.id AS activity_id, COALESCE(a.name, '') AS activity_name,
       p.id AS plant_id, COALESCE(p.name, '') AS plant_name,
       COALESCE(z.name, '') AS zone_name,
       COALESCE(s.name, '') AS strain_name
FROM plant_activity pa
LEFT JOIN activity a ON a.id = pa.activity_id
LEFT JOIN plant    p ON p.id = pa.plant_id
LEFT JOIN zones    z ON z.id = p.zone_id
LEFT JOIN strain   s ON s.id = p.strain_id`

	if f.PlantID != nil {
		where = append(where, "pa.plant_id = "+next())
		args = append(args, *f.PlantID)
	}
	if len(f.ActivityIDs) > 0 {
		phs := make([]string, len(f.ActivityIDs))
		for i, id := range f.ActivityIDs {
			phs[i] = next()
			args = append(args, id)
		}
		where = append(where, "pa.activity_id IN ("+strings.Join(phs, ",")+")")
	}
	if f.ZoneID != nil {
		where = append(where, "p.zone_id = "+next())
		args = append(args, *f.ZoneID)
	}
	if f.From != nil {
		where = append(where, "pa.date >= "+next())
		args = append(args, *f.From)
	}
	if f.To != nil {
		where = append(where, "pa.date <= "+next())
		args = append(args, *f.To)
	}
	if f.Query != "" {
		op := "LIKE"
		if model.IsPostgres() {
			op = "ILIKE"
		}
		where = append(where, "pa.note "+op+" "+next())
		args = append(args, "%"+f.Query+"%")
	}

	q := base
	if len(where) > 0 {
		q += "\nWHERE " + strings.Join(where, " AND ")
	}
	switch f.Order {
	case "date_asc":
		q += "\nORDER BY pa.date ASC, pa.id ASC"
	case "plant":
		q += "\nORDER BY p.name ASC NULLS LAST, pa.date DESC"
		if !model.IsPostgres() {
			// SQLite doesn't support NULLS LAST syntax; rely on default ordering.
			q = strings.Replace(q, "NULLS LAST", "", 1)
		}
	case "activity":
		q += "\nORDER BY a.name ASC, pa.date DESC"
	default:
		q += "\nORDER BY pa.date DESC, pa.id DESC"
	}
	return q, args
}

// countActivityLog returns the total number of rows matching the filters
// (ignores pagination).  Used for page count calculation.
func countActivityLog(db *sql.DB, f ActivityLogFilters) (int, error) {
	// Re-run the builder but swap the SELECT list for COUNT(*).
	fullQuery, args := buildActivityLogQuery(f)
	// Keep everything up to (but not including) the ORDER BY clause, then
	// wrap the WHERE portion in a COUNT query.
	if i := strings.Index(fullQuery, "\nORDER BY"); i >= 0 {
		fullQuery = fullQuery[:i]
	}
	// Replace the original SELECT list with COUNT(*).  The base query always
	// begins with "\nSELECT ... \nFROM"; swap SELECT...FROM for SELECT COUNT(*) FROM.
	fromIdx := strings.Index(fullQuery, "\nFROM ")
	if fromIdx < 0 {
		return 0, fmt.Errorf("buildActivityLogQuery returned malformed SQL")
	}
	countQuery := "SELECT COUNT(*) " + fullQuery[fromIdx:]
	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// scanActivityLog runs the filter query with optional LIMIT/OFFSET and
// returns scanned rows.  Passing limit<=0 returns all rows.
func scanActivityLog(db *sql.DB, f ActivityLogFilters, limit, offset int) ([]ActivityLogEntry, error) {
	query, args := buildActivityLogQuery(f)
	if limit > 0 {
		query += fmt.Sprintf("\nLIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ActivityLogEntry
	for rows.Next() {
		var e ActivityLogEntry
		if err := rows.Scan(
			&e.ID, &e.Date, &e.Note,
			&e.ActivityID, &e.ActivityName,
			&e.PlantID, &e.PlantName,
			&e.ZoneName, &e.StrainName,
		); err != nil {
			return nil, err
		}
		e.Date = utils.AsLocal(e.Date)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// QueryActivityLog fetches a paginated page of activity log rows.  `page` is
// 1-indexed; pageSize<=0 falls back to activityLogPageSize.
func QueryActivityLog(db *sql.DB, f ActivityLogFilters, page, pageSize int) (ActivityLogPage, error) {
	if pageSize <= 0 {
		pageSize = activityLogPageSize
	}
	if page < 1 {
		page = 1
	}

	total, err := countActivityLog(db, f)
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

	entries, err := scanActivityLog(db, f, pageSize, (page-1)*pageSize)
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

	db := DBFromContext(c)
	result, err := QueryActivityLog(db, filters, page, pageSize)
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

// appTimeLocation returns the supplied timezone identifier resolved to
// a *time.Location, falling back to time.Local when the input is empty
// or unparseable. Mirrors the defensive pattern used by the
// "formatDateTime" template helper.
func appTimeLocation(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// formatActivityDate renders the export-side human-readable date for a
// row in the supplied timezone, matching the "formatDateTime" template
// helper. Pass the empty string to render in time.Local.
func formatActivityDate(t time.Time, tz string) string {
	return t.In(appTimeLocation(tz)).Format(utils.LayoutDateTime)
}

// exportFilenameSuffix returns a short descriptor appended to export filenames
// so the user can distinguish single-plant downloads from full-instance ones.
func exportFilenameSuffix(db *sql.DB, f ActivityLogFilters) string {
	now := time.Now().Format("20060102")
	if f.PlantID != nil {
		var name string
		_ = db.QueryRow("SELECT name FROM plant WHERE id = $1", *f.PlantID).Scan(&name)
		if name != "" {
			return slugify(name) + "-" + now
		}
		return fmt.Sprintf("plant-%d-%s", *f.PlantID, now)
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

	db := DBFromContext(c)
	tz := ConfigStoreFromContext(c).Timezone()
	entries, err := scanActivityLog(db, filters, activityLogMaxExport, 0)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities for CSV export")
		apiInternalError(c, "api_database_error")
		return
	}

	filename := "isley-activities-" + exportFilenameSuffix(db, filters) + ".csv"
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
			formatActivityDate(e.Date, tz),
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

	db := DBFromContext(c)
	tz := ConfigStoreFromContext(c).Timezone()
	entries, err := scanActivityLog(db, filters, activityLogMaxExport, 0)
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
		_ = f.SetCellValue(sheet, cellRef("A", row), formatActivityDate(e.Date, tz))
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

	filename := "isley-activities-" + exportFilenameSuffix(db, filters) + ".xlsx"
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

// ActivityFilterPlant is the slim {id, name} record used to populate the
// plant dropdown on the /activities filter bar.  Keeping this distinct from
// the full PlantListResponse avoids pulling sensor/status data we don't need.
type ActivityFilterPlant struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ActivityFilterPlants returns every plant that has at least one activity
// row, sorted by name.  Includes harvested and dead plants so users can
// retroactively review history.  On error, returns nil (not an error) so the
// caller can render the page with an empty filter dropdown.
func ActivityFilterPlants(db *sql.DB) []ActivityFilterPlant {
	fieldLogger := logger.Log.WithField("func", "ActivityFilterPlants")
	rows, err := db.Query(`
SELECT DISTINCT p.id, p.name
FROM plant p
INNER JOIN plant_activity pa ON pa.plant_id = p.id
ORDER BY p.name`)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query filter plants")
		return nil
	}
	defer rows.Close()

	var out []ActivityFilterPlant
	for rows.Next() {
		var p ActivityFilterPlant
		if err := rows.Scan(&p.ID, &p.Name); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan filter plant")
			continue
		}
		out = append(out, p)
	}
	return out
}

// ActivityLogPageContext is the view data passed to views/activities.html.
// Exposed for both the normal server-rendered page and testing.
type ActivityLogPageContext struct {
	Filters     ActivityLogFilters
	Result      ActivityLogPage
	Plants      []ActivityFilterPlant
	QueryNoPage string
	// FromStr / ToStr / QStr / OrderStr / PlantIDStr / ZoneIDStr are the raw
	// query-string values re-surfaced for template form inputs, so the UI
	// round-trips the user's selections without Go type conversions in
	// templates.
	FromStr       string
	ToStr         string
	QStr          string
	OrderStr      string
	PlantIDStr    string
	ZoneIDStr     string
	ActivityIDSet map[int]bool // activity IDs currently selected, for <option selected>
}

// BuildActivityLogPageContext assembles the template context for a single
// /activities render, given the parsed filters and the current page.
func BuildActivityLogPageContext(db *sql.DB, filters ActivityLogFilters, page int, rawQuery map[string][]string) (ActivityLogPageContext, error) {
	result, err := QueryActivityLog(db, filters, page, activityLogPageSize)
	if err != nil {
		return ActivityLogPageContext{}, err
	}

	ctx := ActivityLogPageContext{
		Filters:       filters,
		Result:        result,
		Plants:        ActivityFilterPlants(db),
		ActivityIDSet: make(map[int]bool),
	}
	for _, id := range filters.ActivityIDs {
		ctx.ActivityIDSet[id] = true
	}
	if v, ok := rawQuery["from"]; ok && len(v) > 0 {
		ctx.FromStr = v[0]
	}
	if v, ok := rawQuery["to"]; ok && len(v) > 0 {
		ctx.ToStr = v[0]
	}
	if v, ok := rawQuery["q"]; ok && len(v) > 0 {
		ctx.QStr = v[0]
	}
	if v, ok := rawQuery["order"]; ok && len(v) > 0 {
		ctx.OrderStr = v[0]
	}
	if v, ok := rawQuery["plant_id"]; ok && len(v) > 0 {
		ctx.PlantIDStr = v[0]
	}
	if v, ok := rawQuery["zone_id"]; ok && len(v) > 0 {
		ctx.ZoneIDStr = v[0]
	}

	// Rebuild the query string without page= so the template can append
	// &page=N for pagination links without duplicating every filter.
	ctx.QueryNoPage = rebuildQuery(rawQuery, "page")
	return ctx, nil
}

// rebuildQuery re-encodes a query string from the raw gin map, skipping any
// keys listed in `exclude`.  Values are URL-escaped.  Returns the string
// WITHOUT a leading "?" — callers prepend "?" or "&" as appropriate.
func rebuildQuery(raw map[string][]string, exclude ...string) string {
	skip := make(map[string]bool)
	for _, k := range exclude {
		skip[k] = true
	}
	v := url.Values{}
	for k, vs := range raw {
		if skip[k] {
			continue
		}
		for _, val := range vs {
			if val == "" {
				continue
			}
			v.Add(k, val)
		}
	}
	return v.Encode()
}
