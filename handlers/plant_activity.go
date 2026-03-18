package handlers

import (
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
		apiBadRequest(c, "api_invalid_input")
		return
	}

	fieldLogger = logger.Log.WithFields(logrus.Fields{
		"plant_id":    input.PlantID,
		"activity_id": input.ActivityID,
		"note":        input.Note,
		"date":        input.Date,
	})

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "api_database_error")
		return
	}

	_, err = db.Exec("INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)", input.PlantID, input.ActivityID, input.Note, input.Date)
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

	logger.Log.WithFields(logrus.Fields{
		"id":          input.ID,
		"activity_id": input.ActivityID,
		"date":        input.Date,
		"note":        input.Note,
	})

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "api_database_error")
		return
	}

	query := `UPDATE plant_activity SET date = $1, activity_id = $2, note = $3 WHERE id = $4`
	_, err = db.Exec(query, input.Date, input.ActivityID, input.Note, input.ID)
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

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "api_database_error")
		return
	}

	query := `DELETE FROM plant_activity WHERE id = $1`
	_, err = db.Exec(query, id)
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

	fieldLogger.WithFields(logrus.Fields{
		"activity_id": request.ActivityID,
		"note":        request.Note,
		"date":        request.Date,
		"plant_ids":   request.PlantIDs,
	})

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to open database")
		apiInternalError(c, "api_database_error")
		return
	}

	for _, plantID := range request.PlantIDs {
		_, err = db.Exec(`INSERT INTO plant_activity (plant_id, activity_id, note, date) VALUES ($1, $2, $3, $4)`,
			plantID, request.ActivityID, request.Note, request.Date)
		if err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Failed to insert activity for plant")
			apiInternalError(c, "api_failed_to_save_activity")
			return
		}
	}

	fieldLogger.Info("Activities recorded successfully for multiple plants")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
