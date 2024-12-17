package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"isley/config"
	model "isley/model"
	"isley/model/types"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PlantListResponse struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	StartDT       time.Time `json:"start_dt"`
	CurrentDay    int       `json:"current_day"`
	CurrentWeek   int       `json:"current_week"`
	Status        string    `json:"status"`
	StrainName    string    `json:"strain_name"`
	BreederName   string    `json:"breeder_name"`
	ZoneName      string    `json:"zone_name"`
	HarvestWeight float64   `json:"harvest_weight"`
}

type PlantDataResponse struct {
	ID            uint               `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Status        string             `json:"status"`
	StatusID      int                `json:"status_id"`
	StrainName    string             `json:"strain_name"`
	StrainID      int                `json:"strain_id"`
	BreederName   string             `json:"breeder_name"`
	ZoneName      string             `json:"zone_name"`
	CurrentDay    int                `json:"current_day"`
	CurrentWeek   int                `json:"current_week"`
	CurrentHeight string             `json:"current_height"`
	HeightDate    time.Time          `json:"height_date"`
	LastWaterDate time.Time          `json:"last_water_date"`
	LastFeedDate  time.Time          `json:"last_feed_date"`
	Measurements  []Measurement      `json:"measurements"`
	Activities    []Activity         `json:"activities"`
	StatusHistory []Status           `json:"status_history"`
	Sensors       []Sensor           `json:"sensors"`
	LatestImage   types.PlantImage   `json:"latest_image"`
	Images        []types.PlantImage `json:"images"`
	IsClone       bool               `json:"is_clone"`
	StartDT       time.Time          `json:"start_dt"`
	HarvestWeight float64            `json:"harvest_weight"`
	HarvestDate   time.Time          `json:"harvest_date"`
}

type Sensor struct {
	ID    uint      `json:"id"`
	Name  string    `json:"name"`
	Unit  string    `json:"unit"`
	Value float64   `json:"value"`
	Date  time.Time `json:"date"`
}

type Measurement struct {
	ID    uint      `json:"id"`
	Name  string    `json:"name"`
	Value float64   `json:"value"`
	Date  time.Time `json:"date"`
}

type Activity struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Note       string    `json:"note"`
	Date       time.Time `json:"date"`
	ActivityId int       `json:"activity_id"`
}

type Status struct {
	ID     uint      `json:"id"`
	Status string    `json:"status"`
	Date   time.Time `json:"date"`
}

func GetBreeders() []config.BreederResponse {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query("SELECT id, name FROM breeder")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var breeders []config.BreederResponse
	for rows.Next() {
		var breeder config.BreederResponse
		err = rows.Scan(&breeder.ID, &breeder.Name)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		breeders = append(breeders, breeder)
	}

	//Close the db
	db.Close()

	return breeders
}

func AddPlant(c *gin.Context) {
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
		StatusID int    `json:"status_id"`
		Date     string `json:"date"`
		Sensors  string `json:"sensors"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new strain"})
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	// Insert the new plant
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return
	}

	//Insert into the plants table returning id
	result, err := db.Exec("INSERT INTO plant (name, zone_id, strain_id, description, clone, start_dt, sensors) VALUES (?, ?, ?, '', 'false', ?, '[]')", input.Name, *input.ZoneID, *input.StrainID, input.Date)
	if err != nil {
		fmt.Println(err)
		return
	}
	//Update plant_status_log with the new plant id and status id
	plantID, err := result.LastInsertId()
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = db.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES (?, ?, ?)", plantID, input.StatusID, input.Date)
	if err != nil {
		fmt.Println(err)
		return
	}

	//Close the db
	db.Close()

	c.JSON(http.StatusOK, gin.H{"message": "Plant added successfully"})
}

func GetStrains() []config.StrainResponse {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query("SELECT s.id, s.name, b.id as breeder_id, b.name as breeder, s.indica, s.sativa, s.autoflower, s.description, s.seed_count FROM strain s left outer join breeder b on s.breeder_id = b.id")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var strains []config.StrainResponse
	for rows.Next() {
		var strain config.StrainResponse
		err = rows.Scan(&strain.ID, &strain.Name, &strain.BreederID, &strain.Breeder, &strain.Indica, &strain.Sativa, &strain.Autoflower, &strain.Description, &strain.SeedCount)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		strains = append(strains, strain)
	}

	//Close the db
	db.Close()

	return strains
}

func GetActivities() []config.ActivityResponse {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query("SELECT id, name FROM activity")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var activities []config.ActivityResponse
	for rows.Next() {
		var activity config.ActivityResponse
		err = rows.Scan(&activity.ID, &activity.Name)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		activities = append(activities, activity)
	}
	//Close the db
	db.Close()

	return activities
}

func GetMetrics() []config.MetricResponse {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query("SELECT id, name, unit FROM metric")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var measurements []config.MetricResponse
	for rows.Next() {
		var measurement config.MetricResponse
		err = rows.Scan(&measurement.ID, &measurement.Name, &measurement.Unit)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		measurements = append(measurements, measurement)
	}
	//Close the db
	db.Close()

	return measurements
}

func GetStatuses() []config.StatusResponse {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return nil
	}

	rows, err := db.Query("SELECT id, status FROM plant_status")
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var statuses []config.StatusResponse
	for rows.Next() {
		var status config.StatusResponse
		err = rows.Scan(&status.ID, &status.Status)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		statuses = append(statuses, status)
	}

	//Close the db
	db.Close()

	return statuses

}

func CreateNewStrain(newStrain *struct {
	Name       string `json:"name"`
	BreederId  int    `json:"breeder_id"`
	NewBreeder string `json:"new_breeder"`
}) (int, error) {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var breederId int

	// Check if a new breeder needs to be added
	if newStrain.BreederId == 0 && newStrain.NewBreeder != "" {
		// Insert the new breeder into the `breeder` table
		result, err := db.Exec("INSERT INTO breeder (name) VALUES (?)", newStrain.NewBreeder)
		if err != nil {
			return 0, fmt.Errorf("failed to insert new breeder: %w", err)
		}

		config.Breeders = GetBreeders()

		// Get the ID of the newly inserted breeder
		lastInsertId, err := result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("failed to retrieve new breeder ID: %w", err)
		}
		breederId = int(lastInsertId)
	} else {
		// Use the existing breeder ID
		breederId = newStrain.BreederId
	}

	// Insert the new strain into the `strain` table
	result, err := db.Exec(
		`INSERT INTO strain (name, breeder_id, sativa, indica, autoflower, description, seed_count)
		 VALUES (?, ?, 50, 50, 'true', '', 0)`,
		newStrain.Name, breederId)
	if err != nil {
		return 0, fmt.Errorf("failed to insert new strain: %w", err)
	}

	config.Strains = GetStrains()

	// Get the ID of the newly inserted strain
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve new strain ID: %w", err)
	}

	return int(id), nil
}

func DeletePlant(c *gin.Context) {
	id := c.Param("id")

	err := DeletePlantById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete plant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Plant deleted successfully"})
}

func DeletePlantById(id string) interface{} {
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer db.Close()

	// Delete the plant's images
	_, err = db.Exec("DELETE FROM plant_images WHERE plant_id = ?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// Delete the plant's measurements
	_, err = db.Exec("DELETE FROM plant_measurements WHERE plant_id = ?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// Delete the plant's activities
	_, err = db.Exec("DELETE FROM plant_activity WHERE plant_id = ?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// Delete the plant's status log
	_, err = db.Exec("DELETE FROM plant_status_log WHERE plant_id = ?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}

	// Delete the plant
	_, err = db.Exec("DELETE FROM plant WHERE id = ?", id)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

type SensorData struct {
	ID       uint      `json:"id"`
	Value    float64   `json:"value"`
	CreateDT time.Time `json:"create_dt"`
}

func GetPlant(id string) PlantDataResponse {
	var plant PlantDataResponse
	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return plant
	}
	rows, err := db.Query("SELECT p.id, p.name, p.description, p.clone, p.start_dt, s.name as strain_name, b.name as breeder_name, z.name as zone_name, (select ps.status from plant_status_log psl left outer join plant_status ps on psl.status_id = ps.id where psl.plant_id = p.id order by strftime('%s', psl.date) desc limit 1) as current_status, (select ps.id from plant_status_log psl left outer join plant_status ps on psl.status_id = ps.id where psl.plant_id = p.id order by strftime('%s', psl.date) desc limit 1) as status_id, p.sensors, s.id, p.harvest_weight FROM plant p LEFT OUTER JOIN strain s on p.strain_id = s.id left outer join breeder b on b.id = s.breeder_id LEFT OUTER JOIN zones z on p.zone_id = z.id WHERE p.id = $1", id)
	if err != nil {
		fmt.Println(err)
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
		var status string
		var statusID int
		var sensors string
		var strain_id int
		var harvest_weight float64
		err = rows.Scan(&id, &name, &description, &isClone, &start_dt, &strain_name, &breeder_name, &zone_name, &status, &statusID, &sensors, &strain_id, &harvest_weight)
		if err != nil {
			fmt.Println(err)
			return plant
		}
		// Calculate current day and week
		currentTime := time.Now().In(time.Local)
		//Calculate the # of hours difference between the current timezone and UTC
		_, tzDiff := currentTime.Zone()
		_, utcOffset := start_dt.Zone()
		tzDiff = utcOffset - tzDiff
		start_dt = start_dt.Add(time.Duration(tzDiff) * time.Second)
		diff := currentTime.Sub(start_dt)
		iCurrentDay := int(diff.Hours()/24) + 1
		iCurrentWeek := int((diff.Hours() / 24 / 7) + 1)

		//convert sensors string into list and Iterate over sensors and load sensor and latest sensor_data
		var sensorList []Sensor

		// Retrieve the serialized sensors column from the plant table
		var sensorsJSON string
		err := db.QueryRow("SELECT sensors FROM plant WHERE id = $1", id).Scan(&sensorsJSON)
		if err != nil {
			fmt.Println("Error querying sensors column:", err)
			return plant
		}

		// Deserialize the JSON data into a slice of integers
		var sensorIDs []int
		err = json.Unmarshal([]byte(sensorsJSON), &sensorIDs)
		if err != nil {
			fmt.Println("Error unmarshalling sensors JSON:", err)
			return plant
		}

		// Loop through each sensor ID and fetch details
		for _, sensorID := range sensorIDs {
			var sensor Sensor

			// Query sensor details from the sensors table
			err := db.QueryRow("SELECT id, name, unit FROM sensors WHERE id = ?", sensorID).Scan(&sensor.ID, &sensor.Name, &sensor.Unit)
			if err != nil {
				fmt.Println("Error querying sensor details:", err)
				continue
			}

			// Query the latest sensor data from the sensor_data table
			var sensorData SensorData
			err = db.QueryRow("SELECT id, value, create_dt FROM sensor_data WHERE sensor_id = ? ORDER BY create_dt DESC LIMIT 1", sensorID).Scan(&sensorData.ID, &sensorData.Value, &sensorData.CreateDT)
			if err != nil {
				fmt.Println("Error querying latest sensor data:", err)
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
			fmt.Println(err)
		}
		var measurements []Measurement
		for rows2.Next() {
			var id uint
			var name string
			var value float64
			var date time.Time
			err = rows2.Scan(&id, &name, &value, &date)
			if err != nil {
				fmt.Println(err)
			}
			measurements = append(measurements, Measurement{id, name, value, date})
		}

		//Load activities
		rows3, err := db.Query("SELECT pa.id, a.id as activity_id, a.name, pa.note, pa.date FROM plant_activity pa left outer join activity a on a.id = pa.activity_id WHERE pa.plant_id = $1 ORDER BY date desc", id)
		if err != nil {
			fmt.Println(err)
		}
		var activities []Activity
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
			activities = append(activities, Activity{id, name, note, date, activityId})
		}

		//Load status history
		rows5, err := db.Query("SELECT psl.id, ps.status, psl.date FROM plant_status_log psl left outer join plant_status ps on psl.status_id = ps.id WHERE psl.plant_id = $1 ORDER BY date desc", id)
		if err != nil {
			fmt.Println(err)
		}
		var statusHistory []Status
		for rows5.Next() {
			var id uint
			var status string
			var date time.Time
			err = rows5.Scan(&id, &status, &date)
			if err != nil {
				fmt.Println(err)
			}
			statusHistory = append(statusHistory, Status{id, status, date})
		}

		//Load latest image
		var latestImage types.PlantImage
		err = db.QueryRow("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = ? ORDER BY image_date DESC LIMIT 1", id).Scan(&latestImage.ID, &latestImage.ImagePath, &latestImage.ImageDescription, &latestImage.ImageOrder, &latestImage.ImageDate)
		if err != nil {
			fmt.Println(err)
			latestImage = types.PlantImage{ID: 0, PlantID: plant.ID, ImagePath: "/static/img/winston.hat.jpg", ImageDescription: "Placeholder", ImageOrder: 100, ImageDate: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
		} else {
			latestImage.ImagePath = "/" + strings.Replace(latestImage.ImagePath, "\\", "/", -1)
		}

		//Load images
		rows6, err := db.Query("SELECT id, image_path, image_description, image_order, image_date FROM plant_images WHERE plant_id = $1 ORDER BY image_date desc", id)
		if err != nil {
			fmt.Println(err)
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
				fmt.Println(err)
			}
			//Convert any \ in image_path to /
			image_path = "/" + strings.Replace(image_path, "\\", "/", -1)
			images = append(images, types.PlantImage{ID: id, PlantID: plant.ID, ImagePath: image_path, ImageDescription: image_description, ImageOrder: image_order, ImageDate: image_date, CreatedAt: time.Now(), UpdatedAt: time.Now()})
		}

		iCurrentHeight := 0
		//initialize the height date to a time in the past Jan 1, 1970
		heightDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		lastWaterDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		lastFeedDate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		harvestDate := time.Now()

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

		//Convert int and dates to strings
		strCurrentHeight := strconv.Itoa(iCurrentHeight)

		plant = PlantDataResponse{id, name, description, status, statusID, strain_name, strain_id, breeder_name, zone_name, iCurrentDay, iCurrentWeek, strCurrentHeight, heightDate, lastWaterDate, lastFeedDate, measurements, activities, statusHistory, sensorList, latestImage, images, isClone, start_dt, harvest_weight, harvestDate}
	}

	// Close the db
	err = db.Close()
	if err != nil {
		fmt.Println(err)
		return plant
	}

	return plant
}

func LinkSensorsToPlant(c *gin.Context) {
	var input struct {
		PlantID   string `json:"plant_id"`
		SensorIDs []int  `json:"sensor_ids"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fmt.Println("Error binding JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Serialize SensorIDs to JSON
	sensorIDsJSON, err := json.Marshal(input.SensorIDs)
	if err != nil {
		fmt.Println("Error serializing sensor IDs:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process sensor IDs"})
		return
	}

	// Initialize the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fmt.Println("Database connection error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to the database"})
		return
	}
	defer db.Close()

	// Update the plant with the serialized sensor IDs
	_, err = db.Exec("UPDATE plant SET sensors = ? WHERE id = ?", sensorIDsJSON, input.PlantID)
	if err != nil {
		fmt.Println("Error updating plant sensors:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update sensors for the plant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensors linked to plant successfully"})
}

type AddStrainRequest struct {
	Name        string `json:"name"`
	BreederID   *int   `json:"breeder_id"` // Nullable for new breeders
	NewBreeder  string `json:"new_breeder"`
	Indica      int    `json:"indica"`
	Sativa      int    `json:"sativa"`
	Autoflower  string `json:"autoflower"`
	SeedCount   int    `json:"seed_count"`
	Description string `json:"description"`
}

func AddStrainHandler(c *gin.Context) {
	// Parse the incoming JSON request
	var req AddStrainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Println("Error binding JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Validate Indica and Sativa sum
	if req.Indica+req.Sativa != 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Indica and Sativa must sum to 100"})
		return
	}

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}
	defer db.Close()

	// Check for new breeder and insert if needed
	var breederID int
	if req.BreederID == nil {
		if req.NewBreeder == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "New breeder name is required"})
			return
		}

		// Insert new breeder
		insertBreederStmt := `
			INSERT INTO breeder (name)
			VALUES (?)
		`
		result, err := db.Exec(insertBreederStmt, req.NewBreeder)
		if err != nil {
			log.Println("Error inserting breeder:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add new breeder"})
			return
		}

		config.Breeders = GetBreeders()

		// Get the new breeder's ID
		newBreederID, err := result.LastInsertId()
		if err != nil {
			log.Println("Error retrieving new breeder ID:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve new breeder ID"})
			return
		}
		breederID = int(newBreederID)
	} else {
		// Use existing breeder ID
		breederID = *req.BreederID
	}

	// Insert the new strain into the database
	stmt := `
		INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, seed_count, description)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = db.Exec(stmt, req.Name, breederID, req.Indica, req.Sativa, req.Autoflower, req.SeedCount, req.Description)
	if err != nil {
		log.Println("Error inserting strain:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add strain"})
		return
	}

	config.Strains = GetStrains()

	// Respond with success
	c.JSON(http.StatusCreated, gin.H{"message": "Strain added successfully"})
}

func GetStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println("Invalid strain ID:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	var strain struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Breeder     string `json:"breeder"`
		Indica      int    `json:"indica"`
		Sativa      int    `json:"sativa"`
		Autoflower  string `json:"autoflower"`
		Description string `json:"description"`
		SeedCount   int    `json:"seed_count"`
	}

	err = db.QueryRow(`
        SELECT s.id, s.name, b.name as breeder, s.indica, s.sativa, s.autoflower, s.description, s.seed_count
        FROM strain s LEFT OUTER JOIN breeder b on s.breeder_id = b.id
        WHERE id = ?`, id).Scan(
		&strain.ID, &strain.Name, &strain.Breeder, &strain.Indica, &strain.Sativa,
		&strain.Autoflower, &strain.Description, &strain.SeedCount)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Strain not found"})
		} else {
			log.Println("Error fetching strain:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch strain"})
		}
		return
	}

	c.JSON(http.StatusOK, strain)
}

func UpdateStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println("Invalid strain ID:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	var strain struct {
		Name        string `json:"name"`
		BreederID   *int   `json:"breeder_id"` // Nullable for new breeders
		NewBreeder  string `json:"new_breeder"`
		Indica      int    `json:"indica"`
		Sativa      int    `json:"sativa"`
		Autoflower  string `json:"autoflower"`
		Description string `json:"description"`
		SeedCount   int    `json:"seed_count"`
	}

	if err := c.ShouldBindJSON(&strain); err != nil {
		log.Println("Invalid request body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Validate Indica and Sativa sum
	if strain.Indica+strain.Sativa != 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Indica and Sativa must sum to 100"})
		return
	}

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	// Determine the breeder ID
	var breederID int
	if strain.BreederID == nil {
		if strain.NewBreeder == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "New breeder name is required"})
			return
		}

		// Insert the new breeder into the database
		insertBreederStmt := `
			INSERT INTO breeder (name)
			VALUES (?)
		`
		result, err := db.Exec(insertBreederStmt, strain.NewBreeder)
		if err != nil {
			log.Println("Error inserting new breeder:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add new breeder"})
			return
		}

		config.Breeders = GetBreeders()

		// Get the new breeder's ID
		newBreederID, err := result.LastInsertId()
		if err != nil {
			log.Println("Error retrieving new breeder ID:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve new breeder ID"})
			return
		}
		breederID = int(newBreederID)
	} else {
		breederID = *strain.BreederID
	}

	// Update the strain in the database
	updateStmt := `
        UPDATE strain
        SET name = ?, breeder_id = ?, indica = ?, sativa = ?, autoflower = ?, description = ?, seed_count = ?
        WHERE id = ?
    `
	_, err = db.Exec(updateStmt, strain.Name, breederID, strain.Indica, strain.Sativa,
		strain.Autoflower, strain.Description, strain.SeedCount, id)
	if err != nil {
		log.Println("Error updating strain:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update strain"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strain updated successfully"})
}

func DeleteStrainHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println("Invalid strain ID:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid strain ID"})
		return
	}

	// Open the database
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		log.Println("Error opening database:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	result, err := db.Exec(`DELETE FROM strain WHERE id = ?`, id)
	if err != nil {
		log.Println("Error deleting strain:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete strain"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Strain not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Strain deleted successfully"})
}

func UpdatePlant(c *gin.Context) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	// Init the db
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert new zone into the database
		zoneID, err := CreateNewZone(input.NewZone)
		if err != nil {
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create new strain"})
			return
		}
		input.StrainID = &strainID // Set the created strain ID
	}

	//Update the plant
	_, err = db.Exec("UPDATE plant SET name = ?, description = ?, zone_id = ?, strain_id = ?, clone = ?, start_dt = ?, harvest_weight = ? WHERE id = ?", input.PlantName, input.PlantDescription, input.ZoneID, input.StrainID, input.IsClone, input.StartDT, input.HarvestWeight, input.PlantID)
	if err != nil {
		log.Printf("Error writing to db: %v", err)
		return
	}

	//Update the Plant Status Log
	//Check the current status and do not update if it's unchanged
	var currentStatus int
	err = db.QueryRow("SELECT status_id FROM plant_status_log WHERE plant_id = ? ORDER BY date DESC LIMIT 1", input.PlantID).Scan(&currentStatus)
	if err != nil {
		log.Printf("Error querying db: %v", err)
		return
	}
	if currentStatus != input.StatusID {
		_, err = db.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES (?, ?, ?)", input.PlantID, input.StatusID, input.Date)
		if err != nil {
			log.Printf("Error writing to db: %v", err)
			return
		}
	} else {
		log.Printf("Status unchanged, not updating")
	}
	c.JSON(http.StatusCreated, input)
}

// Plant represents the structure of a plant record.
type PlantTableResponse struct {
	ID                    int       `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	Clone                 bool      `json:"clone"`
	StrainName            string    `json:"strain_name"`
	BreederName           string    `json:"breeder_name"`
	ZoneName              string    `json:"zone_name"`
	StartDT               string    `json:"start_dt"`
	CurrentWeek           int       `json:"current_week"`
	CurrentDay            int       `json:"current_day"`
	DaysSinceLastWatering int       `json:"days_since_last_watering"`
	DaysSinceLastFeeding  int       `json:"days_since_last_feeding"`
	FloweringDays         *int      `json:"flowering_days,omitempty"` // nil if not flowering
	HarvestWeight         float64   `json:"harvest_weight"`
	Status                string    `json:"status"`
	StatusDate            time.Time `json:"status_date"`
}

func getPlantsByStatus(statuses []int) ([]PlantTableResponse, error) {
	// Generate placeholders for the number of statuses
	placeholders := make([]string, len(statuses))
	args := make([]interface{}, len(statuses))
	for i, status := range statuses {
		placeholders[i] = "?"
		args[i] = status
	}

	// Join the placeholders with commas
	inClause := "(" + strings.Join(placeholders, ",") + ")"

	// Use the dynamic IN clause in the query
	query := `
		SELECT p.id, p.name, p.description, p.clone, s.name AS strain_name, b.name AS breeder_name, z.name AS zone_name, 
		       p.start_dt,  
		       ((strftime('%j', 'now') - strftime('%j', p.start_dt)) / 7) +1 AS current_week,
		       (strftime('%j', 'now') - strftime('%j', p.start_dt)) +1 AS current_day,
		       COALESCE((SELECT (strftime('%j', 'now') - strftime('%j', MAX(date))) +1 FROM plant_activity pa JOIN activity a ON pa.activity_id = a.id WHERE pa.plant_id = p.id AND a.id = (SELECT id FROM activity WHERE name = 'Water')),0) AS days_since_last_watering,
		       COALESCE((SELECT (strftime('%j', 'now') - strftime('%j', MAX(date))) +1 FROM plant_activity pa JOIN activity a ON pa.activity_id = a.id WHERE pa.plant_id = p.id AND a.id = (SELECT id FROM activity WHERE name = 'Feed')),0) AS days_since_last_feeding,
		       COALESCE((SELECT (strftime('%j', 'now') - strftime('%j', MAX(date))) +1 FROM plant_status_log WHERE plant_id = p.id AND status_id = (SELECT id FROM plant_status WHERE status = 'Flower')),0) AS flowering_days,
		       p.harvest_weight, ps.status, psl.date as status_date
		FROM plant p
		JOIN strain s ON p.strain_id = s.id
		JOIN breeder b ON s.breeder_id = b.id
		LEFT JOIN zones z ON p.zone_id = z.id
		JOIN plant_status_log psl ON p.id = psl.plant_id
		JOIN plant_status ps ON psl.status_id = ps.id
		WHERE ps.id IN ` + inClause + ` AND psl.date = (
			SELECT MAX(date) FROM plant_status_log WHERE plant_id = p.id
		)
		ORDER BY p.name;
	`

	// Open the database connection
	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Execute the query
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plants := []PlantTableResponse{}
	for rows.Next() {
		var plant PlantTableResponse
		if err := rows.Scan(&plant.ID, &plant.Name, &plant.Description, &plant.Clone, &plant.StrainName, &plant.BreederName, &plant.ZoneName, &plant.StartDT, &plant.CurrentWeek, &plant.CurrentDay, &plant.DaysSinceLastWatering, &plant.DaysSinceLastFeeding, &plant.FloweringDays, &plant.HarvestWeight, &plant.Status, &plant.StatusDate); err != nil {
			return nil, err
		}
		plants = append(plants, plant)
	}

	return plants, nil
}

func GetLivingPlants() []PlantTableResponse {
	statuses := []int{2, 3, 4} // Seedling, Veg, Flower
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
	statuses := []int{7, 5, 6} // Success
	plants, err := getPlantsByStatus(statuses)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve plants"})
		return
	}

	c.JSON(http.StatusOK, plants)
}

// DeadPlantsHandler handles the /plants/dead endpoint.
func DeadPlantsHandler(c *gin.Context) {
	statuses := []int{8} // Dead
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
func getStrainsBySeedCount(inStock bool) ([]config.StrainResponse, error) {
	query := `
        SELECT s.id, s.name, b.name AS breeder, b.id as breeder_id, s.indica, s.sativa, s.autoflower, s.seed_count
        FROM strain s
        JOIN breeder b ON s.breeder_id = b.id
        WHERE s.seed_count > 0
    `
	if !inStock {
		query = `
            SELECT s.id, s.name, b.name AS breeder, b.id as breeder_id, s.indica, s.sativa, s.autoflower, s.seed_count
            FROM strain s
            JOIN breeder b ON s.breeder_id = b.id
            WHERE s.seed_count = 0
        `
	}

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var strains []config.StrainResponse
	for rows.Next() {
		var strain config.StrainResponse
		if err := rows.Scan(&strain.ID, &strain.Name, &strain.Breeder, &strain.BreederID, &strain.Indica, &strain.Sativa, &strain.Autoflower, &strain.SeedCount); err != nil {
			return nil, err
		}
		strains = append(strains, strain)
	}

	return strains, nil
}
