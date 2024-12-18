package handlers

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
	model "isley/model"
	"net/http"
)

func CreatePlantActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "CreatePlantActivity",
	})

	var input struct {
		PlantID    int    `json:"plant_id"`
		ActivityID int    `json:"activity_id"`
		Note       string `json:"note"`
		Date       string `json:"date"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input: " + err.Error()})
		return
	}

	fieldLogger = logger.Log.WithFields(logrus.Fields{
		"plant_id":    input.PlantID,
		"activity_id": input.ActivityID,
		"note":        input.Note,
		"date":        input.Date,
	})

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES (?, ?, ?, ?)", input.PlantID, input.ActivityID, input.Note, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert activity into database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create activity"})
		return
	}

	fieldLogger.Info("Plant activity created successfully")
	c.JSON(http.StatusCreated, input)
}

func EditActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "EditActivity",
	})

	var input struct {
		ID         uint   `json:"id"`
		Date       string `json:"date"`
		ActivityID uint   `json:"activity_id"`
		Note       string `json:"note"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id":          input.ID,
		"activity_id": input.ActivityID,
		"date":        input.Date,
		"note":        input.Note,
	})

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	query := `UPDATE plant_activity SET date = ?, activity_id = ?, note = ? WHERE id = ?`
	_, err = db.Exec(query, input.Date, input.ActivityID, input.Note, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update activity"})
		return
	}

	fieldLogger.Info("Plant activity updated successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Activity updated successfully"})
}

func DeleteActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteActivity",
	})

	id := c.Param("id")
	fieldLogger.WithField("id", id)

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	query := `DELETE FROM plant_activity WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete activity"})
		return
	}

	fieldLogger.Info("Plant activity deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Activity deleted successfully"})
}

func RecordMultiPlantActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "RecordMultiPlantActivity",
	})

	var request struct {
		PlantIDs   []int  `json:"plant_ids"`
		ActivityID int    `json:"activity_id"`
		Note       string `json:"note"`
		Date       string `json:"date"`
	}

	if err := c.BindJSON(&request); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if len(request.PlantIDs) == 0 {
		fieldLogger.Error("No plants selected")
		c.JSON(http.StatusBadRequest, gin.H{"error": "No plants selected"})
		return
	}

	fieldLogger.WithFields(logrus.Fields{
		"activity_id": request.ActivityID,
		"note":        request.Note,
		"date":        request.Date,
		"plant_ids":   request.PlantIDs,
	})

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer db.Close()

	for _, plantID := range request.PlantIDs {
		_, err = db.Exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES (?, ?, ?, ?)`,
			plantID, request.ActivityID, request.Note, request.Date)
		if err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Failed to insert activity for plant")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save activity"})
			return
		}
	}

	fieldLogger.Info("Activities recorded successfully for multiple plants")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
