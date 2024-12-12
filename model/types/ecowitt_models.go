package types

// Define the structures to match the JSON response
type ECWWH25 struct {
	InTemp string `json:"intemp"`
	Unit   string `json:"unit"`
	InHumi string `json:"inhumi"`
	Abs    string `json:"abs"`
	Rel    string `json:"rel"`
}

type ECWCHSoil struct {
	Channel  string `json:"channel"`
	Name     string `json:"name"`
	Battery  string `json:"battery"`
	Humidity string `json:"humidity"`
}

type ECWAPIResponse struct {
	WH25   []ECWWH25   `json:"wh25"`
	CHSoil []ECWCHSoil `json:"ch_soil"`
}
