package types

type ACIResponse struct {
	Data []ACIDeviceData `json:"data"`
}

type ACIDeviceData struct {
	DevCode    string        `json:"devCode"`
	DeviceInfo ACIDeviceInfo `json:"deviceInfo"`
}

type ACIDeviceInfo struct {
	TemperatureF int         `json:"temperatureF"`
	Temperature  int         `json:"temperature"`
	Humidity     int         `json:"humidity"`
	Ports        []ACIPort   `json:"ports"`
	Sensors      []ACISensor `json:"sensors"`
	Unit         int         `json:"unit"` // 0 = °F, 1 = °C (added to allow unit-aware defaults)
}

type ACIPort struct {
	PortName string `json:"portName"`
	Speak    int    `json:"speak"`
	Port     int    `json:"port"`
	CurMode  int    `json:"curMode"`
	Online   int    `json:"online"`
}

type ACISensor struct {
	SensorType int `json:"sensorType"`
	AccessPort int `json:"accessPort"`
	SensorData int `json:"sensorData"`
	SensorUnit int `json:"sensorUnit"` // optional: unit reported per sensor
}
