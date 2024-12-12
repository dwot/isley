package types

// Activity represents the structure of the activity table
type Activity struct {
	ID   uint   `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"not null"`
}
