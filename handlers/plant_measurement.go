package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
	"isley/utils"
	"net/http"
)

// CreatePlantMeasurement adds a new sensor data record
func CreatePlantMeasurement(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "CreatePlantMeasurement",
	})

	var input struct {
		PlantID  int     `json:"plant_id"`
		MetricID int     `json:"metric_id"`
		Value    float64 `json:"value"`
		Date     string  `json:"date"` // YYYY-MM-DD format
	}

	// Bind JSON payload
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	if err := utils.ValidateFiniteFloat64("value", input.Value); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateDate("date", input.Date); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"plant_id":  input.PlantID,
		"metric_id": input.MetricID,
		"value":     input.Value,
		"date":      input.Date,
	})

	// Init the db
	db := DBFromContext(c)

	_, err := db.Exec("INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES ($1, $2, $3, $4)",
		input.PlantID, input.MetricID, input.Value, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert measurement into database")
		apiInternalError(c, "api_failed_to_save_measurement")
		return
	}

	fieldLogger.Info("Plant measurement created successfully")
	c.JSON(http.StatusCreated, input)
}

func EditMeasurement(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "EditMeasurement",
	})

	var input struct {
		ID    uint    `json:"id"`
		Date  string  `json:"date"`
		Value float64 `json:"value"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	if err := utils.ValidateFiniteFloat64("value", input.Value); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateDate("date", input.Date); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"id":    input.ID,
		"date":  input.Date,
		"value": input.Value,
	})

	db := DBFromContext(c)

	query := `UPDATE plant_measurements SET date = $1, value = $2 WHERE id = $3`
	_, err := db.Exec(query, input.Date, input.Value, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update measurement in database")
		apiInternalError(c, "api_failed_to_update_measurement")
		return
	}

	fieldLogger.Info("Measurement updated successfully")
	apiOK(c, "api_measurement_updated")
}

func DeleteMeasurement(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteMeasurement",
	})

	id := c.Param("id")
	fieldLogger = fieldLogger.WithField("id", id)

	db := DBFromContext(c)

	query := `DELETE FROM plant_measurements WHERE id = $1`
	_, err := db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete measurement from database")
		apiInternalError(c, "api_failed_to_delete_measurement")
		return
	}

	fieldLogger.Info("Measurement deleted successfully")
	apiOK(c, "api_measurement_deleted")
}
