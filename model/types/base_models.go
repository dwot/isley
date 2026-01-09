package types

import (
	"time"
)

type PlantActivity struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	Note       string    `json:"note"`
	Date       time.Time `json:"date"`
	ActivityId int       `json:"activity_id"`
}

type Activity struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Breeder struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Measurement struct {
	ID    uint      `json:"id"`
	Name  string    `json:"name"`
	Value float64   `json:"value"`
	Date  time.Time `json:"date"`
}

type Metric struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type Plant struct {
	ID             uint                 `json:"id"`
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Status         string               `json:"status"`
	StatusID       int                  `json:"status_id"`
	StrainName     string               `json:"strain_name"`
	StrainID       int                  `json:"strain_id"`
	BreederName    string               `json:"breeder_name"`
	ZoneName       string               `json:"zone_name"`
	ZoneID         int                  `json:"zone_id"`
	CurrentDay     int                  `json:"current_day"`
	CurrentWeek    int                  `json:"current_week"`
	CurrentHeight  string               `json:"current_height"`
	HeightDate     time.Time            `json:"height_date"`
	LastWaterDate  time.Time            `json:"last_water_date"`
	LastFeedDate   time.Time            `json:"last_feed_date"`
	Measurements   []Measurement        `json:"measurements"`
	Activities     []PlantActivity      `json:"activities"`
	StatusHistory  []Status             `json:"status_history"`
	Sensors        []SensorDataResponse `json:"sensors"`
	LatestImage    PlantImage           `json:"latest_image"`
	Images         []PlantImage         `json:"images"`
	IsClone        bool                 `json:"is_clone"`
	StartDT        time.Time            `json:"start_dt"`
	HarvestWeight  float64              `json:"harvest_weight"`
	HarvestDate    time.Time            `json:"harvest_date"`
	CycleTime      int                  `json:"cycle_time"`
	StrainUrl      string               `json:"strain_url"`
	EstHarvestDate time.Time            `json:"est_harvest_date"`
	Autoflower     bool                 `json:"autoflower"`
	ParentID       uint                 `json:"parent_id"`
	ParentName     string               `json:"parent_name"`
}

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

type PlantListResponse struct {
	ID                    int       `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description"`
	Clone                 bool      `json:"clone"`
	StrainName            string    `json:"strain_name"`
	BreederName           string    `json:"breeder_name"`
	ZoneName              string    `json:"zone_name"`
	StartDT               string    `json:"start_dt"`
	CurrentWeek           int       `json:"current_week"`
	CurrentDay            int       `json:"current_day"`
	DaysSinceLastWatering int       `json:"days_since_last_watering"`
	DaysSinceLastFeeding  int       `json:"days_since_last_feeding"`
	FloweringDays         *int      `json:"flowering_days,omitempty"` // nil if not flowering
	HarvestWeight         float64   `json:"harvest_weight"`
	Status                string    `json:"status"`
	StatusDate            time.Time `json:"status_date"`
	CycleTime             int       `json:"cycle_time"`
	StrainUrl             string    `json:"strain_url"`
	EstHarvestDate        time.Time `json:"est_harvest_date"`
	Autoflower            bool      `json:"autoflower"`
	HarvestDate           time.Time `json:"harvest_date"`
}

type Sensor struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Zone     string `json:"zone"`
	Source   string `json:"source"`
	Device   string `json:"device"`
	Type     string `json:"type"`
	Show     bool   `json:"show"`
	Unit     string `json:"unit"`
	CreateDT string `json:"create_dt"`
	UpdateDT string `json:"update_dt"`
}

// SensorData defines the structure for the sensor_data table
type SensorData struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	SensorID   int       `json:"sensor_id"`
	SensorName string    `json:"sensor_name"`
	Value      float64   `json:"value"`
	CreateDT   time.Time `json:"create_dt"`
}

type SensorDataResponse struct {
	ID    uint      `json:"id"`
	Name  string    `json:"name"`
	Unit  string    `json:"unit"`
	Value float64   `json:"value"`
	Date  time.Time `json:"date"`
}

type Settings struct {
	ACI struct {
		Enabled bool `json:"enabled"`
	} `json:"aci"`
	EC struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
	} `json:"ec"`
	PollingInterval    string `json:"polling_interval"`
	GuestMode          bool   `json:"guest_mode"`
	StreamGrabEnabled  bool   `json:"stream_grab_enabled"`
	StreamGrabInterval string `json:"stream_grab_interval"`
	APIKey             string `json:"api_key"`
	// New: allow disabling API ingest from settings form
	DisableAPIIngest bool `json:"disable_api_ingest"`
}

type ACInfinitySettings struct {
	Enabled  bool `json:"enabled"`
	TokenSet bool `json:"token_set"`
}

type EcoWittSettings struct {
	Enabled bool `json:"enabled"`
}

type SettingsData struct {
	ACI                ACInfinitySettings `json:"aci"`
	EC                 EcoWittSettings    `json:"ec"`
	PollingInterval    int                `json:"polling_interval"`
	GuestMode          bool               `json:"guest_mode"`
	StreamGrabEnabled  bool               `json:"stream_grab_enabled"`
	StreamGrabInterval int                `json:"stream_grab_interval"`
	APIKey             string             `json:"api_key"`
	// New: reflect whether API ingest is enabled (true) or disabled (false)
	APIIngestEnabled bool `json:"api_ingest_enabled"`
}

type Status struct {
	ID     uint      `json:"id"`
	Status string    `json:"status"`
	Date   time.Time `json:"date"`
}

type Strain struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Breeder          string `json:"breeder"`
	BreederID        int    `json:"breeder_id"`
	Indica           int    `json:"indica"`
	Sativa           int    `json:"sativa"`
	Autoflower       bool   `json:"autoflower"`
	Description      string `json:"description"`
	SeedCount        int    `json:"seed_count"`
	CycleTime        int    `json:"cycle_time"`
	Url              string `json:"url"`
	ShortDescription string `json:"short_desc"`
}

type Zone struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type Stream struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	ZoneID   uint   `json:"zone_id"`
	ZoneName string `json:"zone_name"`
	Visible  bool   `json:"visible"`
}
