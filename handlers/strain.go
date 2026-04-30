package handlers

import (
	"database/sql"
	"html"
	"isley/logger"
	model "isley/model"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Breeder helpers & handlers
// ---------------------------------------------------------------------------

// createBreeder inserts a new breeder into the database and returns the new ID.
// The transaction is expected to be managed by the caller
func createBreeder(tx *sql.Tx, name string) (int, error) {
	var id int
	err := tx.QueryRow("INSERT INTO breeder (name) VALUES ($1) RETURNING id", name).Scan(&id)
	return id, err
}

func GetBreeders(db *sql.DB) []types.Breeder {
	fieldLogger := logger.Log.WithField("func", "GetBreeders")

	rows, err := db.Query("SELECT id, name FROM breeder")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query breeders")
		return nil
	}

	var breeders []types.Breeder
	for rows.Next() {
		var breeder types.Breeder
		err = rows.Scan(&breeder.ID, &breeder.Name)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan breeder")
			return nil
		}
		breeders = append(breeders, breeder)
	}

	return breeders
}

func AddBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddBreederHandler")
	var breeder struct {
		Name string `json:"breeder_name"`
	}
	if err := c.ShouldBindJSON(&breeder); err != nil {
		fieldLogger.WithError(err).Error("Failed to add breeder")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("breeder_name", breeder.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Add breeder to database
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start breeder add transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	// Insert new breeder and return new id
	var breederID int
	breederID, err = createBreeder(tx, strainInput.NewBreeder)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to create new breeder")
		apiInternalError(c, "api_failed_to_add_breeder")
		return
	}
	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit strain add transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}
	ConfigStoreFromContext(c).AppendBreeder(types.Breeder{ID: breederID, Name: breeder.Name})

	c.JSON(http.StatusCreated, gin.H{"id": breederID})
}
func UpdateBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateBreederHandler")
	id := c.Param("id")
	var breeder struct {
		Name string `json:"breeder_name"`
	}
	if err := c.ShouldBindJSON(&breeder); err != nil {
		fieldLogger.WithError(err).Error("Failed to update breeder")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("breeder_name", breeder.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update breeder in database
	db := DBFromContext(c)

	// Update breeder in database
	_, err := db.Exec("UPDATE breeder SET name = $1 WHERE id = $2", breeder.Name, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update breeder")
		apiInternalError(c, "api_failed_to_update_breeder")
		return
	}

	//Reload Config
	ConfigStoreFromContext(c).SetBreeders(GetBreeders(db))

	apiOK(c, "api_breeder_updated")
}

func DeleteBreederHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteBreederHandler")
	id := c.Param("id")

	// Delete breeder from database
	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start breeder delete transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	// Query plants associated with this breeder to be deleted
	// This is two step to be able to reuse the deletePlantByID helper which will ensure cleanup of related records
	rows, err := tx.Query("SELECT p.id FROM plant p LEFT OUTER JOIN strain s on s.id = p.strain_id WHERE s.breeder_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to load breeder plants")
		apiInternalError(c, "api_failed_to_delete_plant")
		return
	}

	plantList := []int{}
	for rows.Next() {
		var plantId int
		err = rows.Scan(&plantId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to delete plant")
			continue
		}
		plantList = append(plantList, plantId)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Failed to iterate breeder plants")
		apiInternalError(c, "api_failed_to_delete_plant")
		return
	}

	for _, plantId := range plantList {
		if err := deletePlantByIDTx(tx, strconv.Itoa(plantId)); err != nil {
			fieldLogger.WithError(err).WithField("plantID", plantId).Error("Failed to delete plant")
			apiInternalError(c, "api_failed_to_delete_plant")
			return
		}
	}

	// Delete any strains associated with this breeder
	_, err = tx.Exec("DELETE FROM strain WHERE breeder_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete strains")
		apiInternalError(c, "api_failed_to_delete_strains")
		return
	}

	// Delete breeder from database
	_, err = tx.Exec("DELETE FROM breeder WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete breeder")
		apiInternalError(c, "api_failed_to_delete_breeder")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit breeder delete transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	//Reload Config
	store := ConfigStoreFromContext(c)
	store.SetBreeders(GetBreeders(db))
	store.SetStrains(GetStrains(db))

	apiOK(c, "api_breeder_deleted")
}

// ---------------------------------------------------------------------------
// Strain helpers & handlers
// ---------------------------------------------------------------------------
var strainInput struct {
	Name             string `json:"name"`
	BreederID        *int   `json:"breeder_id"` // Nullable for new breeders
	NewBreeder       string `json:"new_breeder"`
	Indica           int    `json:"indica"`
	Sativa           int    `json:"sativa"`
	Ruderalis        int    `json:"ruderalis"`
	Autoflower       bool   `json:"autoflower"`
	SeedCount        int    `json:"seed_count"`
	Description      string `json:"description"`
	ShortDescription string `json:"short_desc"`
	CycleTime        int    `json:"cycle_time"`
	Url              string `json:"url"`
}

func validateStrainFields(name, description, shortDesc, newBreeder, strainURL string) error {
	if err := utils.ValidateRequiredString("name", name, utils.MaxNameLength); err != nil {
		return err
	}
	if err := utils.ValidateStringLength("description", description, utils.MaxDescriptionLength); err != nil {
		return err
	}
	if err := utils.ValidateStringLength("short_desc", shortDesc, utils.MaxNameLength); err != nil {
		return err
	}
	if err := utils.ValidateStringLength("new_breeder", newBreeder, utils.MaxNameLength); err != nil {
		return err
	}
	return utils.ValidateWebURL("url", strainURL)
}

func GetStrains(db *sql.DB) []types.Strain {
	fieldLogger := logger.Log.WithField("func", "GetStrains")

	rows, err := db.Query(`
		SELECT s.id, s.name, b.id AS breeder_id, b.name AS breeder,
		       s.indica, s.sativa, s.ruderalis, s.autoflower,
		       s.description, COALESCE(s.short_desc, ''), s.seed_count
		FROM strain s
		LEFT OUTER JOIN breeder b ON s.breeder_id = b.id
		ORDER BY s.name ASC`)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query strains")
		return nil
	}

	var strains []types.Strain
	for rows.Next() {
		var strain types.Strain
		err = rows.Scan(&strain.ID, &strain.Name, &strain.BreederID, &strain.Breeder, &strain.Indica, &strain.Sativa, &strain.Ruderalis, &strain.Autoflower, &strain.Description, &strain.ShortDescription, &strain.SeedCount)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan strain")
			return nil
		}
		strains = append(strains, strain)
	}

	return strains
}

func GetStrain(db *sql.DB, id string) types.Strain {
	fieldLogger := logger.Log.WithField("func", "GetStrain")

	var strain types.Strain
	//join in breeder name
	err := db.QueryRow(`
		SELECT s.id, s.name, COALESCE(s.short_desc, ''),
		       b.name AS breeder, b.id AS breeder_id,
		       s.indica, s.sativa, s.ruderalis, s.autoflower,
		       s.seed_count, s.description,
		       COALESCE(s.cycle_time, 0), COALESCE(s.url, '')
		FROM strain s
		JOIN breeder b ON s.breeder_id = b.id
		WHERE s.id = $1`, id).Scan(
		&strain.ID, &strain.Name, &strain.ShortDescription,
		&strain.Breeder, &strain.BreederID,
		&strain.Indica, &strain.Sativa, &strain.Ruderalis, &strain.Autoflower,
		&strain.SeedCount, &strain.Description,
		&strain.CycleTime, &strain.Url)
	if err != nil {
		if err == sql.ErrNoRows {
			fieldLogger.Error("Strain not found")
		} else {
			fieldLogger.WithError(err).Error("Failed to fetch strain")
		}
		return types.Strain{}
	}
	//strain.Description = html.EscapeString(strain.Description)

	return strain
}

func AddStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddStrainHandler")
	// Parse the incoming JSON request
	if err := c.ShouldBindJSON(&strainInput); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_request_payload")
		return
	}

	if err := validateStrainFields(strainInput.Name, strainInput.Description, strainInput.ShortDescription, strainInput.NewBreeder, strainInput.Url); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Validate Indica, Sativa, and Ruderalis sum
	if strainInput.Indica+strainInput.Sativa+strainInput.Ruderalis != 100 {
		fieldLogger.Error("Indica, Sativa, and Ruderalis must sum to 100")
		apiBadRequest(c, "api_genetics_must_sum_100")
		return
	}

	// Open the database
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start strain add transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	// Check for new breeder and insert if needed
	var breederID int
	if strainInput.BreederID == nil {
		if strainInput.NewBreeder == "" {
			fieldLogger.Error("New breeder name is required")
			apiBadRequest(c, "api_new_breeder_name_required")
			return
		}
		breederID, err = createBreeder(tx, strainInput.NewBreeder)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new breeder")
			apiInternalError(c, "api_failed_to_add_breeder")
			return
		}
		store.SetBreeders(GetBreeders(db))
	} else {
		// Use existing breeder ID
		breederID = *strainInput.BreederID
	}

	// Insert the new strain into the database
	stmt := `
		INSERT INTO strain (
			name, breeder_id, indica, sativa, ruderalis,
			autoflower, seed_count, description, cycle_time, url, short_desc
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`
	//convert autoflower to int
	var autoflowerInt int
	if strainInput.Autoflower {
		autoflowerInt = 1
	} else {
		autoflowerInt = 0
	}
	var id int
	if err := tx.QueryRow(stmt, strainInput.Name, breederID, strainInput.Indica, strainInput.Sativa, strainInput.Ruderalis, autoflowerInt, strainInput.SeedCount, strainInput.Description, strainInput.CycleTime, strainInput.Url, strainInput.ShortDescription).Scan(&id); err != nil {
		fieldLogger.WithError(err).Error("Failed to insert strain")
		apiInternalError(c, "api_failed_to_add_strain")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit strain add transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	store.SetBreeders(GetBreeders(db))
	store.SetStrains(GetStrains(db))

	// Respond with success
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": T(c, "api_strain_added")})
}

func GetStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetStrainHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	// Open the database
	db := DBFromContext(c)

	var strain types.Strain

	err = db.QueryRow(`
		SELECT s.id, s.name,
		       b.name AS breeder, b.id AS breeder_id,
		       s.indica, s.sativa, s.ruderalis, s.autoflower,
		       s.description, COALESCE(s.short_desc, ''), s.seed_count,
		       COALESCE(s.cycle_time, 0), COALESCE(s.url, '')
		FROM strain s
		LEFT OUTER JOIN breeder b ON s.breeder_id = b.id
		WHERE s.id = $1`, id).Scan(
		&strain.ID, &strain.Name,
		&strain.Breeder, &strain.BreederID,
		&strain.Indica, &strain.Sativa, &strain.Ruderalis, &strain.Autoflower,
		&strain.Description, &strain.ShortDescription, &strain.SeedCount,
		&strain.CycleTime, &strain.Url)
	if err != nil {
		if err == sql.ErrNoRows {
			apiNotFound(c, "api_strain_not_found")
		} else {
			fieldLogger.WithError(err).Error("Failed to fetch strain")
			apiInternalError(c, "api_failed_to_fetch_strain")
		}
		return
	}

	c.JSON(http.StatusOK, strain)
}

func UpdateStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateStrainHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	if err := c.ShouldBindJSON(&strainInput); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_request_body")
		return
	}

	if err := validateStrainFields(strainInput.Name, strainInput.Description, strainInput.ShortDescription, strainInput.NewBreeder, strainInput.Url); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Validate Indica, Sativa, and Ruderalis sum
	if strainInput.Indica+strainInput.Sativa+strainInput.Ruderalis != 100 {
		fieldLogger.Error("Indica, Sativa, and Ruderalis must sum to 100")
		apiBadRequest(c, "api_genetics_must_sum_100")
		return
	}

	// Open the database
	db := DBFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start strain update transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	// Determine the breeder ID
	var breederID int
	if strainInput.BreederID == nil {
		if strainInput.NewBreeder == "" {
			fieldLogger.Error("New breeder name is required")
			apiBadRequest(c, "api_new_breeder_name_required")
			return
		}

		breederID, err = createBreeder(tx, strainInput.NewBreeder)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new breeder")
			apiInternalError(c, "api_failed_to_add_breeder")
			return
		}

		ConfigStoreFromContext(c).SetBreeders(GetBreeders(db))
	} else {
		breederID = *strainInput.BreederID
	}

	// Update the strain in the database
	updateStmt := `
		UPDATE strain
		SET name = $1,
		    breeder_id = $2,
		    indica = $3,
		    sativa = $4,
		    ruderalis = $5,
		    autoflower = $6,
		    description = $7,
		    seed_count = $8,
		    cycle_time = $9,
		    url = $10,
		    short_desc = $11
		WHERE id = $12
	`
	//Convert autoflower to int
	var autoflowerInt int
	if strainInput.Autoflower {
		autoflowerInt = 1
	} else {
		autoflowerInt = 0
	}
	_, err = tx.Exec(updateStmt, strainInput.Name, breederID, strainInput.Indica, strainInput.Sativa, strainInput.Ruderalis,
		autoflowerInt, strainInput.Description, strainInput.SeedCount, strainInput.CycleTime, strainInput.Url, strainInput.ShortDescription, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update strain")
		apiInternalError(c, "api_failed_to_update_strain")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit strain update transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	apiOK(c, "api_strain_updated")
}

func DeleteStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteStrainHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		apiBadRequest(c, "api_invalid_strain_id")
		return
	}

	// Open the database
	db := DBFromContext(c)

	result, err := db.Exec(`DELETE FROM strain WHERE id = $1`, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete strain")
		apiInternalError(c, "api_failed_to_delete_strain")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		fieldLogger.Error("Strain not found")
		apiNotFound(c, "api_strain_not_found")
		return
	}

	apiOK(c, "api_strain_deleted")
}

func InStockStrainsHandler(c *gin.Context) {
	db := DBFromContext(c)
	strains, err := getStrainsBySeedCount(db, true)
	if err != nil {
		apiInternalError(c, "api_failed_to_fetch_strains")
		return
	}
	c.JSON(http.StatusOK, strains)
}
func OutOfStockStrainsHandler(c *gin.Context) {
	db := DBFromContext(c)
	strains, err := getStrainsBySeedCount(db, false)
	if err != nil {
		apiInternalError(c, "api_failed_to_fetch_strains")
		return
	}
	c.JSON(http.StatusOK, strains)
}
func getStrainsBySeedCount(db *sql.DB, inStock bool) ([]types.Strain, error) {
	fieldLogger := logger.Log.WithField("func", "getStrainsBySeedCount")
	op := ">"
	if !inStock {
		op = "="
	}
	// string_agg is PostgreSQL-only; SQLite uses GROUP_CONCAT.
	aggExpr := "coalesce(GROUP_CONCAT(sl.parent_name, ', '), '')"
	if model.IsPostgres() {
		aggExpr = "coalesce(string_agg(sl.parent_name, ', ' ORDER BY sl.parent_name), '')"
	}

	query := `
		SELECT s.id, s.name,
		       b.name AS breeder, b.id AS breeder_id,
		       s.indica, s.sativa, s.ruderalis, s.autoflower,
		       s.seed_count, s.description,
		       COALESCE(s.short_desc, ''),
		       COALESCE(s.cycle_time, 0),
		       COALESCE(s.url, ''),
		       ` + aggExpr + `
		FROM strain s
		JOIN breeder b ON s.breeder_id = b.id
		LEFT JOIN strain_lineage sl ON sl.strain_id = s.id
		WHERE s.seed_count ` + op + ` 0
		GROUP BY s.id, s.name, b.name, b.id,
		         s.indica, s.sativa, s.ruderalis, s.autoflower,
		         s.seed_count, s.description, s.short_desc,
		         s.cycle_time, s.url
		ORDER BY s.name ASC
	`

	rows, err := db.Query(query)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query strains")
		return nil, err
	}
	defer rows.Close()

	var strains []types.Strain
	for rows.Next() {
		var strain types.Strain
		if err := rows.Scan(&strain.ID, &strain.Name, &strain.Breeder, &strain.BreederID,
			&strain.Indica, &strain.Sativa, &strain.Ruderalis, &strain.Autoflower, &strain.SeedCount,
			&strain.Description, &strain.ShortDescription, &strain.CycleTime, &strain.Url,
			&strain.Lineage); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan strain")
			return nil, err
		}
		strain.Description = html.EscapeString(strain.Description)
		strains = append(strains, strain)
	}

	return strains, nil
}

func PlantsByStrainHandler(context *gin.Context) {
	fieldLogger := logger.Log.WithField("handler", "PlantsByStrainHandler")

	// Parse strain ID from query parameter
	strainID, err := strconv.Atoi(context.Param("strainID"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		apiBadRequest(context, "api_invalid_strain_id")
		return
	}

	fieldLogger = fieldLogger.WithField("strainID", strainID)
	fieldLogger.Info("Fetching plants for strain")

	db := DBFromContext(context)

	// Query plants with the given strain ID
	rows, err := db.Query(`SELECT id, name FROM plant WHERE strain_id = $1 ORDER BY name ASC`, strainID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query database")
		apiInternalError(context, "api_failed_to_fetch_plants")
		return
	}
	defer rows.Close()

	// Parse query results
	var plants []types.Plant
	for rows.Next() {
		var plant types.Plant
		if err := rows.Scan(&plant.ID, &plant.Name); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan row")
			apiInternalError(context, "api_failed_to_process_results")
			return
		}
		plants = append(plants, plant)
	}

	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Error iterating rows")
		apiInternalError(context, "api_failed_to_process_results")
		return
	}

	// Return the list of plants as JSON
	fieldLogger.WithField("plantCount", len(plants)).Info("Plants fetched successfully")
	context.JSON(http.StatusOK, plants)
}
