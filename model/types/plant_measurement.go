package types

import "time"

// PlantMeasurement represents the structure of the plant_measurements table
type PlantMeasurement struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	PlantID  uint      `json:"plant_id" gorm:"not null"`
	MetricID uint      `json:"metric_id" gorm:"not null"`
	Value    float64   `json:"value" gorm:"not null"`
	Date     time.Time `json:"date" gorm:"not null;default:CURRENT_TIMESTAMP"`
}
