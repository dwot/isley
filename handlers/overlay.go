package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetOverlayData returns a JSON snapshot of living plants and sensor readings
// suitable for use in a live stream overlay. Requires API key authentication.
func GetOverlayData(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"plants":  GetLivingPlants(),
		"sensors": GetGroupedSensorsWithLatestReading(),
	})
}
