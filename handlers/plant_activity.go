package handlers

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	model "isley/model"
	"log"
	"net/http"
)

func CreatePlantActivity(c *gin.Context) {
	var input struct {
		PlantID    int    `json:"plant_id"`
		ActivityID int    `json:"activity_id"`
		Note       string `json:"note"`
		Date       string `json:"date"` // YYYY-MM-DD format
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
	_, err = db.Exec("INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES (?, ?, ?, ?)", input.PlantID, input.ActivityID, input.Note, input.Date)
	if err != nil {
		log.Printf("Error writing to db: %v", err)
		return
	}

	c.JSON(http.StatusCreated, input)
}

func EditActivity(c *gin.Context) {
	var input struct {
		ID         uint   `json:"id"`
		Date       string `json:"date"`
		ActivityID uint   `json:"activity_id"`
		Note       string `json:"note"`
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

	query := `UPDATE plant_activity SET date = ?, activity_id = ?, note = ? WHERE id = ?`
	_, err = db.Exec(query, input.Date, input.ActivityID, input.Note, input.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Activity updated successfully"})
}

func DeleteActivity(c *gin.Context) {
	id := c.Param("id")

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	query := `DELETE FROM plant_activity WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Activity deleted successfully"})
}
