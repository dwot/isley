package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"isley/config"
	"isley/logger"
	model "isley/model"
	"isley/model/types"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetBreeders() []types.Breeder {
	fieldLogger := logger.Log.WithField("func", "GetBreeders")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new strain"})
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	// Insert the new plant
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return
	}

	//Insert into the plants table returning id
	plantID := 0
	err = db.QueryRow("INSERT INTO plant (name, zone_id, strain_id, description, clone, parent_plant_id, start_dt, sensors) VALUES ($1, $2, $3, '', $4, $5, $6, '[]') RETURNING id", input.Name, *input.ZoneID, *input.StrainID, input.Clone, input.ParentID, input.Date).Scan(&plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant")
		return
	} else if plantID == 0 {
		fieldLogger.Error("Failed to retrieve plant ID")
		return
	}

	//If decrement seed count, lower seed count on strain by 1, min 0
	if input.DecrementSeedCount {
		var query string
		if model.IsPostgres() {
			query = "UPDATE strain SET seed_count = GREATEST(0, seed_count - 1) WHERE id = $1"
		} else {
			query = "UPDATE strain SET seed_count = MAX(0, seed_count - 1) WHERE id = $1"
		}

		_, err := db.Exec(query, *input.StrainID)

		if err != nil {
			fieldLogger.WithError(err).Error("Failed to decrement seed count")
			return
		}
	}

	_, err = db.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3)", plantID, input.StatusID, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert plant status log")
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": plantID, "message": "Plant added successfully"})
}

func GetStrains() []types.Strain {
	fieldLogger := logger.Log.WithField("func", "GetStrains")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil
	}

	rows, err := db.Query("SELECT s.id, s.name, b.id as breeder_id, b.name as breeder, s.indica, s.sativa, s.autoflower, s.description, coalesce(s.short_desc, ''), s.seed_count FROM strain s left outer join breeder b on s.breeder_id = b.id ORDER BY s.name ASC")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query strains")
		return nil
	}

	var strains []types.Strain
	for rows.Next() {
		var strain types.Strain
		err = rows.Scan(&strain.ID, &strain.Name, &strain.BreederID, &strain.Breeder, &strain.Indica, &strain.Sativa, &strain.Autoflower, &strain.Description, &strain.ShortDescription, &strain.SeedCount)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan strain")
			return nil
		}
		strains = append(strains, strain)
	}

	return strains
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete plant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Plant deleted successfully"})
}

func DeletePlantById(id string) error {
	fieldLogger := logger.Log.WithField("func", "DeletePlantById")
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return err
	}

	// Delete the plant's images
	_, err = db.Exec("DELETE FROM plant_images WHERE plant_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant images")
		return err
	}

	// Delete the plant's measurements
	_, err = db.Exec("DELETE FROM plant_measurements WHERE plant_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant measurements")
		return err
	}

	// Delete the plant's activities
	_, err = db.Exec("DELETE FROM plant_activity WHERE plant_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant activities")
		return err
	}

	// Delete the plant's status log
	_, err = db.Exec("DELETE FROM plant_status_log WHERE plant_id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant status log")
		return err
	}

	// Delete the plant
	_, err = db.Exec("DELETE FROM plant WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete plant")
		return err
	}
	return nil
}

func GetPlant(id string) types.Plant {
	fieldLogger := logger.Log.WithField("func", "GetPlant")
	var plant types.Plant
	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return plant
	}
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

	rows, err := db.Query(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query plant")
		return plant
	}

	// Iterate over rows
	for rows.Next() {
		var id uint
		var name string
		var description string
		var isClone bool
		var start_dt time.Time
		var strain_name string
		var breeder_name string
		var zone_name string
		var zoneID int
		var status string
		var statusID int
		var sensors string
		var strain_id int
		var harvest_weight float64
		var cycle_time int
		var strain_url string
		var autoflower bool
		var parent_id uint
		var parent_name string
		err = rows.Scan(&id, &name, &description, &isClone, &start_dt, &strain_name, &breeder_name, &zone_name, &zoneID, &status, &statusID, &sensors, &strain_id, &harvest_weight, &cycle_time, &strain_url, &autoflower, &parent_id, &parent_name)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to scan plant")
			return plant
		}
		// Calculate current day and week
		currentTime := time.Now().In(time.Local)
		//Calculate the # of hours difference between the current timezone and UTC
		diff := currentTime.Sub(start_dt)
		iCurrentDay := int(diff.Hours()/24) + 1
		iCurrentWeek := int((diff.Hours() / 24 / 7) + 1)

		//convert sensors string into list and Iterate over sensors and load sensor and latest sensor_data
		var sensorList []types.SensorDataResponse

		// Retrieve the serialized sensors column from the plant table
		var sensorsJSON string
		err := db.QueryRow("SELECT sensors FROM plant WHERE id = $1", id).Scan(&sensorsJSON)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query sensors JSON")
			return plant
		}

		// Deserialize the JSON data into a slice of integers
		var sensorIDs []int
		err = json.Unmarshal([]byte(sensorsJSON), &sensorIDs)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to deserialize sensor IDs")
			return plant
		}

		// Loop through each sensor ID and fetch details
		for _, sensorID := range sensorIDs {
			var sensor types.SensorDataResponse

			// Query sensor details from the sensors table
			err := db.QueryRow("SELECT id, name, unit FROM sensors WHERE id = $1", sensorID).Scan(&sensor.ID, &sensor.Name, &sensor.Unit)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to query sensor details")
				continue
			}

			// Query the latest sensor data from the sensor_data table
			var sensorData types.SensorData
			err = db.QueryRow("SELECT id, value, create_dt FROM sensor_data WHERE sensor_id = $1 ORDER BY create_dt DESC LIMIT 1", sensorID).Scan(&sensorData.ID, &sensorData.Value, &sensorData.CreateDT)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to query sensor data")
				continue
			}

			// Combine sensor details with the latest data
			sensor.Value = sensorData.Value
			sensor.Date = sensorData.CreateDT

			// Add the sensor to the sensor list
			sensorList = append(sensorList, sensor)
		}

		//Load measurements
		rows2, err := db.Query("SELECT m.id, me.name, m.value, m.date FROM plant_measurements m left outer join metric me on me.id = m.metric_id WHERE m.plant_id = $1 ORDER BY date desc", id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query measurements")
		}
		var measurements []types.Measurement
		for rows2.Next() {
			var id uint
			var name string
			var value float64
			var date time.Time
			err = rows2.Scan(&id, &name, &value, &date)
			if err != nil {
				fmt.Println(err)
			}
			measurements = append(measurements, types.Measurement{id, name, value, date})
		}

		//Load activities
		rows3, err := db.Query("SELECT pa.id, a.id as activity_id, a.name, pa.note, pa.date FROM plant_activity pa left outer join activity a on a.id = pa.activity_id WHERE pa.plant_id = $1 ORDER BY date desc", id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query activities")
		}
		var activities []types.PlantActivity
		for rows3.Next() {
			var id uint
			var name string
			var note string
			var date time.Time
			var activityId int
			err = rows3.Scan(&id, &activityId, &name, &note, &date)
			if err != nil {
				fmt.Println(err)
			}
			activities = append(activities, types.PlantActivity{id, name, note, date, activityId})
		}

		//Load status history
		rows5, err := db.Query("SELECT psl.id, ps.status, psl.date FROM plant_status_log psl left outer join plant_status ps on psl.status_id = ps.id WHERE psl.plant_id = $1 ORDER BY date desc", id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query status history")
		}
		var statusHistory []types.Status
		for rows5.Next() {
			var id uint
			var status string
			var date time.Time
			err = rows5.Scan(&id, &status, &date)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to scan status history")
			}
			statusHistory = append(statusHistory, types.Status{id, status, date})
		}

		//Load latest image
		var latestImage types.PlantImage
		err = db.QueryRow("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date DESC LIMIT 1", id).Scan(&latestImage.ID, &latestImage.ImagePath, &latestImage.ImageDescription, &latestImage.ImageOrder, &latestImage.ImageDate)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query latest image")
			latestImage = types.PlantImage{ID: 0, PlantID: plant.ID, ImagePath: "/static/img/winston.hat.jpg", ImageDescription: "Placeholder", ImageOrder: 100, ImageDate: time.Now().In(time.Local), CreatedAt: time.Now().In(time.Local), UpdatedAt: time.Now().In(time.Local)}
		} else {
			latestImage.ImagePath = "/" + strings.Replace(latestImage.ImagePath, "\\", "/", -1)
		}

		//Load images
		rows6, err := db.Query("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date desc", id)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to query images")
		}
		var images []types.PlantImage
		for rows6.Next() {
			var id uint
			var image_path string
			var image_description string
			var image_order int
			var image_date time.Time
			err = rows6.Scan(&id, &image_path, &image_description, &image_order, &image_date)
			if err != nil {
				fieldLogger.WithError(err).Error("Failed to scan images")
			}
			//Convert any \ in image_path to /
			image_path = "/" + strings.Replace(image_path, "\\", "/", -1)
			images = append(images, types.PlantImage{ID: id, PlantID: plant.ID, ImagePath: image_path, ImageDescription: image_description, ImageOrder: image_order, ImageDate: image_date, CreatedAt: time.Now().In(time.Local), UpdatedAt: time.Now().In(time.Local)})
		}

		iCurrentHeight := 0
		//initialize the height date to a time in the past Jan 1, 1970
		heightDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		lastWaterDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		lastFeedDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local)
		harvestDate := time.Now().In(time.Local)
		estHarvestDate := time.Now().In(time.Local)

		//iterate measurements to find the last height
		for _, measurement := range measurements {
			if measurement.Name == "Height" {
				if measurement.Date.After(heightDate) {
					heightDate = measurement.Date
					iCurrentHeight = int(measurement.Value)
				}
			}
		}

		//iterate activities to find the last water and feed dates
		for _, activity := range activities {
			if activity.ActivityId == 1 {
				if activity.Date.After(lastWaterDate) {
					lastWaterDate = activity.Date
				}
			}
			if activity.ActivityId == 2 {
				if activity.Date.After(lastFeedDate) {
					lastFeedDate = activity.Date
				}
			}
		}

		//iterate status history to find the last harvest date
		for _, status := range statusHistory {
			if status.Status == "Success" {
				if status.Date.Before(harvestDate) {
					harvestDate = status.Date
				}
			}
			if status.Status == "Dead" {
				if status.Date.Before(harvestDate) {
					harvestDate = status.Date
				}
			}
			if status.Status == "Curing" {
				if status.Date.Before(harvestDate) {
					harvestDate = status.Date
				}
			}
			if status.Status == "Drying" {
				if status.Date.Before(harvestDate) {
					harvestDate = status.Date
				}
			}
		}

		//calculate estimated harvest date
		estHarvestDate = start_dt.AddDate(0, 0, cycle_time)

		//Convert int and dates to strings
		strCurrentHeight := strconv.Itoa(iCurrentHeight)

		plant = types.Plant{id, name, description, status, statusID, strain_name, strain_id, breeder_name, zone_name, zoneID, iCurrentDay, iCurrentWeek, strCurrentHeight, heightDate, lastWaterDate, lastFeedDate, measurements, activities, statusHistory, sensorList, latestImage, images, isClone, start_dt, harvest_weight, harvestDate, cycle_time, strain_url, estHarvestDate, autoflower, parent_id, parent_name}
	}

	return plant
}

func LinkSensorsToPlant(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "LinkSensorsToPlant")
	var input struct {
		PlantID   string `json:"plant_id"`
		SensorIDs []int  `json:"sensor_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Serialize SensorIDs to JSON
	sensorIDsJSON, err := json.Marshal(input.SensorIDs)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to serialize sensor IDs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process sensor IDs"})
		return
	}

	// Initialize the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to the database"})
		return
	}

	// Update the plant with the serialized sensor IDs
	_, err = db.Exec("UPDATE plant SET sensors = $1 WHERE id = $2", sensorIDsJSON, input.PlantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update sensors")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update sensors for the plant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensors linked to plant successfully"})
}

func AddStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddStrainHandler")
	// Parse the incoming JSON request
	var req struct {
		Name             string `json:"name"`
		BreederID        *int   `json:"breeder_id"` // Nullable for new breeders
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

	if err := c.ShouldBindJSON(&req); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Validate Indica and Sativa sum
	if req.Indica+req.Sativa != 100 {
		fieldLogger.Error("Indica and Sativa must sum to 100")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Indica and Sativa must sum to 100"})
		return
	}

	// Open the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	// Check for new breeder and insert if needed
	var breederID int
	if req.BreederID == nil {
		if req.NewBreeder == "" {
			fieldLogger.Error("New breeder name is required")
			c.JSON(http.StatusBadRequest, gin.H{"error": "New breeder name is required"})
			return
		}

		// Insert new breeder
		insertBreederStmt := `
			INSERT INTO breeder (name)
			VALUES ($1)
		 RETURNING id`
		err := db.QueryRow(insertBreederStmt, req.NewBreeder).Scan(&breederID)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert new breeder")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add new breeder"})
			return
		}

		config.Breeders = GetBreeders()
	} else {
		// Use existing breeder ID
		breederID = *req.BreederID
	}

	// Insert the new strain into the database
	stmt := `
		INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, seed_count, description, cycle_time, url, short_desc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id
	`
	//convert autoflower to int
	var autoflowerInt int
	if req.Autoflower {
		autoflowerInt = 1
	} else {
		autoflowerInt = 0
	}
	var id int
	err = db.QueryRow(stmt, req.Name, breederID, req.Indica, req.Sativa, autoflowerInt, req.SeedCount, req.Description, req.CycleTime, req.Url, req.ShortDescription).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert strain")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add strain"})
		return
	}

	config.Strains = GetStrains()

	// Respond with success
	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "Strain added successfully"})
}

func GetStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetStrainHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	// Open the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	var strain types.Strain

	err = db.QueryRow(`
        SELECT s.id, s.name, b.name as breeder, s.indica, s.sativa, s.autoflower, s.description, coalesce(s.short_desc, ''), s.seed_count
        FROM strain s LEFT OUTER JOIN breeder b on s.breeder_id = b.id
        WHERE id = $1`, id).Scan(
		&strain.ID, &strain.Name, &strain.Breeder, &strain.Indica, &strain.Sativa,
		&strain.Autoflower, &strain.Description, &strain.ShortDescription, &strain.SeedCount)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Strain not found"})
		} else {
			fieldLogger.WithError(err).Error("Failed to fetch strain")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch strain"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	var req struct {
		Name             string `json:"name"`
		BreederID        *int   `json:"breeder_id"` // Nullable for new breeders
		NewBreeder       string `json:"new_breeder"`
		Indica           int    `json:"indica"`
		Sativa           int    `json:"sativa"`
		Autoflower       bool   `json:"autoflower"`
		Description      string `json:"description"`
		ShortDescription string `json:"short_desc"`
		SeedCount        int    `json:"seed_count"`
		CycleTime        int    `json:"cycle_time"`
		Url              string `json:"url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate Indica and Sativa sum
	if req.Indica+req.Sativa != 100 {
		fieldLogger.Error("Indica and Sativa must sum to 100")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Indica and Sativa must sum to 100"})
		return
	}

	// Open the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}

	// Determine the breeder ID
	var breederID int
	if req.BreederID == nil {
		if req.NewBreeder == "" {
			fieldLogger.Error("New breeder name is required")
			c.JSON(http.StatusBadRequest, gin.H{"error": "New breeder name is required"})
			return
		}

		// Insert the new breeder into the database
		insertBreederStmt := `
			INSERT INTO breeder (name)
			VALUES ($1)
			RETURNING id
		`
		err := db.QueryRow(insertBreederStmt, req.NewBreeder).Scan(&breederID)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to insert new breeder")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add new breeder"})
			return
		}

		config.Breeders = GetBreeders()
	} else {
		breederID = *req.BreederID
	}

	// Update the strain in the database
	updateStmt := `
        UPDATE strain
        SET name = $1, breeder_id = $2, indica = $3, sativa = $4, autoflower = $5, description = $6, seed_count = $7, cycle_time = $8, url = $9, short_desc = $10
        WHERE id = $11
    `
	//Convert autoflower to int
	var autoflowerInt int
	if req.Autoflower {
		autoflowerInt = 1
	} else {
		autoflowerInt = 0
	}
	_, err = db.Exec(updateStmt, req.Name, breederID, req.Indica, req.Sativa,
		autoflowerInt, req.Description, req.SeedCount, req.CycleTime, req.Url, req.ShortDescription, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update strain")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update strain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strain updated successfully"})
}

func DeleteStrainHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteStrainHandler")
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid strain ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	// Open the database
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}

	result, err := db.Exec(`DELETE FROM strain WHERE id = $1`, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete strain")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete strain"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		fieldLogger.Error("Strain not found")
		c.JSON(http.StatusNotFound, gin.H{"error": "Strain not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strain deleted successfully"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new zone"})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new strain"})
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

	//Update the Plant Status Log
	updated, err := updatePlantStatusLog(db, input.PlantID, input.StatusID, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update plant status")
		return
	}
	if !updated {
		fieldLogger.Info("Plant status unchanged")
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
		} else {
			plant.HarvestDate, err = time.Parse("2006-01-02", harvestDateStr)
		}
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to parse harvest date")
			return nil, err
		}
		// calculate the estimated harvest date
		startDate := plant.StartDT
		//convert start date to a time.Time (has timezone data)
		startTime, err := time.Parse("2006-01-02T15:04:05Z", startDate)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve plants"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve plants"})
		return
	}

	c.JSON(http.StatusOK, plants)
}

func InStockStrainsHandler(c *gin.Context) {
	strains, err := getStrainsBySeedCount(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch in-stock strains"})
		return
	}
	c.JSON(http.StatusOK, strains)
}
func OutOfStockStrainsHandler(c *gin.Context) {
	strains, err := getStrainsBySeedCount(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch out-of-stock strains"})
		return
	}
	c.JSON(http.StatusOK, strains)
}
func getStrainsBySeedCount(inStock bool) ([]types.Strain, error) {
	fieldLogger := logger.Log.WithField("func", "getStrainsBySeedCount")
	query := `
        SELECT s.id, s.name, b.name AS breeder, b.id as breeder_id, s.indica, s.sativa, s.autoflower, s.seed_count, s.description, coalesce(s.short_desc, ''), coalesce(s.cycle_time, 0), coalesce(s.url, '')
        FROM strain s
        JOIN breeder b ON s.breeder_id = b.id
        WHERE s.seed_count > 0
		ORDER BY s.name ASC
    `
	if !inStock {
		query = `
            SELECT s.id, s.name, b.name AS breeder, b.id as breeder_id, s.indica, s.sativa, s.autoflower, s.seed_count, s.description, coalesce(s.short_desc, ''), coalesce(s.cycle_time, 0), coalesce(s.url, '')
            FROM strain s
            JOIN breeder b ON s.breeder_id = b.id
            WHERE s.seed_count = 0
            ORDER BY s.name ASC
        `
	}

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return nil, err
	}

	rows, err := db.Query(query)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query strains")
		return nil, err
	}
	defer rows.Close()

	var strains []types.Strain
	for rows.Next() {
		var strain types.Strain
		if err := rows.Scan(&strain.ID, &strain.Name, &strain.Breeder, &strain.BreederID, &strain.Indica, &strain.Sativa, &strain.Autoflower, &strain.SeedCount, &strain.Description, &strain.ShortDescription, &strain.CycleTime, &strain.Url); err != nil {
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
		context.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	fieldLogger = fieldLogger.WithField("strainID", strainID)
	fieldLogger.Info("Fetching plants for strain")

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return
	}

	// Query plants with the given strain ID
	rows, err := db.Query(`SELECT id, name FROM plant WHERE strain_id = $1 ORDER BY name ASC`, strainID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to query database")
		context.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch plants"})
		return
	}
	defer rows.Close()

	// Parse query results
	var plants []types.Plant
	for rows.Next() {
		var plant types.Plant
		if err := rows.Scan(&plant.ID, &plant.Name); err != nil {
			fieldLogger.WithError(err).Error("Failed to scan row")
			context.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process results"})
			return
		}
		plants = append(plants, plant)
	}

	if err := rows.Err(); err != nil {
		fieldLogger.WithError(err).Error("Error iterating rows")
		context.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process results"})
		return
	}

	// Return the list of plants as JSON
	fieldLogger.WithField("plantCount", len(plants)).Info("Plants fetched successfully")
	context.JSON(http.StatusOK, plants)
}

func GetStrain(id string) types.Strain {
	fieldLogger := logger.Log.WithField("func", "GetStrain")
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		return types.Strain{}
	}

	var strain types.Strain
	//join in breeder name
	err = db.QueryRow(`
		SELECT s.id, s.name, coalesce(s.short_desc, ''), b.name AS breeder, b.id as breeder_id, s.indica, s.sativa, s.autoflower, s.seed_count, s.description, coalesce(s.cycle_time, 0), coalesce(s.url, '')
		FROM strain s
		JOIN breeder b ON s.breeder_id = b.id
		WHERE s.id = $1`, id).Scan(
		&strain.ID, &strain.Name, &strain.ShortDescription, &strain.Breeder, &strain.BreederID, &strain.Indica, &strain.Sativa, &strain.Autoflower, &strain.SeedCount, &strain.Description, &strain.CycleTime, &strain.Url)
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
