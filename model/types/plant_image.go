package types

import "time"

// PlantImage represents the structure of the plant_images table
type PlantImage struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	PlantID          uint      `json:"plant_id" gorm:"not null"`
	ImagePath        string    `json:"image_path" gorm:"not null"`
	ImageDescription string    `json:"image_description" gorm:"not null"`
	ImageOrder       int       `json:"image_order" gorm:"not null"`
	ImageDate        time.Time `json:"image_date" gorm:"not null;default:CURRENT_TIMESTAMP"`
	CreatedAt        time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt        time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
}
