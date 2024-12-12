package types

import "time"

// PlantActivity represents the structure of the plant_activity table
type PlantActivity struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	PlantID    uint      `json:"plant_id" gorm:"not null"`
	ActivityID uint      `json:"activity_id" gorm:"not null"`
	Date       time.Time `json:"date" gorm:"not null;default:CURRENT_TIMESTAMP"`
	Note       string    `json:"note" gorm:"not null"`
}
