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
		apiBadRequest(c, "Invalid input")
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

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
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
		strainID, err := CreateNewStrain(input.NewStrain)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new strain")
			apiInternalError(c, "api_failed_to_create_new_strain")
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	// Insert plant, decrement seed count, and create initial status log
	// inside a transaction so partial writes cannot occur.
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		apiInternalError(c, "Failed to create plant")
		return
	}
	defer tx.Rollback() // no-op after Commit

	plantID := 0
	err = tx.QueryRow("INSERT INTO plant (name, zone_id, strain_id, description, clone, parent_plant_id, start_dt, sensors) VALUES ($1, $2, $3, '', $4, NULLIF($5, 0), $6, '[]') RETURNING id", input.Name, *input.ZoneID, *input.StrainID, input.Clone, input.ParentID, input.Date).Scan(&plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant")
		apiInternalError(c, "Failed to create plant")
		return
	} else if plantID == 0 {
		fieldLogger.Error("Failed to retrieve plant ID")
		apiInternalError(c, "Failed to create plant")
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
			apiInternalError(c, "Failed to create plant")
			return
		}
	}

	if _, err := tx.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)", plantID, input.StatusID, input.Date); err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant status log")
		apiInternalError(c, "Failed to create plant")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit transaction")
		apiInternalError(c, "Failed to create plant")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": plantID, "message": T(c, "api_plant_added")})
}

func GetActivities() []types.Activity {
	fieldLogger := logger.Log.WithField("func", "GetActivities")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil
	}

	rows, err := db.Query("SELECT id, name FROM activity")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities")
		return nil
	}

	var activities []types.Activity
	for rows.Next() {
		var activity types.Activity
		err = rows.Scan(&activity.ID, &activity.Name)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan activity")
			return nil
		}
		activities = append(activities, activity)
	}

	return activities
}

func GetMetrics() []types.Metric {
	fieldLogger := logger.Log.WithField("func", "GetMetrics")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil
	}

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

func GetStatuses() []types.Status {
	fieldLogger := logger.Log.WithField("func", "GetStatuses")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil
	}

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

func CreateNewStrain(newStrain *struct {
	Name       string `json:"name"`
	BreederId  int    `json:"breeder_id"`
	NewBreeder string `json:"new_breeder"`
}) (int, error) {
	fieldLogger := logger.Log.WithField("func", "CreateNewStrain")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return 0, err
	}
	var breederId int

	// Check if a new breeder needs to be added
	if newStrain.BreederId == 0 && newStrain.NewBreeder != "" {
		// Insert the new breeder into the `breeder` table
		err := db.QueryRow("INSERT INTO breeder (name) VALUES ($1) RETURNING id", newStrain.NewBreeder).Scan(&breederId)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert new breeder")
			return 0, fmt.Errorf("failed to insert new breeder: %w", err)
		}

		config.Breeders = GetBreeders()
	} else {
		// Use the existing breeder ID
		breederId = newStrain.BreederId
	}

	// Insert the new strain into the `strain` table
	var id int
	// Use numeric autoflower flag to be consistent with other handlers
	autoflowerInt := 1 // default true
	err = db.QueryRow(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		newStrain.Name, breederId, 50, 50, autoflowerInt, "", 0).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert new strain")
		return 0, fmt.Errorf("failed to insert new strain: %w", err)
	}

	config.Strains = GetStrains()

	return id, nil
}

func DeletePlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeletePlant")
	id := c.Param("id")

	err := DeletePlantById(id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant")
		apiInternalError(c, "api_failed_to_delete_plant")
		return
	}

	apiOK(c, "api_plant_deleted")
}

func DeletePlantById(id string) error {
	fieldLogger := logger.Log.WithField("func", "DeletePlantById")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return err
	}

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

func GetPlant(id string) types.Plant {
	fieldLogger := logger.Log.WithField("func", "GetPlant")
	var plant types.Plant

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return plant
	}

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

	err = db.QueryRow(query, id).Scan(&plantID, &name, &description, &isClone, &startDT,
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

	// 2. Load zone-inherited sensors: non-Soil, visible sensors from the plant's zone
	var zoneSensors []struct {
		ID   int
		Name string
		Unit string
	}
	if zoneID > 0 {
		rows, err := db.Query(`SELECT id, name, unit FROM sensors WHERE zone_id = $1 AND visibility IN ('zone_plant', 'plant') AND type NOT LIKE 'Soil.%'`, zoneID)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query zone sensors")
		} else {
			defer rows.Close()
			for rows.Next() {
				var s struct {
					ID   int
					Name string
					Unit string
				}
				if err := rows.Scan(&s.ID, &s.Name, &s.Unit); err != nil {
					fieldLogger.WithError(err).Error("Failed to scan zone sensor")
					continue
				}
				zoneSensors = append(zoneSensors, s)
			}
		}
	}

	// Helper to fetch latest reading for a sensor
	fetchLatest := func(sensorID int) (float64, time.Time, bool) {
		var sd types.SensorData
		err := db.QueryRow("SELECT id, value, create_dt FROM sensor_data WHERE sensor_id = $1 ORDER BY create_dt DESC LIMIT 1", sensorID).Scan(&sd.ID, &sd.Value, &sd.CreateDT)
		if err != nil {
			return 0, time.Time{}, false
		}
		return sd.Value, sd.CreateDT.Local(), true
	}

	// 3. Build the merged sensor list: directly-linked first
	var sensorList []types.SensorDataResponse
	for _, sensorID := range directIDs {
		var sensor types.SensorDataResponse
		err := db.QueryRow("SELECT id, name, unit FROM sensors WHERE id = $1", sensorID).Scan(&sensor.ID, &sensor.Name, &sensor.Unit)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query sensor details")
			continue
		}
		if val, dt, ok := fetchLatest(sensorID); ok {
			sensor.Value = val
			sensor.Date = dt
		}
		sensor.Inherited = false
		sensorList = append(sensorList, sensor)
	}

	// 4. Append zone-inherited sensors that aren't already directly linked
	for _, zs := range zoneSensors {
		if directSet[zs.ID] {
			continue // already included as a direct link
		}
		sensor := types.SensorDataResponse{
			ID:        uint(zs.ID),
			Name:      zs.Name,
			Unit:      zs.Unit,
			Inherited: true,
		}
		if val, dt, ok := fetchLatest(zs.ID); ok {
			sensor.Value = val
			sensor.Date = dt
		}
		sensorList = append(sensorList, sensor)
	}

	return sensorList
}

// loadPlantMeasurements fetches the most recent measurements for a plant, ordered by date descending.
func loadPlantMeasurements(db *sql.DB, plantID uint) []types.Measurement {
	fieldLogger := logger.Log.WithField("func", "loadPlantMeasurements")

	rows, err := db.Query("SELECT m.id, me.name, m.value, m.date FROM plant_measurements m LEFT OUTER JOIN metric me ON me.id = m.metric_id WHERE m.plant_id = $1 ORDER BY date DESC LIMIT 500", plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query measurements")
		return nil
	}
	defer rows.Close()

	var measurements []types.Measurement
	for rows.Next() {
		var id uint
		var name string
		var value float64
		var date time.Time
		if err := rows.Scan(&id, &name, &value, &date); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan measurement")
			continue
		}
		measurements = append(measurements, types.Measurement{ID: id, Name: name, Value: value, Date: utils.AsLocal(date)})
	}
	return measurements
}

// loadPlantActivities fetches the most recent activities for a plant, ordered by date descending.
func loadPlantActivities(db *sql.DB, plantID uint) []types.PlantActivity {
	fieldLogger := logger.Log.WithField("func", "loadPlantActivities")

	rows, err := db.Query("SELECT pa.id, a.id AS activity_id, a.name, pa.note, pa.date FROM plant_activity pa LEFT OUTER JOIN activity a ON a.id = pa.activity_id WHERE pa.plant_id = $1 ORDER BY date DESC LIMIT 500", plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query activities")
		return nil
	}
	defer rows.Close()

	var activities []types.PlantActivity
	for rows.Next() {
		var id uint
		var activityID int
		var name, note string
		var date time.Time
		if err := rows.Scan(&id, &activityID, &name, &note, &date); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan activity")
			continue
		}
		activities = append(activities, types.PlantActivity{ID: id, Name: name, Note: note, Date: utils.AsLocal(date), ActivityId: activityID})
	}
	return activities
}

// loadPlantStatusHistory fetches the most recent status log entries for a plant, ordered by date descending.
func loadPlantStatusHistory(db *sql.DB, plantID uint) []types.Status {
	fieldLogger := logger.Log.WithField("func", "loadPlantStatusHistory")

	rows, err := db.Query("SELECT psl.id, ps.status, psl.date, psl.status_id FROM plant_status_log psl LEFT OUTER JOIN plant_status ps ON psl.status_id = ps.id WHERE psl.plant_id = $1 ORDER BY date DESC LIMIT 500", plantID)
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
			ImageDescription: "Placeholder", ImageOrder: 100,
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
		if a.ActivityId == 1 && a.Date.After(lastWater) {
			lastWater = a.Date
		}
		if a.ActivityId == 2 && a.Date.After(lastFeed) {
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
		apiBadRequest(c, "Invalid input")
		return
	}

	// Serialize SensorIDs to JSON
	sensorIDsJSON, err := json.Marshal(input.SensorIDs)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to serialize sensor IDs")
		apiInternalError(c, "api_failed_to_process_sensor_ids")
		return
	}

	// Initialize the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "api_failed_to_connect_db")
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
		apiBadRequest(c, "Invalid input")
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
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "Database error")
		return
	}

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
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
		strainID, err := CreateNewStrain(input.NewStrain)
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
	_, err = db.Exec("UPDATE plant SET name = $1, description = $2, zone_id = $3, strain_id = $4, clone = $5, start_dt = $6, harvest_weight = $7 WHERE id = $8", input.PlantName, input.PlantDescription, input.ZoneID, input.StrainID, isClone, input.StartDT, input.HarvestWeight, input.PlantID)
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

func getPlantsByStatus(statuses []int) ([]types.PlantListResponse, error) {
	fieldLogger := logger.Log.WithField("func", "getPlantsByStatus")
	// Generate placeholders for the number of statuses
	placeholders := make([]string, len(statuses))
	args := make([]interface{}, len(statuses))
	for i, status := range statuses {
		placeholders[i] = "?"
		args[i] = status
	}

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
		WHERE pa.plant_id = p.id AND a.name = 'Water'
	), 0) AS days_since_last_watering,
	COALESCE((
		SELECT (CURRENT_DATE - MAX(pa.date)::date)
		FROM plant_activity pa
		JOIN activity a ON pa.activity_id = a.id
		WHERE pa.plant_id = p.id AND a.name = 'Feed'
	), 0) AS days_since_last_feeding,
	COALESCE((
		SELECT (CURRENT_DATE - MAX(date)::date)
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
        WHERE pa.plant_id = p.id AND a.name = 'Water'
    ), 0) AS days_since_last_watering,
    COALESCE((
        SELECT CAST(julianday('now', 'localtime') - julianday(MAX(pa.date)) AS INT)
        FROM plant_activity pa 
        JOIN activity a ON pa.activity_id = a.id
        WHERE pa.plant_id = p.id AND a.name = 'Feed'
    ), 0) AS days_since_last_feeding,
    COALESCE((
        SELECT CAST(julianday('now', 'localtime') - julianday(MAX(date)) AS INT)
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

	// Open the database connection
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil, err
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

func GetLivingPlants() []types.PlantListResponse {
	//Load status ids from database where active = 1
	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return nil
	}
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

	result, _ := getPlantsByStatus(statuses)
	return result
}

// LivingPlantsHandler handles the /plants/living endpoint.
func LivingPlantsHandler(c *gin.Context) {
	plants := GetLivingPlants()
	c.JSON(http.StatusOK, plants)
}

// HarvestedPlantsHandler handles the /plants/harvested endpoint.
func HarvestedPlantsHandler(c *gin.Context) {
	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return
	}
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

	plants, err := getPlantsByStatus(statuses)
	if err != nil {
		apiInternalError(c, "api_failed_to_retrieve_plants")
		return
	}

	c.JSON(http.StatusOK, plants)
}

// DeadPlantsHandler handles the /plants/dead endpoint.
func DeadPlantsHandler(c *gin.Context) {
	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Error("Failed to open database")
		return
	}

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
	plants, err := getPlantsByStatus(statuses)
	if err != nil {
		apiInternalError(c, "api_failed_to_retrieve_plants")
		return
	}

	c.JSON(http.StatusOK, plants)
}

// Strain and breeder handlers have been moved to strain.go
