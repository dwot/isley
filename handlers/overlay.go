package handlers

import (
	"encoding/json"
	"isley/logger"
	"isley/model"
	"isley/model/types"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetOverlayData returns a JSON snapshot of living plants (with linked sensor
// readings embedded) and grouped sensor data, suitable for a live stream overlay.
// Requires API key authentication.
func GetOverlayData(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"plants":  GetOverlayPlants(),
		"sensors": GetGroupedSensorsWithLatestReading(),
	})
}

// OverlayLinkedSensor is a sensor reading attached to a plant in the overlay response.
type OverlayLinkedSensor struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Trend  string  `json:"trend"`
	Source string  `json:"source"`
	Type   string  `json:"type"`
}

// OverlayPlantResponse wraps PlantListResponse and adds linked sensor readings.
type OverlayPlantResponse struct {
	types.PlantListResponse
	LinkedSensors []OverlayLinkedSensor `json:"linked_sensors"`
}

// GetOverlayPlants returns living plants with their linked sensor readings embedded.
func GetOverlayPlants() []OverlayPlantResponse {
	fieldLogger := logger.Log.WithField("func", "GetOverlayPlants")

	living := GetLivingPlants()

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return buildOverlayPlants(living, nil, fieldLogger)
	}

	// Query to fetch latest reading + trend + source + type for a single sensor.
	// $1 placeholder works for both SQLite and PostgreSQL in this codebase.
	const sensorQuery = `
SELECT
    s.name,
    s.unit,
    s.device,
    s.type,
    sd.value,
    CASE
        WHEN sd.value > ra.avg_value THEN 'up'
        WHEN sd.value < ra.avg_value THEN 'down'
        ELSE 'flat'
    END AS trend
FROM sensors s
JOIN sensor_data sd ON s.id = sd.sensor_id
LEFT JOIN rolling_averages ra ON ra.sensor_id = s.id AND ra.create_dt = sd.create_dt
WHERE s.id = $1
  AND sd.id = (SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)
`
	return buildOverlayPlants(living, func(plantID int) []OverlayLinkedSensor {
		var sensorsJSON string
		if err := db.QueryRow("SELECT sensors FROM plant WHERE id = $1", plantID).Scan(&sensorsJSON); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Error fetching sensor IDs")
			return []OverlayLinkedSensor{}
		}

		var sensorIDs []int
		if err := json.Unmarshal([]byte(sensorsJSON), &sensorIDs); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Error parsing sensor IDs")
			return []OverlayLinkedSensor{}
		}

		sensors := []OverlayLinkedSensor{}
		for _, sid := range sensorIDs {
			var s OverlayLinkedSensor
			err := db.QueryRow(sensorQuery, sid).Scan(&s.Name, &s.Unit, &s.Source, &s.Type, &s.Value, &s.Trend)
			if err != nil {
				fieldLogger.WithError(err).WithField("sensor_id", sid).Error("Error querying sensor reading")
				continue
			}
			sensors = append(sensors, s)
		}
		return sensors
	}, fieldLogger)
}

func buildOverlayPlants(living []types.PlantListResponse, fetchSensors func(int) []OverlayLinkedSensor, _ interface{}) []OverlayPlantResponse {
	result := make([]OverlayPlantResponse, 0, len(living))
	for _, p := range living {
		linked := []OverlayLinkedSensor{}
		if fetchSensors != nil {
			linked = fetchSensors(p.ID)
		}
		result = append(result, OverlayPlantResponse{
			PlantListResponse: p,
			LinkedSensors:     linked,
		})
	}
	return result
}
