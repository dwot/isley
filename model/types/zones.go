package types

import "time"

type Zones struct {
	ID       uint      `json:"id" gorm:"primaryKey"`
	Name     string    `json:"name"`
	CreateDT time.Time `json:"create_dt"`
	UpdateDT time.Time `json:"update_dt"`
}
