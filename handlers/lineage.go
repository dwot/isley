package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/model/types"
	"isley/service"
	"isley/utils"
)

// toLineageTree maps an ancestry onto the tree the strain page draws.
func toLineageTree(strainID int, nodes []service.AncestryNode) []types.StrainLineage {
	out := make([]types.StrainLineage, len(nodes))
	for i, n := range nodes {
		out[i] = types.StrainLineage{ID: n.ID, StrainID: strainID, ParentName: n.ParentName}
		if n.Parent != nil {
			pid := n.Parent.ID
			out[i].ParentStrainID = &pid
			if len(n.Parents) > 0 {
				out[i].Children = toLineageTree(pid, n.Parents)
			}
		}
	}
	return out
}

// GetLineageHandler returns a strain's ancestry: its parents, each with
// theirs under "children".
func GetLineageHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	nodes, err := ServicesFromContext(c).Strains.Ancestry(c.Request.Context(), strainID)
	if err != nil {
		apiServiceError(c, err, "api_internal_error")
		return
	}
	c.JSON(http.StatusOK, toLineageTree(strainID, nodes))
}

type lineageRequest struct {
	ParentName     string `json:"parent_name"`
	ParentStrainID *int   `json:"parent_strain_id"`
}

func (r lineageRequest) input() service.ParentInput {
	return service.ParentInput{Name: r.ParentName, StrainID: r.ParentStrainID}
}

// requireParentName keeps the form's rule that a parent always has a name.
func requireParentName(c *gin.Context, name string) bool {
	if err := utils.ValidateRequiredString("parent_name", name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return false
	}
	return true
}

// AddLineageHandler adds a parent to a strain.
func AddLineageHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req lineageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	if !requireParentName(c, req.ParentName) {
		return
	}
	e, err := ServicesFromContext(c).Strains.AddParent(c.Request.Context(), strainID, req.input())
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_lineage")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": e.ID})
}

// DeleteLineageHandler removes a parent.
func DeleteLineageHandler(c *gin.Context) {
	lineageID, err := strconv.Atoi(c.Param("lineageID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_lineage_id")
		return
	}
	if err := ServicesFromContext(c).Strains.DeleteParent(c.Request.Context(), lineageID); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_lineage")
		return
	}
	apiOK(c, "api_lineage_deleted")
}

// UpdateLineageHandler changes a parent (its name, or the strain it links to).
func UpdateLineageHandler(c *gin.Context) {
	lineageID, err := strconv.Atoi(c.Param("lineageID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_lineage_id")
		return
	}
	var req lineageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	if !requireParentName(c, req.ParentName) {
		return
	}
	if _, err := ServicesFromContext(c).Strains.UpdateParent(c.Request.Context(), lineageID, req.input()); err != nil {
		apiServiceError(c, err, "api_failed_to_update_lineage")
		return
	}
	apiOK(c, "api_lineage_updated")
}

// SetLineageHandler replaces a strain's parents. Rows left blank in the
// form are skipped.
func SetLineageHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req struct {
		Parents []lineageRequest `json:"parents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request")
		return
	}
	parents := make([]service.ParentInput, 0, len(req.Parents))
	for _, p := range req.Parents {
		if strings.TrimSpace(p.ParentName) != "" {
			parents = append(parents, p.input())
		}
	}
	if _, err := ServicesFromContext(c).Strains.SetLineage(c.Request.Context(), strainID, parents); err != nil {
		apiServiceError(c, err, "api_failed_to_update_lineage")
		return
	}
	apiOK(c, "api_lineage_updated")
}

// GetDescendantsHandler lists the strains that name this one as a parent,
// with their other parents.
func GetDescendantsHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	ds, err := ServicesFromContext(c).Strains.Descendants(c.Request.Context(), strainID)
	if err != nil {
		apiServiceError(c, err, "api_internal_error")
		return
	}
	out := make([]gin.H, len(ds))
	for i, d := range ds {
		out[i] = gin.H{
			"id": d.ID, "name": d.Name, "breeder": d.Breeder.Name,
			"other_parents": strings.Join(d.OtherParents, ", "),
		}
	}
	c.JSON(http.StatusOK, out)
}

// LookupStrainByName finds up to 10 strains whose name contains q, for the
// parent picker.
func LookupStrainByName(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}
	if len(query) > utils.MaxNameLength {
		query = query[:utils.MaxNameLength]
	}
	strains, _, err := ServicesFromContext(c).Strains.List(c.Request.Context(), service.StrainFilter{Query: query, Limit: 10})
	if err != nil {
		apiServiceError(c, err, "api_internal_error")
		return
	}
	out := make([]gin.H, len(strains))
	for i, st := range strains {
		out[i] = gin.H{"id": st.ID, "name": st.Name, "breeder": st.Breeder.Name}
	}
	c.JSON(http.StatusOK, out)
}
