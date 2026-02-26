package handlers

import (
	"encoding/json"
	"isley/logger"
	"isley/model"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetOverlayData returns a JSON snapshot of living plants and sensor readings
// suitable for use in a live stream overlay. Requires API key authentication.
func GetOverlayData(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"plants":   GetLivingPlants(),
		"sensors":  GetGroupedSensorsWithLatestReading(),
		"moisture": GetPlantMoistureReadings(),
	})
}

// PlantMoistureReading represents a single sensor reading linked to a plant.
type PlantMoistureReading struct {
	PlantName  string  `json:"plant_name"`
	PlantDay   int     `json:"plant_day"`
	PlantWeek  int     `json:"plant_week"`
	SensorName string  `json:"sensor_name"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
	Trend      string  `json:"trend"`
}

// GetPlantMoistureReadings returns the latest sensor readings for all sensors
// linked to living plants, along with plant name and age.
func GetPlantMoistureReadings() []PlantMoistureReading {
	fieldLogger := logger.Log.WithField("func", "GetPlantMoistureReadings")

	db, err := model.GetDB()
	if err != nil {
		fieldLogger.WithError(err).Error("Error opening database")
		return nil
	}

	// Query living plants that have sensors linked
	rows, err := db.Query(`
SELECT
    p.id,
    p.name,
    p.sensors,
    CAST((julianday('now', 'localtime') - julianday(p.start_dt)) / 7 + 1 AS INT) AS current_week,
    CAST((julianday('now', 'localtime') - julianday(p.start_dt)) + 1 AS INT) AS current_day
FROM plant p
JOIN plant_status_log psl ON p.id = psl.plant_id
JOIN plant_status ps ON psl.status_id = ps.id
WHERE ps.active = 1
  AND psl.date = (SELECT MAX(date) FROM plant_status_log WHERE plant_id = p.id)
  AND p.sensors IS NOT NULL
  AND p.sensors != '[]'
  AND p.sensors != ''
ORDER BY p.name
`)
	if err != nil {
		fieldLogger.WithError(err).Error("Error querying plants")
		return nil
	}
	defer rows.Close()

	var results []PlantMoistureReading

	for rows.Next() {
		var plantID, currentWeek, currentDay int
		var plantName, sensorsJSON string

		if err := rows.Scan(&plantID, &plantName, &sensorsJSON, &currentWeek, &currentDay); err != nil {
			fieldLogger.WithError(err).Error("Error scanning plant row")
			continue
		}

		var sensorIDs []int
		if err := json.Unmarshal([]byte(sensorsJSON), &sensorIDs); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Error parsing sensor IDs")
			continue
		}

		for _, sensorID := range sensorIDs {
			var sensorName, unit, trend string
			var value float64

			err := db.QueryRow(`
SELECT
    s.name,
    s.unit,
    sd.value,
    CASE
        WHEN sd.value > ra.avg_value THEN 'up'
        WHEN sd.value < ra.avg_value THEN 'down'
        ELSE 'flat'
    END AS trend
FROM sensors s
JOIN sensor_data sd ON s.id = sd.sensor_id
LEFT JOIN rolling_averages ra ON ra.sensor_id = s.id AND ra.create_dt = sd.create_dt
WHERE s.id = ?
  AND sd.id = (SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)
`, sensorID).Scan(&sensorName, &unit, &value, &trend)
			if err != nil {
				fieldLogger.WithError(err).WithField("sensor_id", sensorID).Error("Error querying sensor reading")
				continue
			}

			results = append(results, PlantMoistureReading{
				PlantName:  plantName,
				PlantDay:   currentDay,
				PlantWeek:  currentWeek,
				SensorName: sensorName,
				Value:      value,
				Unit:       unit,
				Trend:      trend,
			})
		}
	}

	return results
}
