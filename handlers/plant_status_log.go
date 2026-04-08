package handlers

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
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
		apiBadRequest(c, "api_invalid_input")
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"id":   input.ID,
		"date": input.Date,
	})

	db := DBFromContext(c)

	query := `UPDATE plant_status_log SET date = $1 WHERE id = $2`
	_, err := db.Exec(query, input.Date, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update status in database")
		apiInternalError(c, "api_failed_to_update_status")
		return
	}

	logger.Log.Info("Status updated successfully")
	apiOK(c, "api_status_updated")
}

func DeleteStatus(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteStatus",
	})

	id := c.Param("id")
	fieldLogger = fieldLogger.WithField("id", id)

	db := DBFromContext(c)

	// Find the plant_id for this status entry
	var plantID int
	err := db.QueryRow(`SELECT plant_id FROM plant_status_log WHERE id = $1`, id).Scan(&plantID)
	if err != nil {
		if err == sql.ErrNoRows {
			fieldLogger.WithError(err).Warn("Status id not found")
			apiNotFound(c, "api_status_not_found")
			return
		}
		fieldLogger.WithError(err).Error("Failed to lookup status")
		apiInternalError(c, "api_database_error")
		return
	}

	// Count how many status entries exist for this plant
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM plant_status_log WHERE plant_id = $1`, plantID).Scan(&count)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to count status entries for plant")
		apiInternalError(c, "api_database_error")
		return
	}

	if count <= 1 {
		// Prevent deletion of the last remaining status
		msg := fmt.Sprintf("Cannot delete the last status entry for plant %d", plantID)
		fieldLogger.Warn(msg)
		apiBadRequest(c, "api_cannot_delete_last_status")
		return
	}

	query := `DELETE FROM plant_status_log WHERE id = $1`
	_, err = db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete status from database")
		apiInternalError(c, "api_failed_to_delete_status")
		return
	}

	fieldLogger.Info("Status deleted successfully")
	apiOK(c, "api_status_deleted")
}
