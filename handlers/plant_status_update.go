package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"isley/logger"
	model "isley/model"
)

const statusDateTimeLayout = "2006-01-02T15:04:05"

func updatePlantStatusLog(db *sql.DB, plantID int, statusID int, date string) (bool, int, error) {
	if date == "" {
		date = time.Now().Format(statusDateTimeLayout)
	}

	var currentStatus int
	err := db.QueryRow("SELECT status_id FROM plant_status_log WHERE plant_id = $1 ORDER BY date DESC LIMIT 1", plantID).Scan(&currentStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, 0, err
	}
	if err == nil && currentStatus == statusID {
		return false, 0, nil
	}

	// Insert and return new id in a driver-specific way
	if model.IsPostgres() {
		var newID int
		err = db.QueryRow("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES ($1, $2, $3) RETURNING id", plantID, statusID, date).Scan(&newID)
		if err != nil {
			return false, 0, err
		}
		return true, newID, nil
	} else {
		// SQLite (or other) - use Exec and LastInsertId
		res, err := db.Exec("INSERT INTO plant_status_log (plant_id, status_id, date) VALUES (?, ?, ?)", plantID, statusID, date)
		if err != nil {
			return false, 0, err
		}
		lastId, err := res.LastInsertId()
		if err != nil {
			// Insert succeeded but couldn't get id; return updated=true with zero id
			return true, 0, nil
		}
		return true, int(lastId), nil
	}
}

func UpdatePlantStatus(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "UpdatePlantStatus",
	})

	var input struct {
		PlantID  int    `json:"plant_id"`
		StatusID int    `json:"status_id"`
		Date     string `json:"date"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fieldLogger = fieldLogger.WithFields(logrus.Fields{
		"plant_id":  input.PlantID,
		"status_id": input.StatusID,
	})

	if input.PlantID == 0 || input.StatusID == 0 {
		fieldLogger.Error("Plant ID and status ID are required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "plant_id and status_id are required"})
		return
	}

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to connect to database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	updated, newID, err := updatePlantStatusLog(db, input.PlantID, input.StatusID, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update plant status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update plant status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"updated": updated, "id": newID})
}
