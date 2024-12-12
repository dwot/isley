package handlers

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	model "isley/model"
	"net/http"
)

func EditStatus(c *gin.Context) {
	var input struct {
		ID   uint   `json:"id"`
		Date string `json:"date"`
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

	query := `UPDATE plant_status_log SET date = ? WHERE id = ?`
	_, err = db.Exec(query, input.Date, input.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated successfully"})
}

func DeleteStatus(c *gin.Context) {
	id := c.Param("id")

	db, err := sql.Open("sqlite", model.DbPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to database"})
		return
	}
	defer db.Close()

	query := `DELETE FROM plant_status_log WHERE id = ?`
	_, err = db.Exec(query, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status deleted successfully"})
}
