package types

// PlantStatus represents the structure of the plant_status table
type PlantStatus struct {
	ID     uint   `json:"id" gorm:"primaryKey"`
	Status string `json:"status" gorm:"not null"`
	Active int    `json:"active" gorm:"not null"`
}
