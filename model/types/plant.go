package types

import "time"

// Plant represents the structure of the plant table
type Plant struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"not null"`
	StatusID    uint      `json:"status_id" gorm:"not null"`
	Description string    `json:"description" gorm:"not null"`
	Clone       int       `json:"clone" gorm:"not null"`
	StrainID    uint      `json:"strain_id" gorm:"not null"`
	ZoneID      uint      `json:"zone_id" gorm:"not null"`
	StartDT     time.Time `json:"start_dt" gorm:"not null"`
}
