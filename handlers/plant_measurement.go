package handlers

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	model "isley/model"
	"log"
	"net/http"
)

// CreatePlantMeasurement adds a new sensor data record
func CreatePlantMeasurement(c *gin.Context) {
	var input struct {
		PlantID  int     `json:"plant_id"`
		MetricID int     `json:"metric_id"`
		Value    float64 `json:"value"`
		Date     string  `json:"date"` // YYYY-MM-DD format
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
	_, err = db.Exec("INSERT INTO plant_measurements (plant_id, metric_id, value, date) VALUES (?, ?, ?, ?)", input.PlantID, input.MetricID, input.Value, input.Date)
	if err != nil {
		log.Printf("Error writing to db: %v", err)
		return
	}

	c.JSON(http.StatusCreated, input)
}

func EditMeasurement(c *gin.Context) {
	var input struct {
		ID    uint    `json:"id"`
		Date  string  `json:"date"`
		Value float64 `json:"value"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	query := `UPDATE plant_measurements SET date = ?, value = ? WHERE id = ?`
	_, err = db.Exec(query, input.Date, input.Value, input.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update measurement"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Measurement updated successfully"})
}

func DeleteMeasurement(c *gin.Context) {
	id := c.Param("id")

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	query := `DELETE FROM plant_measurements WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete measurement"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Measurement deleted successfully"})
}
