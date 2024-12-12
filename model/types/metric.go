package types

// Metric represents the structure of the metric table
type Metric struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"not null"`
	Unit string `json:"unit" gorm:"not null"`
}
