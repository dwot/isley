package handlers

import (
	"database/sql"
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
	db := DBFromContext(c)
	c.JSON(http.StatusOK, gin.H{
		"plants":  GetOverlayPlants(db),
		"sensors": GetGroupedSensorsWithLatestReading(db, SensorCacheServiceFromContext(c)),
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
// Uses two batch queries instead of per-plant/per-sensor queries to avoid N+1.
func GetOverlayPlants(db *sql.DB) []OverlayPlantResponse {
	fieldLogger := logger.Log.WithField("func", "GetOverlayPlants")

	living := GetLivingPlants(db)
	if len(living) == 0 {
		return []OverlayPlantResponse{}
	}

	driver := "sqlite"
	if model.IsPostgres() {
		driver = "postgres"
	}

	// --- Batch query 1: fetch sensor ID lists for all living plants ---
	plantIDs := make([]interface{}, len(living))
	for i, p := range living {
		plantIDs[i] = p.ID
	}
	inClause, inArgs := model.BuildInClause(driver, plantIDs)

	plantRows, err := db.Query("SELECT id, sensors FROM plant WHERE id IN "+inClause, inArgs...)
	if err != nil {
		fieldLogger.WithError(err).Error("Error batch-fetching plant sensors")
		return wrapPlantsNoSensors(living)
	}
	defer plantRows.Close()

	plantSensorMap := make(map[int][]int) // plantID → []sensorID
	sensorIDSet := make(map[int]struct{})
	for plantRows.Next() {
		var plantID int
		var sensorsJSON sql.NullString
		if err := plantRows.Scan(&plantID, &sensorsJSON); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Error scanning plant sensors")
			continue
		}
		if !sensorsJSON.Valid || sensorsJSON.String == "" || sensorsJSON.String == "null" {
			continue
		}
		var sensorIDs []int
		if err := json.Unmarshal([]byte(sensorsJSON.String), &sensorIDs); err != nil {
			fieldLogger.WithError(err).WithField("plant_id", plantID).Error("Error parsing sensor IDs JSON")
			continue
		}
		plantSensorMap[plantID] = sensorIDs
		for _, sid := range sensorIDs {
			sensorIDSet[sid] = struct{}{}
		}
	}

	if len(sensorIDSet) == 0 {
		return wrapPlantsNoSensors(living)
	}

	// --- Batch query 2: fetch latest reading + trend for all linked sensors ---
	uniqueIDs := make([]interface{}, 0, len(sensorIDSet))
	for sid := range sensorIDSet {
		uniqueIDs = append(uniqueIDs, sid)
	}
	sensorInClause, sensorArgs := model.BuildInClause(driver, uniqueIDs)

	sensorQuery := `
SELECT
    s.id,
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
LEFT JOIN rolling_averages ra ON ra.sensor_id = s.id
WHERE s.id IN ` + sensorInClause + `
  AND sd.id = (SELECT MAX(id) FROM sensor_data WHERE sensor_id = s.id)`

	sensorRows, err := db.Query(sensorQuery, sensorArgs...)
	if err != nil {
		fieldLogger.WithError(err).Error("Error batch-fetching sensor readings")
		return wrapPlantsNoSensors(living)
	}
	defer sensorRows.Close()

	sensorMap := make(map[int]OverlayLinkedSensor) // sensorID → reading
	for sensorRows.Next() {
		var sid int
		var s OverlayLinkedSensor
		if err := sensorRows.Scan(&sid, &s.Name, &s.Unit, &s.Source, &s.Type, &s.Value, &s.Trend); err != nil {
			fieldLogger.WithError(err).WithField("sensor_id", sid).Error("Error scanning sensor reading")
			continue
		}
		sensorMap[sid] = s
	}

	// --- Assemble: attach sensor readings to each plant ---
	result := make([]OverlayPlantResponse, 0, len(living))
	for _, p := range living {
		linked := make([]OverlayLinkedSensor, 0)
		if sensorIDs, ok := plantSensorMap[p.ID]; ok {
			for _, sid := range sensorIDs {
				if s, ok := sensorMap[sid]; ok {
					linked = append(linked, s)
				}
			}
		}
		result = append(result, OverlayPlantResponse{
			PlantListResponse: p,
			LinkedSensors:     linked,
		})
	}
	return result
}

// wrapPlantsNoSensors wraps each plant with an empty sensor list.
func wrapPlantsNoSensors(living []types.PlantListResponse) []OverlayPlantResponse {
	result := make([]OverlayPlantResponse, 0, len(living))
	for _, p := range living {
		result = append(result, OverlayPlantResponse{
			PlantListResponse: p,
			LinkedSensors:     []OverlayLinkedSensor{},
		})
	}
	return result
}
