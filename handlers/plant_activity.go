package handlers

import (
	"database/sql"
	"isley/logger"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type measurementInput struct {
	MetricID int     `json:"metric_id"`
	Value    float64 `json:"value"`
}

// saveActivityMeasurements inserts measurement rows linked to a plant activity.
// The transaction is expected to be managed by the caller
func saveActivityMeasurements(tx *sql.Tx, plantID int, plantActivityID int, date string, measurements []measurementInput) error {
	for _, m := range measurements {
		if m.MetricID <= 0 {
			continue
		}
		_, err := tx.Exec(
			`INSERT INTO plant_measurements (plant_id, metric_id, value, date, plant_activity_id)
			 VALUES ($1, $2, $3, $4, $5)`,
			plantID, m.MetricID, m.Value, date, plantActivityID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// deleteAllActivityMeasurements removes all measurement rows linked to a plant activity.
// The transaction is expected to be managed by the caller
func deleteAllActivityMeasurements(tx *sql.Tx, plantActivityID int) error {
	_, err := tx.Exec("DELETE FROM plant_measurements WHERE plant_activity_id = $1", plantActivityID)
	return err
}

// createPlantActivity creates a new plant activity and its linked measurements within a transaction.
// The transaction is expected to be managed by the caller
func createPlantActivity(tx *sql.Tx, plantID int, activityID int, note string, date string, measurements []measurementInput) error {
	var activityLogID int
	err := tx.QueryRow(`
		INSERT INTO plant_activity (plant_id, activity_id, note, date)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, plantID, activityID, note, date).Scan(&activityLogID)
	if err != nil {
		return err
	}

	if err := saveActivityMeasurements(tx, plantID, activityLogID, date, measurements); err != nil {
		return err
	}

	return nil
}

func CreatePlantActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "CreatePlantActivity",
	})

	var input struct {
		PlantID      int                `json:"plant_id"`
		ActivityID   int                `json:"activity_id"`
		Note         string             `json:"note"`
		Date         string             `json:"date"`
		Measurements []measurementInput `json:"measurements"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	fieldLogger = logger.Log.WithFields(logrus.Fields{
		"plant_id":    input.PlantID,
		"activity_id": input.ActivityID,
		"note":        input.Note,
		"date":        input.Date,
	})

	db := DBFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	if err := createPlantActivity(tx, input.PlantID, input.ActivityID, input.Note, input.Date, input.Measurements); err != nil {
		fieldLogger.WithError(err).Error("Failed to create plant activity")
		apiInternalError(c, "api_failed_to_create_activity")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
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
		ID           uint               `json:"id"`
		Date         string             `json:"date"`
		ActivityID   uint               `json:"activity_id"`
		Note         string             `json:"note"`
		Measurements []measurementInput `json:"measurements"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id":          input.ID,
		"activity_id": input.ActivityID,
		"date":        input.Date,
		"note":        input.Note,
	})

	db := DBFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	var plantID int
	query := `UPDATE plant_activity SET date = $1, activity_id = $2, note = $3 WHERE id = $4 RETURNING plant_id`
	err = tx.QueryRow(query, input.Date, input.ActivityID, input.Note, input.ID).Scan(&plantID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiInternalError(c, "api_failed_to_update_activity")
		return
	}

	// This is a pretty lazy approach since we just delete all measurements linked to this activity
	// and re-create them from the input data.
	if err := deleteAllActivityMeasurements(tx, int(input.ID)); err != nil {
		fieldLogger.WithError(err).Error("Failed to delete old activity measurements")
		apiInternalError(c, "api_failed_to_update_activity")
		return
	}

	if err := saveActivityMeasurements(tx, plantID, int(input.ID), input.Date, input.Measurements); err != nil {
		fieldLogger.WithError(err).Error("Failed to save activity measurements")
		apiInternalError(c, "api_failed_to_update_activity")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	fieldLogger.Info("Plant activity updated successfully")
	apiOK(c, "api_activity_updated")
}

func DeleteActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "DeleteActivity",
	})

	id := c.Param("id")
	fieldLogger.WithField("id", id)
	activityID, convErr := strconv.Atoi(id)
	if convErr != nil {
		fieldLogger.WithError(convErr).Error("Invalid activity ID")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	db := DBFromContext(c)
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start transaction")
		apiInternalError(c, "api_failed_to_start_tx")
		return
	}
	defer tx.Rollback()

	// Delete linked measurements first
	err = deleteAllActivityMeasurements(tx, activityID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity measurements")
		apiInternalError(c, "api_failed_to_delete_activity")
		return
	}

	// Then delete the activity itself
	_, err = tx.Exec("DELETE FROM plant_activity WHERE id = $1", activityID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		apiInternalError(c, "api_failed_to_delete_activity")
		return
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	fieldLogger.Info("Plant activity deleted successfully")
	apiOK(c, "api_activity_deleted")
}

func RecordMultiPlantActivity(c *gin.Context) {
	fieldLogger := logger.Log.WithFields(logrus.Fields{
		"handler": "RecordMultiPlantActivity",
	})

	var request struct {
		PlantIDs     []int              `json:"plant_ids"`
		ActivityID   int                `json:"activity_id"`
		Note         string             `json:"note"`
		Date         string             `json:"date"`
		Measurements []measurementInput `json:"measurements"`
	}

	if err := c.BindJSON(&request); err != nil {
		fieldLogger.WithError(err).Error("Failed to bind JSON")
		apiBadRequest(c, "api_invalid_request")
		return
	}

	if len(request.PlantIDs) == 0 {
		fieldLogger.Error("No plants selected")
		apiBadRequest(c, "api_no_plants_selected")
		return
	}

	fieldLogger.WithFields(logrus.Fields{
		"activity_id": request.ActivityID,
		"note":        request.Note,
		"date":        request.Date,
		"plant_ids":   request.PlantIDs,
	})

	db := DBFromContext(c)

	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to start transaction")
		apiInternalError(c, "api_failed_to_save_activity")
		return
	}
	defer tx.Rollback()

	for _, plantID := range request.PlantIDs {
		if err := createPlantActivity(tx, plantID, request.ActivityID, request.Note, request.Date, request.Measurements); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Failed to create activity for plant")
			apiInternalError(c, "api_failed_to_save_activity")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit multi activity transaction")
		apiInternalError(c, "api_failed_to_commit_tx")
		return
	}

	fieldLogger.Info("Activities recorded successfully for multiple plants")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
