package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/cannadb"
	"isley/logger"
	"isley/service"
)

// ---------------------------------------------------------------------------
// CannaDB import handlers.
//
// Pattern A from the integration handoff: the user searches CannaDB by name
// and one-click imports a chosen strain into Isley's local library. Records
// are keyed on the CannaDB AT-URI (cannadb_uri) so a re-import upserts in
// place rather than duplicating.
// ---------------------------------------------------------------------------

// cannadbSearchRow is the trimmed search result returned to the frontend.
type cannadbSearchRow struct {
	URI         string  `json:"uri"`
	Name        string  `json:"name"`
	BreederName string  `json:"breeder_name"`
	Description string  `json:"description"`
	Similarity  float64 `json:"similarity"`
}

// cannadbEnabled reports whether the integration is switched on.
func cannadbEnabled(c *gin.Context) bool {
	return ConfigStoreFromContext(c).CannadbEnabled() == 1
}

// CannadbSearchHandler proxies searchStrains and returns candidate rows.
// GET /strains/cannadb/search?q=<name>&limit=<n>
func CannadbSearchHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "CannadbSearchHandler")

	if !cannadbEnabled(c) {
		apiBadRequest(c, "api_cannadb_disabled")
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		apiBadRequest(c, "api_cannadb_query_required")
		return
	}

	limit := 10
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 25 {
			limit = n
		}
	}

	baseURL := ConfigStoreFromContext(c).CannadbBaseURL()
	results, err := cannadb.SearchStrains(baseURL, query, limit)
	if err != nil {
		respondCannadbError(c, err, "api_cannadb_search_failed")
		return
	}

	rows := make([]cannadbSearchRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, cannadbSearchRow{
			URI:         r.URI,
			Name:        r.Name,
			BreederName: r.BreederName,
			Description: r.Description,
			Similarity:  r.Similarity,
		})
	}

	fieldLogger.WithField("count", len(rows)).Debug("CannaDB search complete")
	c.JSON(http.StatusOK, gin.H{"results": rows})
}

// CannadbImportHandler fetches a full strain record and upserts it locally.
// POST /strains/cannadb/import  {"uri": "<at-uri>"}
func CannadbImportHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "CannadbImportHandler")

	if !cannadbEnabled(c) {
		apiBadRequest(c, "api_cannadb_disabled")
		return
	}

	var req struct {
		URI string `json:"uri"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}
	req.URI = strings.TrimSpace(req.URI)
	if req.URI == "" || !strings.HasPrefix(req.URI, "at://") {
		apiBadRequest(c, "api_cannadb_invalid_uri")
		return
	}

	baseURL := ConfigStoreFromContext(c).CannadbBaseURL()
	rec, val, err := cannadb.GetStrain(baseURL, req.URI)
	if err != nil {
		respondCannadbError(c, err, "api_cannadb_import_failed")
		return
	}
	if val.Name == "" {
		apiInternalError(c, "api_cannadb_import_failed")
		return
	}
	breeder, err := cannadbBreeder(baseURL, val)
	if err != nil {
		respondCannadbError(c, err, "api_cannadb_import_failed")
		return
	}
	in := mapCannadbStrain(rec, val)
	in.Breeder = breeder

	strain, err := ServicesFromContext(c).Strains.ImportCannadb(c.Request.Context(), in)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to import strain")
		apiInternalError(c, "api_cannadb_import_failed")
		return
	}

	fieldLogger.WithField("strain_id", strain.ID).WithField("uri", req.URI).Info("Imported strain from CannaDB")
	c.JSON(http.StatusOK, gin.H{
		"id":          strain.ID,
		"name":        strain.Name,
		"cannadb_url": cannadb.WebURL(req.URI),
		"message":     T(c, "api_cannadb_imported"),
	})
}

// ---------------------------------------------------------------------------
// Mapping + persistence helpers
// ---------------------------------------------------------------------------

// mapCannadbStrain converts a CannaDB strain record into the service's
// import, applying the field transforms from the integration plan (§1).
func mapCannadbStrain(rec *cannadb.Record, val *cannadb.StrainValue) service.CannadbStrain {
	// indicaSativa: 0 = pure indica, 100 = pure sativa. Default 50/50 when
	// absent. Isley requires indica + sativa == 100.
	sativa := 50
	if val.IndicaSativa != nil {
		sativa = min(max(*val.IndicaSativa, 0), 100)
	}

	// cycleTime is a {min,max} in whole days; Isley stores a single int.
	// Prefer max, fall back to min.
	cycleTime := 0
	if val.CycleTime != nil {
		if val.CycleTime.Max != nil {
			cycleTime = *val.CycleTime.Max
		} else if val.CycleTime.Min != nil {
			cycleTime = *val.CycleTime.Min
		}
	}

	return service.CannadbStrain{
		URI:              rec.URI,
		IndexedAt:        rec.IndexedAt,
		Name:             val.Name,
		IndicaPct:        100 - sativa,
		SativaPct:        sativa,
		Autoflower:       val.Autoflower,
		CycleTimeDays:    cycleTime,
		ShortDescription: val.ShortDescription,
		Description:      val.Description, // markdown, stored raw
		URL:              val.SourceURL,
		ParentNames:      val.ParentNames,
	}
}

// cannadbBreeder resolves the strain's breeder: its full record when the
// strain links one, else the breederName fallback.
func cannadbBreeder(baseURL string, val *cannadb.StrainValue) (service.CannadbBreeder, error) {
	b := service.CannadbBreeder{Name: strings.TrimSpace(val.BreederName)}
	if val.Breeder == "" {
		return b, nil
	}
	rec, bval, err := cannadb.GetBreeder(baseURL, val.Breeder)
	if err != nil {
		// A breeder ref can be (rarely) unresolvable: fall back to the
		// name on RecordNotFound, surface other errors.
		var apiErr *cannadb.Error
		if errors.As(err, &apiErr) && apiErr.HTTPStatus == http.StatusNotFound {
			return b, nil
		}
		return b, err
	}
	if name := strings.TrimSpace(bval.Name); name != "" {
		b = service.CannadbBreeder{Name: name, URI: rec.URI, IndexedAt: rec.IndexedAt}
	}
	return b, nil
}

// respondCannadbError maps a client error to an appropriate API response.
func respondCannadbError(c *gin.Context, err error, fallbackKey string) {
	var apiErr *cannadb.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code == "RecordNotFound" || apiErr.HTTPStatus == http.StatusNotFound:
			apiNotFound(c, "api_cannadb_not_found")
			return
		case apiErr.Code == "RateLimited" || apiErr.HTTPStatus == http.StatusTooManyRequests:
			c.JSON(http.StatusTooManyRequests, gin.H{"error": T(c, "api_cannadb_rate_limited")})
			return
		}
	}
	logger.Log.WithError(err).Error("CannaDB request failed")
	apiInternalError(c, fallbackKey)
}
