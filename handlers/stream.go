package handlers

import (
	"database/sql"
	"fmt"
	"isley/config"
	"isley/logger"
	"isley/model/types"
	"isley/utils"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func GetStreams(db *sql.DB) []types.Stream {
	streams := []types.Stream{}
	fieldLogger := logger.Log.WithField("func", "GetStreams")
	rows, err := db.Query("SELECT s.id, s.name, url, zone_id, visible, z.name as zone_name FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read stream")
		return streams
	}
	defer rows.Close()

	stream := types.Stream{}
	for rows.Next() {
		var id, zoneID uint
		var visible bool
		var name, url, zoneName string
		err = rows.Scan(&id, &name, &url, &zoneID, &visible, &zoneName)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read stream")
			continue
		}
		stream = types.Stream{ID: id, Name: name, URL: url, ZoneID: zoneID, ZoneName: zoneName, Visible: visible}
		streams = append(streams, stream)
	}

	return streams
}

func AddStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "AddStreamHandler")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  string `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("stream_name", stream.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStreamURL(stream.URL); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Add stream to database
	db := DBFromContext(c)

	// Insert new stream and return new id
	var id int
	err := db.QueryRow("INSERT INTO streams (name, url, zone_id, visible) VALUES ($1, $2, $3, $4) RETURNING id", stream.Name, stream.URL, stream.ZoneID, stream.Visible).Scan(&id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to add stream")
		apiInternalError(c, "api_failed_to_add_stream")
		return
	}

	streams := GetStreams(db)
	config.Streams = streams

	latestFileName := fmt.Sprintf("stream_%d_latest%s", id, filepath.Ext(".jpg"))
	latestSavePath := filepath.Join("uploads", "streams", latestFileName)
	utils.GrabWebcamImage(stream.URL, latestSavePath)

	c.JSON(http.StatusCreated, gin.H{"id": id, "streams": streams})
}

func UpdateStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "UpdateStreamHandler")
	id := c.Param("id")
	var stream struct {
		Name    string `json:"stream_name"`
		URL     string `json:"url"`
		ZoneID  int    `json:"zone_id"`
		Visible bool   `json:"visible"`
	}
	if err := c.ShouldBindJSON(&stream); err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if err := utils.ValidateRequiredString("stream_name", stream.Name, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStreamURL(stream.URL); err != nil {
		apiBadRequest(c, err.Error())
		return
	}

	// Update stream in database
	db := DBFromContext(c)

	//convert visible to int
	var visibleInt int
	if stream.Visible {
		visibleInt = 1
	} else {
		visibleInt = 0
	}

	// Update stream in database
	_, err := db.Exec("UPDATE streams SET name = $1, url = $2, zone_id = $3, visible = $4 WHERE id = $5", stream.Name, stream.URL, stream.ZoneID, visibleInt, id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to update stream")
		apiInternalError(c, "api_failed_to_update_stream")
		return
	}

	streams := GetStreams(db)
	config.Streams = streams

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_updated"), "streams": streams})
}

func DeleteStreamHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamHandler")
	id := c.Param("id")

	// Delete stream from database
	db := DBFromContext(c)

	// Delete stream from database
	_, err := db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to delete stream")
		apiInternalError(c, "api_failed_to_delete_stream")
		return
	}

	streams := GetStreams(db)
	config.Streams = streams

	c.JSON(http.StatusOK, gin.H{"message": T(c, "api_stream_deleted"), "streams": streams})
}

func GetStreamsByZoneHandler(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "GetStreamsByZoneHandler")
	db := DBFromContext(c)

	rows, err := db.Query("SELECT s.id, s.name, s.url, z.name as zone_name, visible FROM streams s left outer join zones z on s.zone_id = z.id")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read streams")
		apiInternalError(c, "api_failed_to_read_streams")
		return
	}
	defer rows.Close()

	streamsByZone := make(map[string][]types.Stream)
	for rows.Next() {
		var id int
		var name, url, zoneName string
		var visible bool
		err = rows.Scan(&id, &name, &url, &zoneName, &visible)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read streams")
			apiInternalError(c, "api_failed_to_read_streams")
			return
		}
		streamsByZone[zoneName] = append(streamsByZone[zoneName], types.Stream{ID: uint(id), Name: name, URL: url, ZoneName: zoneName, Visible: visible})
	}

	c.JSON(http.StatusOK, streamsByZone)
}

func DeleteStreamByID(db *sql.DB, id string) error {
	fieldLogger := logger.Log.WithField("func", "DeleteStreamByID")

	_, err := db.Exec("DELETE FROM streams WHERE id = $1", id)
	if err != nil {
		fieldLogger.WithError(err).Error("Error deleting sensor")
		return err
	}
	return nil
}
