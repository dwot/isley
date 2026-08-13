package handlers

// Smoke reports: post-harvest tasting notes attached to a plant.
//
// Reads follow the same visibility as the plant page that displays them:
// public when guest mode is on, behind AuthMiddleware otherwise (see
// routes.go's AddBasicRoutes vs AddProtectedApiRoutes). Writes are always
// session-gated. The write path is transactional because a report and its
// per-criterion scores are one logical record — a half-written report is
// never acceptable.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

// harvestedStatuses are the plant statuses that make a plant eligible for a
// smoke report. Dead is deliberately excluded: deriveHarvestDate treats it as
// terminal, which is right for a date calculation and absurd for a tasting.
var harvestedStatuses = []string{"Drying", "Curing", "Success"}

// harvestedStatusesInClause returns the "$2, $3, ..." placeholder list for
// harvestedStatuses — built from its length rather than hardcoded — together
// with the matching argument slice, meant to be appended after a leading $1
// plant-id parameter. Both of this function's call sites used to index
// harvestedStatuses[0..2] against a literal "IN ($2, $3, $4)": that compiles
// fine if a fourth status is ever added to the slice, and silently drops it
// from the query instead of failing loudly. Deriving the placeholder count
// from len(harvestedStatuses) keeps the two coupled.
func harvestedStatusesInClause() (string, []interface{}) {
	placeholders := make([]string, len(harvestedStatuses))
	args := make([]interface{}, len(harvestedStatuses))
	for i, s := range harvestedStatuses {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i] = s
	}
	return strings.Join(placeholders, ", "), args
}

// scoreInput is one criterion's score in a create/update request body.
type scoreInput struct {
	CriterionID int `json:"criterion_id"`
	Value       int `json:"value"`
}

// smokeReportInput is the create/update request body.
type smokeReportInput struct {
	ReportDate        string       `json:"report_date"`
	ConsumptionMethod string       `json:"consumption_method"`
	Rating            int          `json:"rating"`
	Description       string       `json:"description"`
	Scores            []scoreInput `json:"scores"`
}

// plantIsHarvested reports whether the plant has ever reached a status that
// makes a tasting note meaningful.
func plantIsHarvested(db *sql.DB, plantID int) (bool, error) {
	inClause, statusArgs := harvestedStatusesInClause()
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM plant_status_log psl
		JOIN plant_status ps ON ps.id = psl.status_id
		WHERE psl.plant_id = $1 AND ps.status IN (%s)`, inClause)

	var n int
	err := db.QueryRow(query, append([]interface{}{plantID}, statusArgs...)...).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SmokeRatingScale returns the selectable rating values in ascending order,
// derived from SmokeRatingMin/SmokeRatingMax so the scale is defined once.
// The smoke-report modals range over it to emit their star radio groups; a
// hardcoded 1..5 in two templates would silently drift if the bounds ever
// changed.
func SmokeRatingScale() []int {
	scale := make([]int, 0, SmokeRatingMax-SmokeRatingMin+1)
	for v := SmokeRatingMin; v <= SmokeRatingMax; v++ {
		scale = append(scale, v)
	}
	return scale
}

// PlantIsHarvestedForTemplate is the template-facing wrapper around
// plantIsHarvested: it swallows the error into false so the plant page still
// renders when the status lookup fails, minus the smoke-report card.
func PlantIsHarvestedForTemplate(db *sql.DB, plantID int) bool {
	harvested, err := plantIsHarvested(db, plantID)
	if err != nil {
		logger.Log.WithError(err).WithField("func", "PlantIsHarvestedForTemplate").
			Error("Failed to check plant harvest status")
		return false
	}
	return harvested
}

// plantHasSmokeReports reports whether the plant already has at least one
// smoke report on record, independent of whether it is currently harvested.
func plantHasSmokeReports(db *sql.DB, plantID int) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM smoke_report WHERE plant_id = $1`, plantID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// PlantHasSmokeReportsForTemplate is the template-facing wrapper around
// plantHasSmokeReports: it swallows the error into false so the plant page
// still renders when the lookup fails.
//
// Used alongside PlantIsHarvestedForTemplate to decide plant.html's
// smoke-report visibility. A plant's harvested status is not permanent: a
// mis-logged Curing entry can be deleted from the status-history modal,
// which flips PlantIsHarvestedForTemplate back to false even though the
// plant already has reports sitting in the database (still returned by
// GET /plant/:id/smoke-reports and still counted by GetStrainSmokeSummary).
// Gating the card purely on "harvested" would make those reports
// unreachable from the UI while they keep affecting the strain average — so
// the card and its modals render on harvested || hasReports, and only the
// "+ Add" trigger stays gated on harvested alone (see routes.go and
// plant.html), keeping the create path guarded while existing reports stay
// viewable, editable and deletable.
func PlantHasSmokeReportsForTemplate(db *sql.DB, plantID int) bool {
	has, err := plantHasSmokeReports(db, plantID)
	if err != nil {
		logger.Log.WithError(err).WithField("func", "PlantHasSmokeReportsForTemplate").
			Error("Failed to check plant smoke report existence")
		return false
	}
	return has
}

// harvestDateFor returns the plant's harvest date — the earliest date among
// its harvested statuses — or the zero time when it has none.
//
// This deliberately avoids MIN(psl.date): an aggregate has no column-level
// decltype, and modernc.org/sqlite gates its TEXT→time.Time conversion
// entirely on the decltype of the selected column. Aggregating there makes
// the driver hand back a bare Go string instead of a time.Time, which
// sql.NullTime.Scan rejects — a query that behaves differently on SQLite
// than on Postgres despite being the same string. Selecting the raw column
// and doing the MIN via ORDER BY/LIMIT keeps the decltype intact on both.
func harvestDateFor(db *sql.DB, plantID int) time.Time {
	fieldLogger := logger.Log.WithField("func", "harvestDateFor")

	inClause, statusArgs := harvestedStatusesInClause()
	query := fmt.Sprintf(`
		SELECT psl.date
		FROM plant_status_log psl
		JOIN plant_status ps ON ps.id = psl.status_id
		WHERE psl.plant_id = $1 AND ps.status IN (%s)
		ORDER BY psl.date ASC
		LIMIT 1`, inClause)

	var t sql.NullTime
	err := db.QueryRow(query, append([]interface{}{plantID}, statusArgs...)...).Scan(&t)
	if err != nil {
		// sql.ErrNoRows is the legitimate "never harvested" case, not a
		// failure — only log the rest.
		if !errors.Is(err, sql.ErrNoRows) {
			fieldLogger.WithError(err).Error("Failed to query plant harvest date")
		}
		return time.Time{}
	}
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

// cureDays returns the whole days between the harvest date and the report
// date, or nil when there is no harvest date or the report predates it. The
// guard compares the instants directly, before any arithmetic: truncating
// integer division on a negative sub-day duration (e.g. harvest at noon,
// report parsed as midnight the same day → -12h) rounds toward zero, so a
// `d < 0` check on the divided result would miss it and report a cure of 0
// days instead of "no measurement". A negative cure duration is never
// shown — it means a backdated report or a status date corrected after the
// fact, not a real measurement.
func cureDays(harvest, report time.Time) *int {
	if harvest.IsZero() || report.Before(harvest) {
		return nil
	}
	d := int(report.Sub(harvest).Hours() / 24)
	return &d
}

// GetSmokeReports returns a plant's reports, most recent first, each with its
// scores. Criterion names are joined in so archived criteria still render with
// a label.
func GetSmokeReports(db *sql.DB, plantID int) []types.SmokeReport {
	fieldLogger := logger.Log.WithField("func", "GetSmokeReports")

	rows, err := db.Query(`
		SELECT id, plant_id, report_date, consumption_method, rating, description
		FROM smoke_report
		WHERE plant_id = $1
		ORDER BY report_date DESC, id DESC`, plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query smoke reports")
		return nil
	}
	defer rows.Close()

	reports := []types.SmokeReport{}
	byID := map[int]int{} // report id → index in reports
	for rows.Next() {
		var r types.SmokeReport
		if err := rows.Scan(&r.ID, &r.PlantID, &r.ReportDate,
			&r.ConsumptionMethod, &r.Rating, &r.Description); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan smoke report row")
			return nil
		}
		r.Scores = []types.SmokeReportScore{}
		byID[r.ID] = len(reports)
		reports = append(reports, r)
	}
	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Failed to iterate smoke report rows")
		return nil
	}
	if len(reports) == 0 {
		return reports
	}

	// Computed before the score query so a score-query failure below cannot
	// also silently strip CureDays from an otherwise-successful read.
	harvest := harvestDateFor(db, plantID)
	for i := range reports {
		reports[i].CureDays = cureDays(harvest, reports[i].ReportDate)
	}

	scoreRows, err := db.Query(`
		SELECT s.report_id, s.criterion_id, c.name, s.value
		FROM smoke_report_score s
		JOIN smoke_report_criterion c ON c.id = s.criterion_id
		JOIN smoke_report r ON r.id = s.report_id
		WHERE r.plant_id = $1
		ORDER BY c.sort_order ASC, c.name ASC`, plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query smoke report scores")
		// A caller must not be able to mistake "the score query failed" for
		// "this report has no scores": the former is a transient failure
		// that, fed back through an edit form, would make writeScores
		// delete real scores. nil (distinct from an empty slice) signals
		// the failure up to GetSmokeReportsHandler, matching the row-scan
		// error path above.
		return nil
	}
	defer scoreRows.Close()

	for scoreRows.Next() {
		var reportID int
		var s types.SmokeReportScore
		if err := scoreRows.Scan(&reportID, &s.CriterionID, &s.Name, &s.Value); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan smoke report score row")
			return nil
		}
		if idx, ok := byID[reportID]; ok {
			reports[idx].Scores = append(reports[idx].Scores, s)
		}
	}
	if err := scoreRows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Failed to iterate smoke report score rows")
		return nil
	}

	return reports
}

// GetSmokeReportsHandler returns a plant's reports as JSON. Visibility
// follows the plant page it feeds: public under guest mode, behind
// AuthMiddleware otherwise (see routes.go).
func GetSmokeReportsHandler(c *gin.Context) {
	plantID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}
	reports := GetSmokeReports(DBFromContext(c), plantID)
	if reports == nil {
		reports = []types.SmokeReport{}
	}
	c.JSON(http.StatusOK, reports)
}

// errKeyCriteriaLookupFailed is a sentinel returned by
// validateSmokeReportInput when the active-criteria lookup itself failed (a
// DB error), as opposed to a submitted score legitimately failing
// validation. GetSmokeCriteria returns nil on a query error and a non-nil
// (possibly empty) slice when the query succeeds and simply found no active
// criteria — the two are distinguishable, and callers must not report the
// former as the caller's bad input.
const errKeyCriteriaLookupFailed = "internal:smoke_criteria_lookup_failed"

// validateSmokeReportInput normalises and checks a request body, returning a
// translation key on failure and the parsed report date on success. The key
// errKeyCriteriaLookupFailed is a special case: it means validation itself
// could not run, not that the input failed it, and callers must map it to a
// 5xx rather than treating it like any other key with apiBadRequest.
func validateSmokeReportInput(db *sql.DB, in *smokeReportInput) (time.Time, string) {
	in.ConsumptionMethod = strings.TrimSpace(in.ConsumptionMethod)
	in.Description = strings.TrimSpace(in.Description)

	if err := utils.ValidateDate("report_date", in.ReportDate); err != nil {
		return time.Time{}, "api_invalid_smoke_report_date"
	}
	reportDate, err := utils.ParseFlexibleDate(in.ReportDate)
	if err != nil {
		return time.Time{}, "api_invalid_smoke_report_date"
	}

	if in.Rating < SmokeRatingMin || in.Rating > SmokeRatingMax {
		return time.Time{}, "api_invalid_smoke_rating"
	}
	if err := utils.ValidateStringLength("consumption_method", in.ConsumptionMethod, MaxConsumptionMethodLength); err != nil {
		return time.Time{}, "api_invalid_smoke_consumption_method"
	}
	if err := utils.ValidateStringLength("description", in.Description, utils.MaxNotesLength); err != nil {
		return time.Time{}, "api_invalid_smoke_description"
	}

	// Every submitted score must target an active criterion and sit in range.
	// Scores are optional one by one — an empty list is a valid report.
	criteria := GetSmokeCriteria(db, true)
	if criteria == nil && len(in.Scores) > 0 {
		// The lookup query failed (not "no active criteria" — that returns a
		// non-nil empty slice). Without this check every submitted score
		// below would fail the active[...] test and be reported as a 400
		// api_invalid_smoke_criterion_id, which is wrong: the client did
		// nothing bad, the database call did.
		return time.Time{}, errKeyCriteriaLookupFailed
	}
	active := map[int]bool{}
	for _, cr := range criteria {
		active[cr.ID] = true
	}
	seen := map[int]bool{}
	for _, s := range in.Scores {
		if !active[s.CriterionID] {
			return time.Time{}, "api_invalid_smoke_criterion_id"
		}
		if seen[s.CriterionID] {
			return time.Time{}, "api_duplicate_smoke_score"
		}
		seen[s.CriterionID] = true
		if s.Value < SmokeRatingMin || s.Value > SmokeRatingMax {
			return time.Time{}, "api_invalid_smoke_rating"
		}
	}

	return reportDate, ""
}

// writeScores replaces a report's scores inside an open transaction.
//
// The delete is deliberately scoped to active criteria only. An edit form
// (see smoke-report-edit-modal.html) renders a <select> for active criteria
// alone, so an archived axis never appears in the submitted payload — not
// because the client chose to omit it, but because it structurally cannot
// resubmit it (validateSmokeReportInput checks the same active-only set). An
// unscoped "DELETE ... WHERE report_id = $1" would therefore erase every
// archived criterion's score on every edit, even though nothing about the
// request expressed an intent to change it. The design rule is that an
// archived criterion's existing score is immutable, not absent — reads
// already honour this (GetSmokeReports' criterion-name JOIN is not filtered
// on active), and this scoped delete is what makes the write side honour it
// too. Do not "simplify" this back to an unscoped DELETE.
func writeScores(tx *sql.Tx, reportID int, scores []scoreInput) error {
	if _, err := tx.Exec(`
		DELETE FROM smoke_report_score
		WHERE report_id = $1
		  AND criterion_id IN (SELECT id FROM smoke_report_criterion WHERE active = 1)`,
		reportID); err != nil {
		return err
	}
	for _, s := range scores {
		if _, err := tx.Exec(
			`INSERT INTO smoke_report_score (report_id, criterion_id, value) VALUES ($1, $2, $3)`,
			reportID, s.CriterionID, s.Value); err != nil {
			return err
		}
	}
	return nil
}

// AddSmokeReportHandler creates a report for a harvested plant.
func AddSmokeReportHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddSmokeReportHandler")

	plantID, err := strconv.Atoi(c.Param("plantID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_plant_id")
		return
	}

	var in smokeReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	db := DBFromContext(c)

	harvested, err := plantIsHarvested(db, plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to check plant harvest status")
		apiInternalError(c, "api_failed_to_add_smoke_report")
		return
	}
	if !harvested {
		apiBadRequest(c, "api_plant_not_harvested")
		return
	}

	reportDate, errKey := validateSmokeReportInput(db, &in)
	if errKey == errKeyCriteriaLookupFailed {
		fieldLogger.Error("Failed to load active smoke criteria during validation")
		apiInternalError(c, "api_failed_to_add_smoke_report")
		return
	}
	if errKey != "" {
		apiBadRequest(c, errKey)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start smoke report transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	var reportID int
	err = tx.QueryRow(`
		INSERT INTO smoke_report (plant_id, report_date, consumption_method, rating, description)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		plantID, reportDate, in.ConsumptionMethod, in.Rating, in.Description).Scan(&reportID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert smoke report")
		apiInternalError(c, "api_failed_to_add_smoke_report")
		return
	}

	if err := writeScores(tx, reportID, in.Scores); err != nil {
		fieldLogger.WithError(err).Error("Failed to insert smoke report scores")
		apiInternalError(c, "api_failed_to_add_smoke_report")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit smoke report transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": reportID})
}

// UpdateSmokeReportHandler rewrites a report and its scores.
func UpdateSmokeReportHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateSmokeReportHandler")

	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_report_id")
		return
	}

	var in smokeReportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	db := DBFromContext(c)

	reportDate, errKey := validateSmokeReportInput(db, &in)
	if errKey == errKeyCriteriaLookupFailed {
		fieldLogger.Error("Failed to load active smoke criteria during validation")
		apiInternalError(c, "api_failed_to_update_smoke_report")
		return
	}
	if errKey != "" {
		apiBadRequest(c, errKey)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start smoke report transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		UPDATE smoke_report
		SET report_date = $1, consumption_method = $2, rating = $3,
		    description = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5`,
		reportDate, in.ConsumptionMethod, in.Rating, in.Description, reportID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update smoke report")
		apiInternalError(c, "api_failed_to_update_smoke_report")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read rows affected updating smoke report")
		apiInternalError(c, "api_failed_to_update_smoke_report")
		return
	}
	if n == 0 {
		apiNotFound(c, "api_smoke_report_not_found")
		return
	}

	if err := writeScores(tx, reportID, in.Scores); err != nil {
		fieldLogger.WithError(err).Error("Failed to rewrite smoke report scores")
		apiInternalError(c, "api_failed_to_update_smoke_report")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit smoke report transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	apiOK(c, "api_smoke_report_updated")
}

// DeleteSmokeReportHandler removes a report; its scores are deleted
// explicitly first (see the comment below on why this doesn't rely on the
// FK cascade).
func DeleteSmokeReportHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteSmokeReportHandler")

	reportID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_report_id")
		return
	}

	db := DBFromContext(c)

	// Scores are removed explicitly rather than relying on the cascade:
	// SQLite only enforces foreign keys when PRAGMA foreign_keys is on, and
	// the app does not depend on that pragma anywhere else.
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start smoke report delete transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM smoke_report_score WHERE report_id = $1`, reportID); err != nil {
		fieldLogger.WithError(err).Error("Failed to delete smoke report scores")
		apiInternalError(c, "api_failed_to_delete_smoke_report")
		return
	}

	res, err := tx.Exec(`DELETE FROM smoke_report WHERE id = $1`, reportID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete smoke report")
		apiInternalError(c, "api_failed_to_delete_smoke_report")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read rows affected deleting smoke report")
		apiInternalError(c, "api_failed_to_delete_smoke_report")
		return
	}
	if n == 0 {
		apiNotFound(c, "api_smoke_report_not_found")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit smoke report delete transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	apiOK(c, "api_smoke_report_deleted")
}

// GetStrainSmokeSummary aggregates a strain's tasting notes for the strain
// page card.
//
// One entry per harvested plant, carrying that plant's most recent report.
// Averaging the entries rather than every report means a plant tasted ten
// times does not outweigh one tasted once, and — the point of the card — the
// headline figure is always the mean of the rows shown underneath it, so the
// reader can check the arithmetic by eye.
func GetStrainSmokeSummary(db *sql.DB, strainID int) types.SmokeReportSummary {
	fieldLogger := logger.Log.WithField("func", "GetStrainSmokeSummary")

	summary := types.SmokeReportSummary{Entries: []types.SmokeReportSummaryRow{}}

	rows, err := db.Query(`
		SELECT p.id, p.name, sr.rating, sr.report_date
		FROM plant p
		JOIN smoke_report sr ON sr.id = (
			SELECT inner_sr.id
			FROM smoke_report inner_sr
			WHERE inner_sr.plant_id = p.id
			ORDER BY inner_sr.report_date DESC, inner_sr.id DESC
			LIMIT 1
		)
		WHERE p.strain_id = $1
		ORDER BY sr.report_date DESC, p.id DESC`, strainID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query strain smoke summary")
		return summary
	}
	defer rows.Close()

	total := 0
	for rows.Next() {
		var e types.SmokeReportSummaryRow
		if err := rows.Scan(&e.PlantID, &e.PlantName, &e.Rating, &e.ReportDate); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan strain smoke summary row")
			return types.SmokeReportSummary{Entries: []types.SmokeReportSummaryRow{}}
		}
		total += e.Rating
		summary.Entries = append(summary.Entries, e)
	}
	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Failed to iterate strain smoke summary rows")
		return types.SmokeReportSummary{Entries: []types.SmokeReportSummaryRow{}}
	}

	summary.Count = len(summary.Entries)
	if summary.Count > 0 {
		summary.Average = float64(total) / float64(summary.Count)
	}
	return summary
}

// GetStrainSmokeSummaryHandler serves the strain page's tasting card.
func GetStrainSmokeSummaryHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	c.JSON(http.StatusOK, GetStrainSmokeSummary(DBFromContext(c), strainID))
}
