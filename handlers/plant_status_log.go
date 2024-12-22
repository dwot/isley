package handlers

import (
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

	query := `UPDATE plant_status_log SET date = ? WHERE id = ?`
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

	query := `DELETE FROM plant_status_log WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete status from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete status"})
		return
	}

	fieldLogger.Info("Status deleted successfully")
	c.JSON(http.StatusOK, gin.H{"message": "Status deleted successfully"})
}
