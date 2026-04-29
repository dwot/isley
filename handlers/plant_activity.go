package handlers

import (
	"isley/logger"
	"isley/utils"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
		apiBadRequest(c, "api_invalid_input")
		return
	}

	if err := utils.ValidateStringLength("note", input.Note, utils.MaxNotesLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateDate("date", input.Date); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	fieldLogger = logger.Log.WithFields(logrus.Fields{
		"plant_id":    input.PlantID,
		"activity_id": input.ActivityID,
		"note":        input.Note,
		"date":        input.Date,
	})

	db := DBFromContext(c)

	_, err := db.Exec("INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)", input.PlantID, input.ActivityID, input.Note, input.Date)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to insert activity into database")
		apiInternalError(c, "api_failed_to_create_activity")
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
		apiBadRequest(c, "api_invalid_input")
		return
	}

	if err := utils.ValidateStringLength("note", input.Note, utils.MaxNotesLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateDate("date", input.Date); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	logger.Log.WithFields(logrus.Fields{
		"id":          input.ID,
		"activity_id": input.ActivityID,
		"date":        input.Date,
		"note":        input.Note,
	})

	db := DBFromContext(c)

	query := `UPDATE plant_activity SET date = $1, activity_id = $2, note = $3 WHERE id = $4`
	_, err := db.Exec(query, input.Date, input.ActivityID, input.Note, input.ID)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update activity")
		apiInternalError(c, "api_failed_to_update_activity")
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

	db := DBFromContext(c)

	query := `DELETE FROM plant_activity WHERE id = $1`
	_, err := db.Exec(query, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete activity")
		apiInternalError(c, "api_failed_to_delete_activity")
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
		PlantIDs   []int  `json:"plant_ids"`
		ActivityID int    `json:"activity_id"`
		Note       string `json:"note"`
		Date       string `json:"date"`
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

	if err := utils.ValidateStringLength("note", request.Note, utils.MaxNotesLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateDate("date", request.Date); err != nil {
		apiBadRequest(c, err.Error())
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
		fieldLogger.WithError(err).Error("Failed to begin transaction")
		apiInternalError(c, "api_failed_to_save_activity")
		return
	}
	defer tx.Rollback() // no-op once Commit succeeds

	stmt, err := tx.Prepare(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to prepare activity insert")
		apiInternalError(c, "api_failed_to_save_activity")
		return
	}
	defer stmt.Close()

	for _, plantID := range request.PlantIDs {
		if _, err := stmt.Exec(plantID, request.ActivityID, request.Note, request.Date); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Failed to insert activity for plant")
			apiInternalError(c, "api_failed_to_save_activity")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to commit activity inserts")
		apiInternalError(c, "api_failed_to_save_activity")
		return
	}

	fieldLogger.Info("Activities recorded successfully for multiple plants")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
