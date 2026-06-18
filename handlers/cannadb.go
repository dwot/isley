package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/model/types"
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
	results, err := cannadbSearchStrains(baseURL, query, limit)
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

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)
	baseURL := store.CannadbBaseURL()

	rec, val, err := cannadbGetStrain(baseURL, req.URI)
	if err != nil {
		respondCannadbError(c, err, "api_cannadb_import_failed")
		return
	}
	if val.Name == "" {
		apiInternalError(c, "api_cannadb_import_failed")
		return
	}

	// Resolve + upsert the breeder first so the strain FK resolves locally.
	breederID, err := importCannadbBreeder(db, baseURL, val)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to resolve breeder")
		apiInternalError(c, "api_cannadb_import_failed")
		return
	}

	strain := mapCannadbStrain(rec, val)

	strainID, err := upsertCannadbStrain(db, breederID, strain)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to upsert strain")
		apiInternalError(c, "api_cannadb_import_failed")
		return
	}

	if err := replaceCannadbLineage(db, strainID, val.ParentNames); err != nil {
		// Lineage is best-effort; log but don't fail the import.
		fieldLogger.WithError(err).Warn("Failed to import lineage")
	}

	// Refresh in-memory caches so the UI reflects the import immediately.
	store.SetBreeders(GetBreeders(db))
	store.SetStrains(GetStrains(db))

	fieldLogger.WithField("strain_id", strainID).WithField("uri", req.URI).Info("Imported strain from CannaDB")
	c.JSON(http.StatusOK, gin.H{
		"id":          strainID,
		"name":        strain.Name,
		"cannadb_url": cannadbWebURL(req.URI),
		"message":     T(c, "api_cannadb_imported"),
	})
}

// ---------------------------------------------------------------------------
// Mapping + persistence helpers
// ---------------------------------------------------------------------------

// mapCannadbStrain converts a CannaDB strain record into Isley's Strain,
// applying the field transforms from the integration plan (§1).
func mapCannadbStrain(rec *cannadbRecord, val *cannadbStrainValue) types.Strain {
	// indicaSativa: 0 = pure indica, 100 = pure sativa. Default 50/50 when
	// absent. Isley requires indica + sativa == 100.
	sativa := 50
	if val.IndicaSativa != nil {
		sativa = *val.IndicaSativa
		if sativa < 0 {
			sativa = 0
		}
		if sativa > 100 {
			sativa = 100
		}
	}
	indica := 100 - sativa

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

	return types.Strain{
		Name:             val.Name,
		Indica:           indica,
		Sativa:           sativa,
		Autoflower:       val.Autoflower,
		Description:      val.Description, // markdown, stored raw
		ShortDescription: val.ShortDescription,
		CycleTime:        cycleTime,
		Url:              val.SourceURL,
		CannadbURI:       rec.URI,
		CannadbIndexedAt: rec.IndexedAt,
	}
}

// importCannadbBreeder resolves the strain's breeder (full record when a
// breeder AT-URI is present, else the breederName fallback) and upserts it
// locally, returning the local breeder id.
func importCannadbBreeder(db *sql.DB, baseURL string, val *cannadbStrainValue) (int, error) {
	name := strings.TrimSpace(val.BreederName)
	breederURI := ""
	indexedAt := ""

	if val.Breeder != "" {
		rec, bval, err := cannadbGetBreeder(baseURL, val.Breeder)
		if err != nil {
			// A breeder ref can be absent or (rarely) unresolvable — fall back
			// to breederName on RecordNotFound, surface other errors.
			var apiErr *cannadbError
			if !errors.As(err, &apiErr) || apiErr.HTTPStatus != http.StatusNotFound {
				return 0, err
			}
		} else if strings.TrimSpace(bval.Name) != "" {
			name = strings.TrimSpace(bval.Name)
			breederURI = rec.URI
			indexedAt = rec.IndexedAt
		}
	}

	if name == "" {
		name = "Unknown Breeder"
	}

	return upsertCannadbBreeder(db, name, breederURI, indexedAt)
}

// upsertCannadbBreeder finds an existing breeder by CannaDB URI, then by name,
// inserting a new one if neither matches. Returns the breeder id.
func upsertCannadbBreeder(db *sql.DB, name, uri, indexedAt string) (int, error) {
	var id int

	if uri != "" {
		err := db.QueryRow("SELECT id FROM breeder WHERE cannadb_uri = $1", uri).Scan(&id)
		if err == nil {
			_, uerr := db.Exec("UPDATE breeder SET name = $1, cannadb_indexed_at = $2 WHERE id = $3", name, indexedAt, id)
			return id, uerr
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}

	// Match an existing breeder by name (case-insensitive) to avoid duplicates
	// when the user already added it manually.
	err := db.QueryRow("SELECT id FROM breeder WHERE LOWER(name) = LOWER($1)", name).Scan(&id)
	if err == nil {
		if uri != "" {
			_, uerr := db.Exec("UPDATE breeder SET cannadb_uri = $1, cannadb_indexed_at = $2 WHERE id = $3", uri, indexedAt, id)
			return id, uerr
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	// Insert new. nullableStr keeps cannadb_uri NULL (not "") so the partial
	// unique index doesn't collide across manually-added breeders.
	err = db.QueryRow(
		"INSERT INTO breeder (name, cannadb_uri, cannadb_indexed_at) VALUES ($1, $2, $3) RETURNING id",
		name, nullableStr(uri), nullableStr(indexedAt),
	).Scan(&id)
	return id, err
}

// upsertCannadbStrain inserts a new strain or updates the existing one keyed
// on cannadb_uri. Returns the strain id.
func upsertCannadbStrain(db *sql.DB, breederID int, s types.Strain) (int, error) {
	autoflower := 0
	if s.Autoflower {
		autoflower = 1
	}

	var id int
	err := db.QueryRow("SELECT id FROM strain WHERE cannadb_uri = $1", s.CannadbURI).Scan(&id)
	switch {
	case err == nil:
		_, uerr := db.Exec(`
			UPDATE strain
			SET name = $1, breeder_id = $2, indica = $3, sativa = $4, autoflower = $5,
			    description = $6, short_desc = $7, cycle_time = $8, url = $9, cannadb_indexed_at = $10
			WHERE id = $11`,
			s.Name, breederID, s.Indica, s.Sativa, autoflower,
			s.Description, s.ShortDescription, s.CycleTime, s.Url, s.CannadbIndexedAt, id)
		return id, uerr
	case errors.Is(err, sql.ErrNoRows):
		ierr := db.QueryRow(`
			INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, seed_count,
			                    description, short_desc, cycle_time, url, cannadb_uri, cannadb_indexed_at)
			VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $8, $9, $10, $11) RETURNING id`,
			s.Name, breederID, s.Indica, s.Sativa, autoflower,
			s.Description, s.ShortDescription, s.CycleTime, s.Url, s.CannadbURI, nullableStr(s.CannadbIndexedAt)).Scan(&id)
		return id, ierr
	default:
		return 0, err
	}
}

// replaceCannadbLineage rewrites the strain's lineage from the CannaDB
// parentNames (display fallbacks). Parent strains are not recursively imported
// in v1, so parent_strain_id is left NULL.
func replaceCannadbLineage(db *sql.DB, strainID int, parentNames []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM strain_lineage WHERE strain_id = $1", strainID); err != nil {
		tx.Rollback()
		return err
	}
	for _, name := range parentNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := tx.Exec(
			"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, NULL)",
			strainID, name); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// nullableStr returns nil for empty strings so NULL is stored instead of "",
// keeping the partial-unique cannadb_uri index well-behaved.
func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// respondCannadbError maps a client error to an appropriate API response.
func respondCannadbError(c *gin.Context, err error, fallbackKey string) {
	var apiErr *cannadbError
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
