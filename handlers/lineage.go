package handlers

import (
	"database/sql"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetLineage returns direct parents for a strain
func GetLineage(db *sql.DB, strainID int) []types.StrainLineage {
	fieldLogger := logger.Log.WithField("func", "GetLineage")

	rows, err := db.Query(`
		SELECT sl.id, sl.strain_id, sl.parent_name, sl.parent_strain_id
		FROM strain_lineage sl
		WHERE sl.strain_id = $1
		ORDER BY sl.parent_name ASC`, strainID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query lineage")
		return nil
	}
	defer rows.Close()

	var lineage []types.StrainLineage
	for rows.Next() {
		var l types.StrainLineage
		err = rows.Scan(&l.ID, &l.StrainID, &l.ParentName, &l.ParentStrainID)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan lineage row")
			return nil
		}
		lineage = append(lineage, l)
	}
	return lineage
}

// GetAncestryTree recursively builds the full ancestry tree for a strain
func GetAncestryTree(db *sql.DB, strainID int, depth int) []types.StrainLineage {
	if depth > MaxAncestryDepth {
		return nil // safety limit to prevent infinite recursion
	}

	parents := GetLineage(db, strainID)
	if parents == nil {
		return nil
	}

	for i := range parents {
		if parents[i].ParentStrainID != nil {
			parents[i].Children = GetAncestryTree(db, *parents[i].ParentStrainID, depth+1)
		}
	}
	return parents
}

// GetLineageHandler returns the lineage JSON for a strain
func GetLineageHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	db := DBFromContext(c)
	tree := GetAncestryTree(db, strainID, 0)
	if tree == nil {
		tree = []types.StrainLineage{} // return empty array, not null
	}
	c.JSON(http.StatusOK, tree)
}

// AddLineageHandler adds a parent to a strain's lineage
func AddLineageHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddLineageHandler")

	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	var req struct {
		ParentName     string `json:"parent_name"`
		ParentStrainID *int   `json:"parent_strain_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	if err := utils.ValidateRequiredString("parent_name", req.ParentName, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	db := DBFromContext(c)

	var id int
	err = db.QueryRow(`
		INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id)
		VALUES ($1, $2, $3) RETURNING id`,
		strainID, req.ParentName, req.ParentStrainID).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add lineage entry")
		apiInternalError(c, "api_failed_to_add_lineage")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// DeleteLineageHandler removes a lineage entry
func DeleteLineageHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteLineageHandler")

	lineageID, err := strconv.Atoi(c.Param("lineageID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_lineage_id")
		return
	}

	db := DBFromContext(c)

	result, err := db.Exec(`DELETE FROM strain_lineage WHERE id = $1`, lineageID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete lineage entry")
		apiInternalError(c, "api_failed_to_delete_lineage")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		apiNotFound(c, "api_lineage_not_found")
		return
	}

	apiOK(c, "api_lineage_deleted")
}

// UpdateLineageHandler updates a lineage entry (e.g., link/unlink parent strain)
func UpdateLineageHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateLineageHandler")

	lineageID, err := strconv.Atoi(c.Param("lineageID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_lineage_id")
		return
	}

	var req struct {
		ParentName     string `json:"parent_name"`
		ParentStrainID *int   `json:"parent_strain_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	if err := utils.ValidateRequiredString("parent_name", req.ParentName, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	db := DBFromContext(c)

	_, err = db.Exec(`
		UPDATE strain_lineage
		SET parent_name = $1, parent_strain_id = $2
		WHERE id = $3`,
		req.ParentName, req.ParentStrainID, lineageID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update lineage entry")
		apiInternalError(c, "api_failed_to_update_lineage")
		return
	}

	apiOK(c, "api_lineage_updated")
}

// SetLineageHandler replaces all lineage entries for a strain (bulk operation)
func SetLineageHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SetLineageHandler")

	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	var req struct {
		Parents []struct {
			ParentName     string `json:"parent_name"`
			ParentStrainID *int   `json:"parent_strain_id"`
		} `json:"parents"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}

	db := DBFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		apiInternalError(c, "api_internal_error")
		return
	}

	// Delete existing lineage
	_, err = tx.Exec(`DELETE FROM strain_lineage WHERE strain_id = $1`, strainID)
	if err != nil {
		tx.Rollback()
		fieldLogger.WithError(err).Error("Failed to clear existing lineage")
		apiInternalError(c, "api_failed_to_update_lineage")
		return
	}

	// Insert new entries
	for _, p := range req.Parents {
		if err := utils.ValidateRequiredString("parent_name", p.ParentName, utils.MaxNameLength); err != nil {
			continue // skip entries with empty or invalid names
		}
		_, err = tx.Exec(`
			INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id)
			VALUES ($1, $2, $3)`,
			strainID, p.ParentName, p.ParentStrainID)
		if err != nil {
			tx.Rollback()
			fieldLogger.WithError(err).Error("Failed to insert lineage entry")
			apiInternalError(c, "api_failed_to_add_lineage")
			return
		}
	}

	if err = tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit lineage transaction")
		apiInternalError(c, "api_internal_error")
		return
	}

	apiOK(c, "api_lineage_updated")
}

// GetDescendants returns strains that have the given strain as a parent,
// including the other parent names and breeder for each descendant.
func GetDescendants(db *sql.DB, strainID int) []gin.H {
	fieldLogger := logger.Log.WithField("func", "GetDescendants")

	// string_agg is PostgreSQL-only; SQLite uses GROUP_CONCAT.
	aggExpr := "GROUP_CONCAT(sl2.parent_name, ', ')"
	if model.IsPostgres() {
		aggExpr = "string_agg(sl2.parent_name, ', ' ORDER BY sl2.parent_name)"
	}

	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.name, b.name as breeder,
			COALESCE((
				SELECT `+aggExpr+`
				FROM strain_lineage sl2
				WHERE sl2.strain_id = s.id
				AND (sl2.parent_strain_id IS NULL OR sl2.parent_strain_id != $1)
			), '') as other_parents
		FROM strain_lineage sl
		JOIN strain s ON sl.strain_id = s.id
		JOIN breeder b ON s.breeder_id = b.id
		WHERE sl.parent_strain_id = $1
		ORDER BY s.name ASC`, strainID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query descendants")
		return nil
	}
	defer rows.Close()

	var results []gin.H
	for rows.Next() {
		var id int
		var name, breeder, otherParents string
		if err := rows.Scan(&id, &name, &breeder, &otherParents); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan descendant row")
			continue
		}
		results = append(results, gin.H{
			"id":            id,
			"name":          name,
			"breeder":       breeder,
			"other_parents": otherParents,
		})
	}
	return results
}

// GetDescendantsHandler returns strains that list this strain as a parent
func GetDescendantsHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	db := DBFromContext(c)
	descendants := GetDescendants(db, strainID)
	if descendants == nil {
		descendants = []gin.H{}
	}
	c.JSON(http.StatusOK, descendants)
}

// LookupStrainByName finds strains whose name matches a search query (for autocomplete)
func LookupStrainByName(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "LookupStrainByName")

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, []types.Strain{})
		return
	}
	if len(query) > utils.MaxNameLength {
		query = query[:utils.MaxNameLength]
	}

	db := DBFromContext(c)

	rows, err := db.Query(`
		SELECT s.id, s.name, b.name as breeder
		FROM strain s
		JOIN breeder b ON s.breeder_id = b.id
		WHERE LOWER(s.name) LIKE LOWER('%' || $1 || '%')
		ORDER BY s.name ASC
		LIMIT 10`, query)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to search strains")
		apiInternalError(c, "api_internal_error")
		return
	}
	defer rows.Close()

	var results []gin.H
	for rows.Next() {
		var id int
		var name, breeder string
		if err := rows.Scan(&id, &name, &breeder); err != nil {
			continue
		}
		results = append(results, gin.H{"id": id, "name": name, "breeder": breeder})
	}

	if results == nil {
		results = []gin.H{}
	}
	c.JSON(http.StatusOK, results)
}
