package types

import "time"

// PlantStatusLog represents the structure of the plant_status_log table
type PlantStatusLog struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	PlantID  uint      `json:"plant_id" gorm:"not null"`
	StatusID uint      `json:"status_id" gorm:"not null"`
	Date     time.Time `json:"date" gorm:"not null;default:CURRENT_TIMESTAMP"`
}
