package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
	model "isley/model"
	"net/http"
)

func EditStatus(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "EditStatus",
	})

	var input struct {
		ID   uint   `json:"id"`
		Date string `json:"date"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"id":   input.ID,
		"date": input.Date,
	})

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to connect to database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	query := `UPDATE plant_status_log SET date = $1 WHERE id = $2`
	_, err = db.Exec(query, input.Date, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update status in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}

	logger.Log.Info("Status updated successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Status updated successfully"})
}

func DeleteStatus(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteStatus",
	})

	id := c.Param("id")
	fieldLogger = fieldLogger.WithField("id", id)

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to connect to database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Find the plant_id for this status entry
	var plantID int
	err = db.QueryRow(`SELECT plant_id FROM plant_status_log WHERE id = $1`, id).Scan(&plantID)
	if err != nil {
		if err == sql.ErrNoRows {
			fieldLogger.WithError(err).Warn("Status id not found")
			c.JSON(http.StatusNotFound, gin.H{"error": "Status not found"})
			return
		}
		fieldLogger.WithError(err).Error("Failed to lookup status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Count how many status entries exist for this plant
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM plant_status_log WHERE plant_id = $1`, plantID).Scan(&count)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to count status entries for plant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if count <= 1 {
		// Prevent deletion of the last remaining status
		msg := fmt.Sprintf("Cannot delete the last status entry for plant %d", plantID)
		fieldLogger.Warn(msg)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete the last status entry for a plant"})
		return
	}

	query := `DELETE FROM plant_status_log WHERE id = $1`
	_, err = db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete status from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete status"})
		return
	}

	fieldLogger.Info("Status deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Status deleted successfully"})
}
