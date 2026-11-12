package handlers

// Smoke report criteria, managed under Settings › Customization.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"isley/model/types"
	"isley/service"
)

// criterionError keeps the criteria form's messages.
func criterionError(c *gin.Context, err error, fallbackKey string) {
	if e, ok := service.AsError(err); ok && e.Fields["name"] != "" {
		apiBadRequest(c, "api_invalid_smoke_criterion_name")
		return
	}
	apiServiceError(c, err, fallbackKey)
}

// GetSmokeCriteriaHandler returns every criterion, archived ones included,
// so the Settings table can grey them out.
func GetSmokeCriteriaHandler(c *gin.Context) {
	all, err := ServicesFromContext(c).SmokeReports.Criteria(c.Request.Context(), false)
	if err != nil {
		apiServiceError(c, err, "api_internal_error")
		return
	}
	out := make([]types.SmokeReportCriterion, len(all))
	for i, cr := range all {
		out[i] = types.SmokeReportCriterion{ID: cr.ID, Name: cr.Name, SortOrder: cr.SortOrder, Active: cr.Active}
	}
	c.JSON(http.StatusOK, out)
}

// AddSmokeCriterionHandler creates a criterion.
func AddSmokeCriterionHandler(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	cr, err := ServicesFromContext(c).SmokeReports.CreateCriterion(c.Request.Context(), req.Name, req.SortOrder)
	if err != nil {
		criterionError(c, err, "api_failed_to_add_smoke_criterion")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": cr.ID})
}

// UpdateSmokeCriterionHandler renames, reorders, archives or restores a
// criterion; scores keep pointing at it and take the new name.
func UpdateSmokeCriterionHandler(c *gin.Context) {
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
	if _, err := ServicesFromContext(c).SmokeReports.UpdateCriterion(c.Request.Context(), id, req.Name, req.SortOrder, req.Active); err != nil {
		criterionError(c, err, "api_failed_to_update_smoke_criterion")
		return
	}
	apiOK(c, "api_smoke_criterion_updated")
}

// DeleteSmokeCriterionHandler deletes a criterion nobody has scored and
// archives one with scores; the response says which, so the page can word
// its confirmation honestly.
func DeleteSmokeCriterionHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_smoke_criterion_id")
		return
	}
	archived, err := ServicesFromContext(c).SmokeReports.DeleteCriterion(c.Request.Context(), id)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_delete_smoke_criterion")
		return
	}
	c.JSON(http.StatusOK, gin.H{"archived": archived, "message": T(c, "api_smoke_criterion_deleted")})
}
