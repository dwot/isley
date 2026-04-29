package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"isley/config"
	"isley/logger"
	model "isley/model"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func AddPlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddPlant")
	var input struct {
		Name      string `json:"name"`
		ZoneID    *int   `json:"zone_id"`
		NewZone   string `json:"new_zone"`
		StrainID  *int   `json:"strain_id"`
		NewStrain *struct {
			Name       string `json:"name"`
			BreederId  int    `json:"breeder_id"`
			NewBreeder string `json:"new_breeder"`
		} `json:"new_strain"`
		StatusID           int    `json:"status_id"`
		Date               string `json:"date"`
		Sensors            string `json:"sensors"`
		Clone              int    `json:"clone"`
		ParentID           int    `json:"parent_id"`
		DecrementSeedCount bool   `json:"decrement_seed_count"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_input")
		return
	}
	// Validate string lengths
	if err := utils.ValidateRequiredString("name", input.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("new_zone", input.NewZone, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(db, store, input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			apiInternalError(c, "api_failed_to_create_new_zone")
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	// Handle new strain creation
	if input.StrainID == nil && input.NewStrain != nil {
		// Insert new strain into the database
		strainID, err := CreateNewStrain(db, store, input.NewStrain)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new strain")
			apiInternalError(c, "api_failed_to_create_new_strain")
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	// Insert plant, decrement seed count, and create initial status log
	// inside a transaction so partial writes cannot occur.
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		apiInternalError(c, "api_failed_to_create_plant")
		return
	}
	defer tx.Rollback() // no-op after Commit

	plantID := 0
	err = tx.QueryRow("INSERT INTO plant (name, zone_id, strain_id, description, clone, parent_plant_id, start_dt, sensors) VALUES ($1, $2, $3, '', $4, NULLIF($5, 0), $6, '[]') RETURNING id", input.Name, *input.ZoneID, *input.StrainID, input.Clone, input.ParentID, input.Date).Scan(&plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant")
		apiInternalError(c, "api_failed_to_create_plant")
		return
	} else if plantID == 0 {
		fieldLogger.Error("Failed to retrieve plant ID")
		apiInternalError(c, "api_failed_to_create_plant")
		return
	}

	if input.DecrementSeedCount {
		var query string
		if model.IsPostgres() {
			query = "UPDATE strain SET seed_count = GREATEST(0, seed_count - 1) WHERE id = $1"
		} else {
			query = "UPDATE strain SET seed_count = MAX(0, seed_count - 1) WHERE id = $1"
		}
		if _, err := tx.Exec(query, *input.StrainID); err != nil {
			fieldLogger.WithError(err).Error("Failed to decrement seed count")
			apiInternalError(c, "api_failed_to_create_plant")
			return
		}
	}

	if _, err := tx.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)", plantID, input.StatusID, input.Date); err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant status log")
		apiInternalError(c, "api_failed_to_create_plant")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit transaction")
		apiInternalError(c, "api_failed_to_create_plant")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": plantID, "message": T(c, "api_plant_added")})
}

func GetActivities(db *sql.DB) []types.Activity {
	fieldLogger := logger.Log.WithField("func", "GetActivities")

	rows, err := db.Query(`
		SELECT
			a.id,
			a.name,
			a.is_watering,
			a.is_feeding,
			am.metric_id,
			am.required
		FROM activity a
		LEFT JOIN activity_metric am ON am.activity_id = a.id
		ORDER BY a.name`)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities")
		return nil
	}
	defer rows.Close()

	var activities []types.Activity
	activityIdx := make(map[int]int)
	for rows.Next() {
		var (
			activityID   int
			activityName string
			isWatering   bool
			isFeeding    bool
			// Use Null types for optional metric fields
			metricID sql.NullInt64
			required sql.NullBool
		)

		err = rows.Scan(&activityID, &activityName, &isWatering, &isFeeding, &metricID, &required)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan activity")
			return nil
		}

		idx, exists := activityIdx[activityID]
		if !exists {
			activities = append(activities, types.Activity{
				ID:         activityID,
				Name:       activityName,
				IsWatering: isWatering,
				IsFeeding:  isFeeding,
				Metrics:    []types.ActivityMetricLink{},
			})
			idx = len(activities) - 1
			activityIdx[activityID] = idx
		}

		if metricID.Valid && required.Valid {
			activities[idx].Metrics = append(activities[idx].Metrics, types.ActivityMetricLink{
				MetricID: int(metricID.Int64),
				Required: required.Bool,
			})
		}
	}

	return activities
}

func GetMetrics(db *sql.DB) []types.Metric {
	fieldLogger := logger.Log.WithField("func", "GetMetrics")

	rows, err := db.Query("SELECT id, name, unit FROM metric")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query metrics")
		return nil
	}

	var measurements []types.Metric
	for rows.Next() {
		var measurement types.Metric
		err = rows.Scan(&measurement.ID, &measurement.Name, &measurement.Unit)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan metric")
			return nil
		}
		measurements = append(measurements, measurement)
	}

	return measurements
}

func GetStatuses(db *sql.DB) []types.Status {
	fieldLogger := logger.Log.WithField("func", "GetStatuses")

	rows, err := db.Query("SELECT id, status FROM plant_status ORDER BY status_order")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query statuses")
		return nil
	}

	var statuses []types.Status
	for rows.Next() {
		var status types.Status
		err = rows.Scan(&status.ID, &status.Status)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan status")
			return nil
		}
		statuses = append(statuses, status)
	}

	return statuses

}

// CreateNewStrain inserts a new strain (and optionally a new breeder)
// and returns the new strain id. If store is non-nil it is refreshed
// from the DB so subsequent reads observe the new rows; tests that
// don't care about the in-memory side-effect may pass nil.
func CreateNewStrain(db *sql.DB, store *config.Store, newStrain *struct {
	Name       string `json:"name"`
	BreederId  int    `json:"breeder_id"`
	NewBreeder string `json:"new_breeder"`
}) (int, error) {
	fieldLogger := logger.Log.WithField("func", "CreateNewStrain")
	var breederId int

	// Check if a new breeder needs to be added
	if newStrain.BreederId == 0 && newStrain.NewBreeder != "" {
		// Insert the new breeder into the `breeder` table
		err := db.QueryRow("INSERT INTO breeder (name) VALUES ($1) RETURNING id", newStrain.NewBreeder).Scan(&breederId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert new breeder")
			return 0, fmt.Errorf("failed to insert new breeder: %w", err)
		}

		if store != nil {
			store.SetBreeders(GetBreeders(db))
		}
	} else {
		// Use the existing breeder ID
		breederId = newStrain.BreederId
	}

	// Insert the new strain into the `strain` table
	var id int
	// Use numeric autoflower flag to be consistent with other handlers
	autoflowerInt := 1 // default true
	err := db.QueryRow(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		newStrain.Name, breederId, 50, 50, autoflowerInt, "", 0).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert new strain")
		return 0, fmt.Errorf("failed to insert new strain: %w", err)
	}

	if store != nil {
		store.SetStrains(GetStrains(db))
	}

	return id, nil
}

func DeletePlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeletePlant")
	id := c.Param("id")

	db := DBFromContext(c)
	err := DeletePlantById(db, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant")
		apiInternalError(c, "api_failed_to_delete_plant")
		return
	}

	apiOK(c, "api_plant_deleted")
}

func DeletePlantById(db *sql.DB, id string) error {
	fieldLogger := logger.Log.WithField("func", "DeletePlantById")

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // no-op after Commit

	// Delete child records first, then the plant itself.
	deletes := []struct {
		table string
		query string
	}{
		{"plant_images", "DELETE FROM plant_images WHERE plant_id = $1"},
		{"plant_measurements", "DELETE FROM plant_measurements WHERE plant_id = $1"},
		{"plant_activity", "DELETE FROM plant_activity WHERE plant_id = $1"},
		{"plant_status_log", "DELETE FROM plant_status_log WHERE plant_id = $1"},
		{"plant", "DELETE FROM plant WHERE id = $1"},
	}
	for _, d := range deletes {
		if _, err := tx.Exec(d.query, id); err != nil {
			fieldLogger.WithError(err).WithField("table", d.table).Error("Failed to delete records")
			return fmt.Errorf("failed to delete from %s: %w", d.table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit delete transaction")
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func GetPlant(db *sql.DB, id string) types.Plant {
	fieldLogger := logger.Log.WithField("func", "GetPlant")
	var plant types.Plant

	// --- Load base plant data via single JOIN query ---
	var orderByExpr string
	if model.IsPostgres() {
		orderByExpr = "EXTRACT(EPOCH FROM psl.date)"
	} else {
		orderByExpr = "strftime('%s', psl.date)"
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.name, p.description, p.clone, p.start_dt,
			   s.name AS strain_name, b.name AS breeder_name, z.name AS zone_name, z.id AS zone_id,
			   (SELECT ps.status
				FROM plant_status_log psl
				LEFT OUTER JOIN plant_status ps ON psl.status_id = ps.id
				WHERE psl.plant_id = p.id
				ORDER BY %s DESC
				LIMIT 1) AS current_status,
			   (SELECT ps.id
				FROM plant_status_log psl
				LEFT OUTER JOIN plant_status ps ON psl.status_id = ps.id
				WHERE psl.plant_id = p.id
				ORDER BY %s DESC
				LIMIT 1) AS status_id,
			   p.sensors, s.id, p.harvest_weight,
			   COALESCE(s.cycle_time, 0),
			   COALESCE(s.url, ''),
			   s.autoflower,
			   COALESCE(p.parent_plant_id, 0),
			   COALESCE(p2.name, '') AS parent_name
		FROM plant p
		LEFT OUTER JOIN plant p2 ON COALESCE(p.parent_plant_id, 0) = p2.id
		LEFT OUTER JOIN strain s ON p.strain_id = s.id
		LEFT OUTER JOIN breeder b ON b.id = s.breeder_id
		LEFT OUTER JOIN zones z ON p.zone_id = z.id
		WHERE p.id = $1`, orderByExpr, orderByExpr)

	var plantID uint
	var name, description, strainName, breederName, zoneName, status, sensors, strainURL, parentName string
	var isClone, autoflower bool
	var startDT time.Time
	var zoneID, statusID, strainID, cycleTime int
	var harvestWeight float64
	var parentID uint

	err := db.QueryRow(query, id).Scan(&plantID, &name, &description, &isClone, &startDT,
		&strainName, &breederName, &zoneName, &zoneID, &status, &statusID,
		&sensors, &strainID, &harvestWeight, &cycleTime, &strainURL,
		&autoflower, &parentID, &parentName)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query plant")
		return plant
	}
	startDT = startDT.Local()

	// Calculate current day and week
	diff := time.Now().Sub(startDT)
	currentDay := int(diff.Hours()/24) + 1
	currentWeek := int((diff.Hours() / 24 / 7) + 1)

	// --- Load related data via focused sub-functions ---
	sensorList := loadPlantSensors(db, plantID, zoneID)
	measurements := loadPlantMeasurements(db, plantID)
	activities := loadPlantActivities(db, plantID)
	statusHistory := loadPlantStatusHistory(db, plantID)
	latestImage, images := loadPlantImages(db, plantID)

	// --- Derive business-logic dates ---
	lastWaterDate, lastFeedDate := deriveActivityDates(activities)
	harvestDate := deriveHarvestDate(statusHistory)
	estHarvestDate := deriveEstimatedHarvestDate(statusHistory, startDT, cycleTime, autoflower)

	plant = types.Plant{
		ID: plantID, Name: name, Description: description, Status: status,
		StatusID: statusID, StrainName: strainName, StrainID: strainID,
		BreederName: breederName, ZoneName: zoneName, ZoneID: zoneID,
		CurrentDay: currentDay, CurrentWeek: currentWeek,
		CurrentHeight: strconv.Itoa(0), HeightDate: time.Time{},
		LastWaterDate: lastWaterDate, LastFeedDate: lastFeedDate,
		Measurements: measurements, Activities: activities,
		StatusHistory: statusHistory, Sensors: sensorList,
		LatestImage: latestImage, Images: images,
		IsClone: isClone, StartDT: startDT, HarvestWeight: harvestWeight,
		HarvestDate: harvestDate, CycleTime: cycleTime, StrainUrl: strainURL,
		EstHarvestDate: estHarvestDate, Autoflower: autoflower,
		ParentID: parentID, ParentName: parentName,
	}
	return plant
}

// loadPlantSensors fetches the linked sensor IDs from the plant's JSON column,
// merges in zone-inherited sensors (non-Soil sensors from the plant's zone),
// and returns each sensor's details with its latest reading.
// Directly-linked sensors take priority over zone-inherited ones (no duplicates).
// Uses batch queries to avoid per-sensor N+1 query overhead.
func loadPlantSensors(db *sql.DB, plantID uint, zoneID int) []types.SensorDataResponse {
	fieldLogger := logger.Log.WithField("func", "loadPlantSensors")

	// 1. Load directly-linked sensor IDs from the plant's JSON column
	var sensorsJSON string
	err := db.QueryRow("SELECT sensors FROM plant WHERE id = $1", plantID).Scan(&sensorsJSON)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query sensors JSON")
		return nil
	}

	var directIDs []int
	if err := json.Unmarshal([]byte(sensorsJSON), &directIDs); err != nil {
		fieldLogger.WithError(err).Error("Failed to deserialize sensor IDs")
		return nil
	}

	// Build a set of directly-linked IDs for deduplication
	directSet := make(map[int]bool, len(directIDs))
	for _, id := range directIDs {
		directSet[id] = true
	}

	// 2. Load zone-inherited sensor IDs
	var zoneIDs []int
	if zoneID > 0 {
		rows, err := db.Query(`SELECT id FROM sensors WHERE zone_id = $1 AND visibility IN ('zone_plant', 'plant') AND type NOT LIKE 'Soil.%'`, zoneID)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query zone sensors")
		} else {
			defer rows.Close()
			for rows.Next() {
				var sid int
				if err := rows.Scan(&sid); err != nil {
					continue
				}
				zoneIDs = append(zoneIDs, sid)
			}
		}
	}

	// 3. Collect all unique sensor IDs (direct + zone-inherited) for a single batch query
	allIDSet := make(map[int]struct{})
	for _, id := range directIDs {
		allIDSet[id] = struct{}{}
	}
	for _, id := range zoneIDs {
		allIDSet[id] = struct{}{}
	}

	if len(allIDSet) == 0 {
		return nil
	}

	// 4. Batch query: fetch sensor details + latest reading + timestamp for all sensors at once
	driver := model.GetDriver()
	uniqueIDs := make([]interface{}, 0, len(allIDSet))
	for sid := range allIDSet {
		uniqueIDs = append(uniqueIDs, sid)
	}
	inClause, inArgs := model.BuildInClause(driver, uniqueIDs)

	query := `
		SELECT s.id, s.name, s.unit, sd.value, sd.create_dt
		FROM sensors s
		LEFT JOIN sensor_data sd ON s.id = sd.sensor_id
			AND sd.id = (SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)
		WHERE s.id IN ` + inClause

	rows, err := db.Query(query, inArgs...)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to batch-fetch sensor details and readings")
		return nil
	}
	defer rows.Close()

	type sensorInfo struct {
		ID   uint
		Name string
		Unit string
		Val  sql.NullFloat64
		Date sql.NullTime
	}
	sensorMap := make(map[int]sensorInfo)
	for rows.Next() {
		var sid int
		var s sensorInfo
		if err := rows.Scan(&sid, &s.Name, &s.Unit, &s.Val, &s.Date); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan batch sensor row")
			continue
		}
		s.ID = uint(sid)
		sensorMap[sid] = s
	}

	// 5. Assemble the result: directly-linked first (preserving order), then zone-inherited
	var sensorList []types.SensorDataResponse

	for _, sensorID := range directIDs {
		s, ok := sensorMap[sensorID]
		if !ok {
			continue
		}
		resp := types.SensorDataResponse{
			ID:        s.ID,
			Name:      s.Name,
			Unit:      s.Unit,
			Inherited: false,
		}
		if s.Val.Valid {
			resp.Value = s.Val.Float64
		}
		if s.Date.Valid {
			resp.Date = s.Date.Time.Local()
		}
		sensorList = append(sensorList, resp)
	}

	for _, sensorID := range zoneIDs {
		if directSet[sensorID] {
			continue // already included as a direct link
		}
		s, ok := sensorMap[sensorID]
		if !ok {
			continue
		}
		resp := types.SensorDataResponse{
			ID:        s.ID,
			Name:      s.Name,
			Unit:      s.Unit,
			Inherited: true,
		}
		if s.Val.Valid {
			resp.Value = s.Val.Float64
		}
		if s.Date.Valid {
			resp.Date = s.Date.Time.Local()
		}
		sensorList = append(sensorList, resp)
	}

	return sensorList
}

// loadPlantMeasurements fetches the most recent measurements for a plant, ordered by date descending.
func loadPlantMeasurements(db *sql.DB, plantID uint) []types.Measurement {
	fieldLogger := logger.Log.WithField("func", "loadPlantMeasurements")

	rows, err := db.Query(fmt.Sprintf(`
		SELECT m.id, m.metric_id, me.name, m.value, m.date
		FROM plant_measurements m
		LEFT OUTER JOIN metric me ON me.id = m.metric_id
		WHERE m.plant_id = $1
		ORDER BY date DESC
		LIMIT %d`, PlantHistoryLimit), plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query measurements")
		return nil
	}
	defer rows.Close()

	var measurements []types.Measurement
	for rows.Next() {
		var id uint
		var metricID int
		var name string
		var value float64
		var date time.Time
		if err := rows.Scan(&id, &metricID, &name, &value, &date); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan measurement")
			continue
		}
		measurements = append(measurements, types.Measurement{ID: id, MetricID: metricID, Name: name, Value: value, Date: utils.AsLocal(date)})
	}
	return measurements
}

// loadPlantActivities fetches the most recent activities for a plant, ordered by date descending.
func loadPlantActivities(db *sql.DB, plantID uint) []types.PlantActivity {
	fieldLogger := logger.Log.WithField("func", "loadPlantActivities")

	// Use a CTE here to be able to limit the results to PlantHistoryLimit
	rows, err := db.Query(fmt.Sprintf(`
		WITH recent_activities AS (
			SELECT pa.id,
			       COALESCE(a.id, 0) AS activity_id,
			       COALESCE(a.name, '') AS name,
			       pa.note,
			       pa.date,
			       COALESCE(a.is_watering, false) AS is_watering,
			       COALESCE(a.is_feeding, false) AS is_feeding
			FROM plant_activity pa
			LEFT OUTER JOIN activity a ON a.id = pa.activity_id
			WHERE pa.plant_id = $1
			ORDER BY pa.date DESC
			LIMIT %d
		)
		SELECT ra.id,
		       ra.activity_id,
		       ra.name,
		       ra.note,
		       ra.date,
		       ra.is_watering,
		       ra.is_feeding,
		       pm.id,
		       pm.metric_id,
		       me.name,
		       pm.value,
		       pm.date
		FROM recent_activities ra
		LEFT OUTER JOIN plant_measurements pm ON pm.plant_activity_id = ra.id
		LEFT OUTER JOIN metric me ON me.id = pm.metric_id
		ORDER BY ra.date DESC, ra.id DESC, me.name`, PlantHistoryLimit), plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities")
		return nil
	}
	defer rows.Close()

	var activities []types.PlantActivity
	activityIdx := make(map[uint]int)
	for rows.Next() {
		var id uint
		var activityID int
		var name, note string
		var date time.Time
		var isWatering, isFeeding bool
		var measurementID sql.NullInt64
		var metricID sql.NullInt64
		var measurementName sql.NullString
		var measurementValue sql.NullFloat64
		var measurementDate sql.NullTime

		if err := rows.Scan(&id, &activityID, &name, &note, &date, &isWatering, &isFeeding, &measurementID, &metricID, &measurementName, &measurementValue, &measurementDate); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan activity")
			continue
		}

		i, exists := activityIdx[id]
		if !exists {
			activities = append(activities, types.PlantActivity{ID: id, Name: name, Note: note, Date: utils.AsLocal(date), ActivityId: activityID, IsWatering: isWatering, IsFeeding: isFeeding})
			i = len(activities) - 1
			activityIdx[id] = i
		}

		if measurementID.Valid {
			measurement := types.Measurement{
				ID:    uint(measurementID.Int64),
				Name:  measurementName.String,
				Value: measurementValue.Float64,
			}
			if metricID.Valid {
				measurement.MetricID = int(metricID.Int64)
			}
			if measurementDate.Valid {
				measurement.Date = utils.AsLocal(measurementDate.Time)
			}
			activities[i].Measurements = append(activities[i].Measurements, measurement)
		}
	}

	return activities
}

// loadPlantStatusHistory fetches the most recent status log entries for a plant, ordered by date descending.
func loadPlantStatusHistory(db *sql.DB, plantID uint) []types.Status {
	fieldLogger := logger.Log.WithField("func", "loadPlantStatusHistory")

	rows, err := db.Query(fmt.Sprintf("SELECT psl.id, ps.status, psl.date, psl.status_id FROM plant_status_log psl LEFT OUTER JOIN plant_status ps ON psl.status_id = ps.id WHERE psl.plant_id = $1 ORDER BY date DESC LIMIT %d", PlantHistoryLimit), plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query status history")
		return nil
	}
	defer rows.Close()

	var history []types.Status
	for rows.Next() {
		var id uint
		var status string
		var date time.Time
		var statusID int
		if err := rows.Scan(&id, &status, &date, &statusID); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan status history")
			continue
		}
		history = append(history, types.Status{ID: id, Status: status, Date: utils.AsLocal(date), StatusID: statusID})
	}
	return history
}

// loadPlantImages fetches the latest image and all images for a plant.
// Returns (latestImage, allImages).
func loadPlantImages(db *sql.DB, plantID uint) (types.PlantImage, []types.PlantImage) {
	fieldLogger := logger.Log.WithField("func", "loadPlantImages")

	// Latest image
	var latestImage types.PlantImage
	err := db.QueryRow("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date DESC LIMIT 1", plantID).Scan(
		&latestImage.ID, &latestImage.ImagePath, &latestImage.ImageDescription, &latestImage.ImageOrder, &latestImage.ImageDate)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query latest image")
		latestImage = types.PlantImage{
			ID: 0, PlantID: plantID, ImagePath: "/static/img/winston.hat.jpg",
			ImageDescription: "Placeholder", ImageOrder: DefaultPlantImageOrder,
			ImageDate: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
	} else {
		latestImage.ImagePath = "/" + strings.Replace(latestImage.ImagePath, "\\", "/", -1)
		latestImage.ImageDate = utils.AsLocal(latestImage.ImageDate)
	}

	// All images
	rows, err := db.Query("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date DESC", plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query images")
		return latestImage, nil
	}
	defer rows.Close()

	var images []types.PlantImage
	for rows.Next() {
		var id uint
		var imagePath, imageDescription string
		var imageOrder int
		var imageDate time.Time
		if err := rows.Scan(&id, &imagePath, &imageDescription, &imageOrder, &imageDate); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan image")
			continue
		}
		imagePath = "/" + strings.Replace(imagePath, "\\", "/", -1)
		images = append(images, types.PlantImage{
			ID: id, PlantID: plantID, ImagePath: imagePath,
			ImageDescription: imageDescription, ImageOrder: imageOrder,
			ImageDate: utils.AsLocal(imageDate), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
	}
	return latestImage, images
}

// deriveActivityDates scans activities to find the most recent water and feed dates.
func deriveActivityDates(activities []types.PlantActivity) (lastWater, lastFeed time.Time) {
	for _, a := range activities {
		if a.IsWatering && a.Date.After(lastWater) {
			lastWater = a.Date
		}
		if a.IsFeeding && a.Date.After(lastFeed) {
			lastFeed = a.Date
		}
	}
	return
}

// deriveHarvestDate finds the earliest terminal status date (Success, Dead, Curing, Drying).
func deriveHarvestDate(history []types.Status) time.Time {
	harvestDate := time.Now()
	for _, s := range history {
		switch s.Status {
		case "Success", "Dead", "Curing", "Drying":
			if s.Date.Before(harvestDate) {
				harvestDate = s.Date
			}
		}
	}
	return harvestDate
}

// deriveEstimatedHarvestDate calculates the estimated harvest date based on
// cycle time and strain type (autoflower vs photosensitive).
func deriveEstimatedHarvestDate(history []types.Status, startDT time.Time, cycleTime int, autoflower bool) time.Time {
	if cycleTime <= 0 {
		return time.Time{}
	}
	if autoflower {
		return startDT.AddDate(0, 0, cycleTime)
	}
	// Photosensitive: find earliest "Flower" status and add cycle_time
	var flowerDate time.Time
	found := false
	for _, s := range history {
		if s.Status == "Flower" {
			if !found || s.Date.Before(flowerDate) {
				flowerDate = s.Date
				found = true
			}
		}
	}
	if found {
		return flowerDate.AddDate(0, 0, cycleTime)
	}
	return time.Time{}
}

func LinkSensorsToPlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "LinkSensorsToPlant")
	var input struct {
		PlantID   string `json:"plant_id"`
		SensorIDs []int  `json:"sensor_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	// Initialize the database
	db := DBFromContext(c)

	// Validate that every supplied sensor ID actually exists. Without this
	// check the JSON column happily accepts dangling references that later
	// surface as silent NULL joins on the plant detail page.
	if len(input.SensorIDs) > 0 {
		placeholders := make([]string, len(input.SensorIDs))
		args := make([]interface{}, len(input.SensorIDs))
		for i, id := range input.SensorIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		var found int
		query := fmt.Sprintf("SELECT COUNT(*) FROM sensors WHERE id IN (%s)", strings.Join(placeholders, ","))
		if err := db.QueryRow(query, args...).Scan(&found); err != nil {
			fieldLogger.WithError(err).Error("Failed to validate sensor IDs")
			apiInternalError(c, "api_database_query_error")
			return
		}
		if found != len(input.SensorIDs) {
			fieldLogger.WithField("requested", len(input.SensorIDs)).WithField("found", found).Warn("Rejected link with unknown sensor IDs")
			apiBadRequest(c, "api_invalid_sensor_ids")
			return
		}
	}

	// Serialize SensorIDs to JSON
	sensorIDsJSON, err := json.Marshal(input.SensorIDs)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to serialize sensor IDs")
		apiInternalError(c, "api_failed_to_process_sensor_ids")
		return
	}

	// Update the plant with the serialized sensor IDs
	_, err = db.Exec("UPDATE plant SET sensors = $1 WHERE id = $2", sensorIDsJSON, input.PlantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update sensors")
		apiInternalError(c, "api_failed_to_update_sensors")
		return
	}

	apiOK(c, "api_sensors_linked")
}

func UpdatePlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdatePlant")
	var input struct {
		PlantID          int    `json:"plant_id"`
		PlantName        string `json:"plant_name"`
		PlantDescription string `json:"plant_description"`
		StatusID         int    `json:"status_id"`
		Date             string `json:"date"` // YYYY-MM-DD format
		ZoneID           *int   `json:"zone_id"`
		NewZone          string `json:"new_zone"`
		StrainID         *int   `json:"strain_id"`
		NewStrain        *struct {
			Name       string `json:"name"`
			BreederId  int    `json:"breeder_id"`
			NewBreeder string `json:"new_breeder"`
		} `json:"new_strain"`
		IsClone       bool    `json:"clone"`
		StartDT       string  `json:"start_date"`
		HarvestWeight float64 `json:"harvest_weight"`
	}

	// Bind JSON payload
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_input")
		return
	}
	// Validate string lengths
	if err := utils.ValidateStringLength("plant_name", input.PlantName, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("plant_description", input.PlantDescription, utils.MaxDescriptionLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("new_zone", input.NewZone, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Init the db
	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(db, store, input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			apiInternalError(c, "api_failed_to_create_new_zone")
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	// Handle new strain creation
	if input.StrainID == nil && input.NewStrain != nil {
		// Insert new strain into the database
		strainID, err := CreateNewStrain(db, store, input.NewStrain)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new strain")
			apiInternalError(c, "api_failed_to_create_new_strain")
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	isClone := 0
	if input.IsClone {
		isClone = 1
	}

	//Update the plant
	_, err := db.Exec("UPDATE plant SET name = $1, description = $2, zone_id = $3, strain_id = $4, clone = $5, start_dt = $6, harvest_weight = $7 WHERE id = $8", input.PlantName, input.PlantDescription, input.ZoneID, input.StrainID, isClone, input.StartDT, input.HarvestWeight, input.PlantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update plant")
		return
	}

	// Only update the Plant Status Log if a status was provided in the request
	if input.StatusID != 0 {
		updated, _, err := updatePlantStatusLog(db, input.PlantID, input.StatusID, input.Date)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to update plant status")
			return
		}
		if !updated {
			fieldLogger.Info("Plant status unchanged")
		}
	}
	c.JSON(http.StatusCreated, input)
}

func getPlantsByStatus(db *sql.DB, statuses []int) ([]types.PlantListResponse, error) {
	fieldLogger := logger.Log.WithField("func", "getPlantsByStatus")

	driver := model.GetDriver()
	statusVals := make([]interface{}, len(statuses))
	for i, s := range statuses {
		statusVals[i] = s
	}
	inClause, args := model.BuildInClause(driver, statusVals)

	// Use the dynamic IN clause in the query
	var query string

	if model.IsPostgres() {
		query = `
SELECT 
    p.id, p.name, p.description, p.clone, 
    s.name AS strain_name, b.name AS breeder_name, z.name AS zone_name,
    p.start_dt,
    (((CURRENT_DATE - p.start_dt::date)) / 7 + 1) AS current_week,
	((CURRENT_DATE - p.start_dt::date) + 1) AS current_day,
	COALESCE((
		SELECT (CURRENT_DATE - MAX(pa.date)::date)
		FROM plant_activity pa
		JOIN activity a ON pa.activity_id = a.id
		WHERE pa.plant_id = p.id AND a.is_watering = TRUE
	), 0) AS days_since_last_watering,
	COALESCE((
		SELECT (CURRENT_DATE - MAX(pa.date)::date)
		FROM plant_activity pa
		JOIN activity a ON pa.activity_id = a.id
		WHERE pa.plant_id = p.id AND a.is_feeding = TRUE
	), 0) AS days_since_last_feeding,
	COALESCE((
		SELECT (
			LEAST(
				COALESCE((
					SELECT MIN(h2.date)::date FROM plant_status_log h2
					WHERE h2.plant_id = p.id
					  AND h2.status_id IN (SELECT id FROM plant_status WHERE status IN ('Drying','Curing','Success','Dead'))
					  AND h2.date > MAX(plant_status_log.date)
				), CURRENT_DATE),
				CURRENT_DATE
			) - MAX(date)::date
		)
		FROM plant_status_log
		WHERE plant_id = p.id
		  AND status_id = (SELECT id FROM plant_status WHERE status = 'Flower')
	), 0) AS flowering_days,
    p.harvest_weight, ps.status, psl.date as status_date,
    COALESCE(s.cycle_time, 0), 
    COALESCE(s.url, '') AS strain_url, 
    s.autoflower,
    COALESCE((
        SELECT MIN(h.date)
        FROM plant_status_log h
        WHERE h.plant_id = p.id
          AND h.status_id IN (
            SELECT id FROM plant_status 
            WHERE status IN ('Drying','Curing','Success','Dead')
        )
    ), CURRENT_DATE) AS harvest_date
FROM plant p
JOIN strain s ON p.strain_id = s.id
JOIN breeder b ON s.breeder_id = b.id
LEFT JOIN zones z ON p.zone_id = z.id
JOIN plant_status_log psl ON p.id = psl.plant_id
JOIN plant_status ps ON psl.status_id = ps.id
WHERE ps.id IN ` + inClause + `
  AND psl.date = (SELECT MAX(date) FROM plant_status_log WHERE plant_id = p.id)
ORDER BY p.start_dt, p.name;
`
	} else {
		query = `
SELECT 
    p.id, p.name, p.description, p.clone, 
    s.name AS strain_name, b.name AS breeder_name, z.name AS zone_name,
    p.start_dt,
    CAST((julianday('now', 'localtime') - julianday(p.start_dt)) / 7 + 1 AS INT) AS current_week,
    CAST((julianday('now', 'localtime') - julianday(p.start_dt)) + 1 AS INT) AS current_day,
    COALESCE((
        SELECT CAST(julianday('now', 'localtime') - julianday(MAX(pa.date)) AS INT)
        FROM plant_activity pa
		JOIN activity a ON pa.activity_id = a.id
		WHERE pa.plant_id = p.id AND a.is_watering = TRUE
    ), 0) AS days_since_last_watering,
    COALESCE((
        SELECT CAST(julianday('now', 'localtime') - julianday(MAX(pa.date)) AS INT)
        FROM plant_activity pa
		JOIN activity a ON pa.activity_id = a.id
		WHERE pa.plant_id = p.id AND a.is_feeding = TRUE
    ), 0) AS days_since_last_feeding,
    COALESCE((
        SELECT CAST(
            julianday(
                MIN(
                    COALESCE((
                        SELECT MIN(h2.date) FROM plant_status_log h2
                        WHERE h2.plant_id = p.id
                          AND h2.status_id IN (SELECT id FROM plant_status WHERE status IN ('Drying','Curing','Success','Dead'))
                          AND h2.date > MAX(plant_status_log.date)
                    ), datetime('now', 'localtime')),
                    datetime('now', 'localtime')
                )
            ) - julianday(MAX(date)) AS INT)
        FROM plant_status_log
        WHERE plant_id = p.id AND status_id = (
            SELECT id FROM plant_status WHERE status = 'Flower'
        )
    ), 0) AS flowering_days,
    p.harvest_weight, ps.status, psl.date as status_date,
    COALESCE(s.cycle_time, 0), 
    COALESCE(s.url, '') AS strain_url, 
    s.autoflower,
    COALESCE((
        SELECT MIN(h.date)
        FROM plant_status_log h
        WHERE h.plant_id = p.id
          AND h.status_id IN (
            SELECT id FROM plant_status 
            WHERE status IN ('Drying','Curing','Success','Dead')
        )
    ), DATE('now', 'localtime')) AS harvest_date
FROM plant p
JOIN strain s ON p.strain_id = s.id
JOIN breeder b ON s.breeder_id = b.id
LEFT JOIN zones z ON p.zone_id = z.id
JOIN plant_status_log psl ON p.id = psl.plant_id
JOIN plant_status ps ON psl.status_id = ps.id
WHERE ps.id IN ` + inClause + `
  AND psl.date = (SELECT MAX(date) FROM plant_status_log WHERE plant_id = p.id)
ORDER BY p.start_dt, p.name;
`
	}

	// Execute the query
	rows, err := db.Query(query, args...)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query plants")
		return nil, err
	}

	plants := []types.PlantListResponse{}
	for rows.Next() {
		var plant types.PlantListResponse

		var harvestDateStr string
		if err := rows.Scan(&plant.ID, &plant.Name, &plant.Description, &plant.Clone, &plant.StrainName, &plant.BreederName, &plant.ZoneName, &plant.StartDT, &plant.CurrentWeek, &plant.CurrentDay, &plant.DaysSinceLastWatering, &plant.DaysSinceLastFeeding, &plant.FloweringDays, &plant.HarvestWeight, &plant.Status, &plant.StatusDate, &plant.CycleTime, &plant.StrainUrl, &plant.Autoflower, &harvestDateStr); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan plant")
			return nil, err
		}

		// Parse the date string into time.Time
		//If harvestDateStr contains T it has a time component, otherwise it's just a date, parse it accordingly
		if strings.Contains(harvestDateStr, "T") {
			plant.HarvestDate, err = time.Parse(time.RFC3339, harvestDateStr)
			if err != nil {
				// SQLite returns datetime without timezone info, try LayoutDateTimeLocal
				plant.HarvestDate, err = time.ParseInLocation(utils.LayoutDateTimeLocal, harvestDateStr, time.Local)
			}
		} else {
			plant.HarvestDate, err = time.Parse(utils.LayoutDate, harvestDateStr)
		}
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to parse harvest date")
			return nil, err
		}
		// calculate the estimated harvest date
		startDate := plant.StartDT
		//convert start date to a time.Time (has timezone data)
		startTime, err := time.Parse(time.RFC3339, startDate)
		if err != nil {
			startTime, err = time.ParseInLocation(utils.LayoutDateTimeLocal, startDate, time.Local)
		}
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to parse start date")
		} else {
			estHarvestDate := startTime.AddDate(0, 0, plant.CycleTime)
			plant.EstHarvestDate = estHarvestDate
		}
		plants = append(plants, plant)
	}

	return plants, nil
}

func GetLivingPlants(db *sql.DB) []types.PlantListResponse {
	//Load status ids from database where active = 1
	rows, err := db.Query("SELECT id FROM plant_status WHERE active = 1")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to query plant statuses")
		return nil
	}
	defer rows.Close()

	var statuses []int
	for rows.Next() {
		var status int
		if err := rows.Scan(&status); err != nil {
			logger.Log.WithError(err).Error("Failed to scan status")
			return nil
		}
		statuses = append(statuses, status)
	}

	result, _ := getPlantsByStatus(db, statuses)
	return result
}

// LivingPlantsHandler handles the /plants/living endpoint.
func LivingPlantsHandler(c *gin.Context) {
	db := DBFromContext(c)
	plants := GetLivingPlants(db)
	c.JSON(http.StatusOK, plants)
}

// HarvestedPlantsHandler handles the /plants/harvested endpoint.
func HarvestedPlantsHandler(c *gin.Context) {
	db := DBFromContext(c)
	rows, err := db.Query("SELECT id FROM plant_status WHERE active = 0 and status <> 'Dead'")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to query plant statuses")
		return
	}
	defer rows.Close()

	var statuses []int
	for rows.Next() {
		var status int
		if err := rows.Scan(&status); err != nil {
			logger.Log.WithError(err).Error("Failed to scan status")
			return
		}
		statuses = append(statuses, status)
	}

	plants, err := getPlantsByStatus(db, statuses)
	if err != nil {
		apiInternalError(c, "api_failed_to_retrieve_plants")
		return
	}

	c.JSON(http.StatusOK, plants)
}

// DeadPlantsHandler handles the /plants/dead endpoint.
func DeadPlantsHandler(c *gin.Context) {
	db := DBFromContext(c)

	rows, err := db.Query("SELECT id FROM plant_status WHERE status = 'Dead'")
	if err != nil {
		logger.Log.WithError(err).Error("Failed to query plant statuses")
		return
	}
	defer rows.Close()

	var statuses []int
	for rows.Next() {
		var status int
		if err := rows.Scan(&status); err != nil {
			logger.Log.WithError(err).Error("Failed to scan status")
			return
		}
		statuses = append(statuses, status)
	}
	plants, err := getPlantsByStatus(db, statuses)
	if err != nil {
		apiInternalError(c, "api_failed_to_retrieve_plants")
		return
	}

	c.JSON(http.StatusOK, plants)
}

// GetAdjacentPlantIDs returns the prev and next plant IDs within the same status group,
// ordered by start_dt then name. Returns 0, 0 when there is no adjacent plant.
func GetAdjacentPlantIDs(db *sql.DB, currentID int) (prevID, nextID int) {
	fieldLogger := logger.Log.WithField("func", "GetAdjacentPlantIDs")

	// Partition plants into status groups (active/not active),
	// then use LAG/LEAD to get the neighbors of the requested plant.
	const query = `
		WITH ranked AS (
			SELECT
				p.id,
				LAG(p.id)  OVER (PARTITION BY ps.active ORDER BY p.start_dt, p.name) AS prev_id,
				LEAD(p.id) OVER (PARTITION BY ps.active ORDER BY p.start_dt, p.name) AS next_id
			FROM plant p
			JOIN plant_status_log psl ON p.id = psl.plant_id
			JOIN plant_status ps      ON psl.status_id = ps.id
			WHERE psl.date = (SELECT MAX(date) FROM plant_status_log WHERE plant_id = p.id)
		)
		SELECT COALESCE(prev_id, 0), COALESCE(next_id, 0)
		FROM   ranked
		WHERE  id = $1
	`

	if err := db.QueryRow(query, currentID).Scan(&prevID, &nextID); err != nil {
		if err != sql.ErrNoRows {
			fieldLogger.WithError(err).Error("Failed to query adjacent plant IDs")
		}
		return 0, 0
	}
	return prevID, nextID
}

// Strain and breeder handlers have been moved to strain.go
