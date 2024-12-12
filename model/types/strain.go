package types

// Strain represents the structure of the strain table
type Strain struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"not null"`
	Breeder     string `json:"breeder" gorm:"not null"`
	Sativa      int    `json:"sativa" gorm:"not null"`
	Indica      int    `json:"indica" gorm:"not null"`
	Autoflower  int    `json:"autoflower" gorm:"not null"`
	Description string `json:"description" gorm:"not null"`
}
