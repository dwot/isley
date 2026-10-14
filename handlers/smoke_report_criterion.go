package handlers

// CRUD for smoke-report tasting criteria, kept out of handlers/settings.go
// (already the largest file in the tree at ~1390 lines). Mirrors the metric
// CRUD there: mutate, then refresh the per-engine config.Store so the next
// request sees the change without a restart.

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/model/types"
	"isley/utils"
)

// GetSmokeCriteria returns the tasting criteria ordered by sort_order then
// name. When activeOnly is true, archived criteria are excluded — that is the
// list the entry form renders. Reads pass false so the Settings table and
// historical reports can still label archived criteria.
func GetSmokeCriteria(db *sql.DB, activeOnly bool) []types.SmokeReportCriterion {
	fieldLogger := logger.Log.WithField("func", "GetSmokeCriteria")

	query := `SELECT id, name, sort_order, active FROM smoke_report_criterion`
	if activeOnly {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY sort_order ASC, name ASC`

	rows, err := db.Query(query)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query smoke criteria")
		return nil
	}
	defer rows.Close()

	criteria := []types.SmokeReportCriterion{}
	for rows.Next() {
		var c types.SmokeReportCriterion
		var active int
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &active); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan smoke criterion row")
			return nil
		}
		c.Active = active == 1
		criteria = append(criteria, c)
	}
	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Failed to iterate smoke criterion rows")
		return nil
	}
	return criteria
}

// GetSmokeCriteriaHandler returns every criterion, archived ones included, so
// the Settings table can grey them out rather than hide them.
func GetSmokeCriteriaHandler(c *gin.Context) {
	criteria := GetSmokeCriteria(DBFromContext(c), false)
	if criteria == nil {
		criteria = []types.SmokeReportCriterion{} // return empty array, not null
	}
	c.JSON(http.StatusOK, criteria)
}

// AddSmokeCriterionHandler creates a criterion.
func AddSmokeCriterionHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddSmokeCriterionHandler")

	var req struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if err := utils.ValidateRequiredString("name", req.Name, MaxSmokeCriterionNameLength); err != nil {
		apiBadRequest(c, "api_invalid_smoke_criterion_name")
		return
	}

	db := DBFromContext(c)

	var id int
	err := db.QueryRow(
		`INSERT INTO smoke_report_criterion (name, sort_order, active)
		 VALUES ($1, $2, 1) RETURNING id`,
		req.Name, req.SortOrder).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add smoke criterion")
		apiInternalError(c, "api_failed_to_add_smoke_criterion")
		return
	}

	ConfigStoreFromContext(c).SetSmokeCriteria(GetSmokeCriteria(db, false))
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// UpdateSmokeCriterionHandler renames a criterion, reorders it, or flips its
// archived state. Renaming is non-destructive: historical scores keep pointing
// at the same row and pick up the new label.
func UpdateSmokeCriterionHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateSmokeCriterionHandler")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_criterion_id")
		return
	}

	var req struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
		Active    bool   `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if err := utils.ValidateRequiredString("name", req.Name, MaxSmokeCriterionNameLength); err != nil {
		apiBadRequest(c, "api_invalid_smoke_criterion_name")
		return
	}

	active := 0
	if req.Active {
		active = 1
	}

	db := DBFromContext(c)
	res, err := db.Exec(
		`UPDATE smoke_report_criterion SET name = $1, sort_order = $2, active = $3 WHERE id = $4`,
		req.Name, req.SortOrder, active, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update smoke criterion")
		apiInternalError(c, "api_failed_to_update_smoke_criterion")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		apiNotFound(c, "api_smoke_criterion_not_found")
		return
	}

	ConfigStoreFromContext(c).SetSmokeCriteria(GetSmokeCriteria(db, false))
	apiOK(c, "api_smoke_criterion_updated")
}

// DeleteSmokeCriterionHandler archives a criterion that reports already
// reference, and hard-deletes one that has never been scored.
//
// This deliberately departs from DeleteMetricHandler, which cascades into
// plant_measurements and destroys the history. A plant height can be measured
// again; a tasting impression cannot. The response reports which branch ran so
// the UI can word its confirmation honestly.
func DeleteSmokeCriterionHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteSmokeCriterionHandler")

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_criterion_id")
		return
	}

	db := DBFromContext(c)

	// Delete only if still unscored. Doing the check inside the DELETE closes
	// the window where a concurrent score insert would otherwise be destroyed
	// by the ON DELETE CASCADE. A plain transaction is not enough here: under
	// PostgreSQL's default READ COMMITTED, a preceding SELECT COUNT does not
	// block a concurrent insert.
	res, err := db.Exec(`
		DELETE FROM smoke_report_criterion
		WHERE id = $1
		  AND NOT EXISTS (SELECT 1 FROM smoke_report_score WHERE criterion_id = $1)`, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete smoke criterion")
		apiInternalError(c, "api_failed_to_delete_smoke_criterion")
		return
	}
	deleted, _ := res.RowsAffected()

	archived := false
	if deleted == 0 {
		// Either the criterion is scored, or it does not exist. Archiving
		// tells the two apart: it affects a row only in the first case.
		res, err = db.Exec(`UPDATE smoke_report_criterion SET active = 0 WHERE id = $1`, id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to archive smoke criterion")
			apiInternalError(c, "api_failed_to_delete_smoke_criterion")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			apiNotFound(c, "api_smoke_criterion_not_found")
			return
		}
		archived = true
	}

	ConfigStoreFromContext(c).SetSmokeCriteria(GetSmokeCriteria(db, false))
	c.JSON(http.StatusOK, gin.H{
		"archived": archived,
		"message":  T(c, "api_smoke_criterion_deleted"),
	})
}
