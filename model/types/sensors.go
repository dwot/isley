package types

import "time"

// Sensor represents the structure of the sensors table
type Sensor struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	Name     string    `json:"name" gorm:"not null"`
	ZoneID   uint      `json:"zone_id" gorm:"not null"` // Foreign key to zones table
	Source   string    `json:"source" gorm:"not null"`  // Integration type (e.g., acinfinity, ecowitt)
	Device   string    `json:"device" gorm:"not null"`  // Device unique ID from source
	Type     string    `json:"type" gorm:"not null"`    // Sensor type (e.g., temperature, humidity)
	CreateDT time.Time `json:"create_dt" gorm:"autoCreateTime"`
	UpdateDT time.Time `json:"update_dt" gorm:"autoUpdateTime"`
}
