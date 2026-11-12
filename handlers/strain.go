package handlers

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/model/types"
	"isley/service"
)

// ---------------------------------------------------------------------------
// Breeders
// ---------------------------------------------------------------------------

type breederRequest struct {
	Name string `json:"breeder_name"`
}

func AddBreederHandler(c *gin.Context) {
	var req breederRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	b, err := ServicesFromContext(c).Breeders.Create(c.Request.Context(), req.Name)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_breeder")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": b.ID})
}

func UpdateBreederHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	var req breederRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if _, err := ServicesFromContext(c).Breeders.Rename(c.Request.Context(), id, req.Name); err != nil {
		apiServiceError(c, err, "api_failed_to_update_breeder")
		return
	}
	apiOK(c, "api_breeder_updated")
}

// DeleteBreederHandler deletes a breeder with its strains and their plants,
// as the settings page warns.
func DeleteBreederHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := ServicesFromContext(c).Breeders.DeleteWithEverything(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_breeder")
		return
	}
	apiOK(c, "api_breeder_deleted")
}

// ---------------------------------------------------------------------------
// Strains
// ---------------------------------------------------------------------------

// toTypeStrain maps a strain onto the shape the web pages and the legacy
// JSON endpoints use.
func toTypeStrain(st service.Strain) types.Strain {
	out := types.Strain{
		ID: st.ID, Name: st.Name, Breeder: st.Breeder.Name, BreederID: st.Breeder.ID,
		Indica: st.IndicaPct, Sativa: st.SativaPct, Autoflower: st.Autoflower,
		Description: st.Description, SeedCount: st.SeedCount, Url: st.URL,
		ShortDescription: st.ShortDescription, CannadbURI: st.CannadbURI,
	}
	if st.CycleTimeDays != nil {
		out.CycleTime = *st.CycleTimeDays
	}
	names := make([]string, len(st.Parents))
	for i, p := range st.Parents {
		names[i] = p.ParentName
	}
	out.Lineage = strings.Join(names, ", ")
	return out
}

// GetStrain returns a strain for the strain pages, or the zero Strain when
// there is none.
func GetStrain(ctx context.Context, svc *service.Services, id string) types.Strain {
	n, err := strconv.Atoi(id)
	if err != nil {
		return types.Strain{}
	}
	st, err := svc.Strains.Get(ctx, n)
	if err != nil {
		if !service.IsKind(err, service.KindNotFound) {
			logger.Log.WithError(err).WithField("strain", n).Error("Failed to fetch strain")
		}
		return types.Strain{}
	}
	return toTypeStrain(st)
}

// strainRequest is the strain form. A null breeder_id means new_breeder
// names the breeder.
type strainRequest struct {
	Name             string `json:"name"`
	BreederID        *int   `json:"breeder_id"`
	NewBreeder       string `json:"new_breeder"`
	Indica           int    `json:"indica"`
	Sativa           int    `json:"sativa"`
	Autoflower       bool   `json:"autoflower"`
	SeedCount        int    `json:"seed_count"`
	Description      string `json:"description"`
	ShortDescription string `json:"short_desc"`
	CycleTime        int    `json:"cycle_time"`
	Url              string `json:"url"`
}

// check keeps the form's own messages for its two most common mistakes.
func (r strainRequest) check(c *gin.Context) bool {
	if r.Indica+r.Sativa != 100 {
		apiBadRequest(c, "api_indica_sativa_must_sum_100")
		return false
	}
	if r.BreederID == nil && strings.TrimSpace(r.NewBreeder) == "" {
		apiBadRequest(c, "api_new_breeder_name_required")
		return false
	}
	return true
}

func (r strainRequest) breeder() (int, string) {
	if r.BreederID == nil {
		return 0, r.NewBreeder
	}
	return *r.BreederID, ""
}

func AddStrainHandler(c *gin.Context) {
	var req strainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	if !req.check(c) {
		return
	}
	breederID, breederName := req.breeder()
	cycle := req.CycleTime
	st, err := ServicesFromContext(c).Strains.Create(c.Request.Context(), service.StrainInput{
		Name: req.Name, BreederID: breederID, BreederName: breederName,
		IndicaPct: req.Indica, SativaPct: req.Sativa, Autoflower: req.Autoflower,
		SeedCount: req.SeedCount, CycleTimeDays: &cycle, URL: req.Url,
		ShortDescription: req.ShortDescription, Description: req.Description,
	})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_add_strain")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": st.ID, "message": T(c, "api_strain_added")})
}

func GetStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	st, err := ServicesFromContext(c).Strains.Get(c.Request.Context(), id)
	if err != nil {
		apiServiceError(c, err, "api_failed_to_fetch_strain")
		return
	}
	out := toTypeStrain(st)
	out.Lineage = ""
	c.JSON(http.StatusOK, out)
}

// UpdateStrainHandler saves the whole strain form.
func UpdateStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	var req strainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_body")
		return
	}
	if !req.check(c) {
		return
	}
	breederID, breederName := req.breeder()
	upd := service.StrainUpdate{
		Name:             service.Put(&req.Name),
		BreederName:      breederName,
		IndicaPct:        service.Put(&req.Indica),
		SativaPct:        service.Put(&req.Sativa),
		Autoflower:       service.Put(&req.Autoflower),
		SeedCount:        service.Put(&req.SeedCount),
		CycleTimeDays:    service.Put(&req.CycleTime),
		URL:              service.Put(&req.Url),
		ShortDescription: service.Put(&req.ShortDescription),
		Description:      service.Put(&req.Description),
	}
	if breederName == "" {
		upd.BreederID = service.Put(&breederID)
	}
	if _, err := ServicesFromContext(c).Strains.Update(c.Request.Context(), id, upd); err != nil {
		apiServiceError(c, err, "api_failed_to_update_strain")
		return
	}
	apiOK(c, "api_strain_updated")
}

func DeleteStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	if err := ServicesFromContext(c).Strains.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_strain")
		return
	}
	apiOK(c, "api_strain_deleted")
}

func InStockStrainsHandler(c *gin.Context)    { strainsBySeedCount(c, true) }
func OutOfStockStrainsHandler(c *gin.Context) { strainsBySeedCount(c, false) }

func strainsBySeedCount(c *gin.Context, inStock bool) {
	strains, _, err := ServicesFromContext(c).Strains.List(c.Request.Context(), service.StrainFilter{InStock: &inStock})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_fetch_strains")
		return
	}
	out := make([]types.Strain, len(strains))
	for i, st := range strains {
		out[i] = toTypeStrain(st)
		// The lists never carried CannaDB provenance.
		out[i].CannadbURI = ""
	}
	c.JSON(http.StatusOK, out)
}

// PlantsByStrainHandler lists the plants of a strain by name.
func PlantsByStrainHandler(c *gin.Context) {
	strainID, err := strconv.Atoi(c.Param("strainID"))
	if err != nil {
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}
	plants, _, err := ServicesFromContext(c).Plants.List(c.Request.Context(), service.PlantFilter{StrainID: strainID})
	if err != nil {
		apiServiceError(c, err, "api_failed_to_fetch_plants")
		return
	}
	sort.SliceStable(plants, func(i, j int) bool { return plants[i].Name < plants[j].Name })
	out := make([]types.Plant, len(plants))
	for i, p := range plants {
		out[i] = types.Plant{ID: uint(p.ID), Name: p.Name}
	}
	c.JSON(http.StatusOK, out)
}
