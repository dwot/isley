package types

import "time"

// Setting represents the structure of the settings table
type Setting struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	Name     string    `json:"name" gorm:"not null"`
	Value    string    `json:"value" gorm:"not null"`
	CreateDT time.Time `json:"create_dt" gorm:"autoCreateTime"`
	UpdateDT time.Time `json:"update_dt" gorm:"autoUpdateTime"`
}
