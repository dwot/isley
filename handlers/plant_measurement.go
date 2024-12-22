package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
	model "isley/model"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"plant_id":  input.PlantID,
		"metric_id": input.MetricID,
		"value":     input.Value,
		"date":      input.Date,
	})

	// Init the db
	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	_, err = db.Exec("INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES (?, ?, ?, ?)",
		input.PlantID, input.MetricID, input.Value, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert measurement into database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save measurement"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"id":    input.ID,
		"date":  input.Date,
		"value": input.Value,
	})

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	query := `UPDATE plant_measurements SET date = ?, value = ? WHERE id = ?`
	_, err = db.Exec(query, input.Date, input.Value, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update measurement in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update measurement"})
		return
	}

	fieldLogger.Info("Measurement updated successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Measurement updated successfully"})
}

func DeleteMeasurement(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteMeasurement",
	})

	id := c.Param("id")
	fieldLogger = fieldLogger.WithField("id", id)

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	query := `DELETE FROM plant_measurements WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete measurement from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete measurement"})
		return
	}

	fieldLogger.Info("Measurement deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Measurement deleted successfully"})
}
