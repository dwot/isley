package config

type ActivityResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type MetricResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type StatusResponse struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

type StrainResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Breeder     string `json:"breeder"`
	BreederID   int    `json:"breeder_id"`
	Indica      int    `json:"indica"`
	Sativa      int    `json:"sativa"`
	Autoflower  string `json:"autoflower"`
	Description string `json:"description"`
	SeedCount   int    `json:"seed_count"`
}

type BreederResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ZoneResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

var (
	PollingInterval = 60 // Default polling interval
	ACIEnabled      = 0  // Default ACI enabled
	ECEnabled       = 0  // Default EC enabled
	ACIToken        = "" // Default ACI token
	ECDevices       []string
	Activities      []ActivityResponse
	Metrics         []MetricResponse
	Statuses        []StatusResponse
	Zones           []ZoneResponse
	Strains         []StrainResponse
	Breeders        []BreederResponse
)
