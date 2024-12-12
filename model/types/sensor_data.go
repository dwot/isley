package types

import "time"

// SensorData defines the structure for the sensor_data table
type SensorData struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	SensorID int       `json:"sensor_id"`
	Value    float64   `json:"value"`
	CreateDT time.Time `json:"create_dt"`
}
