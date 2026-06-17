package types

import (
	"fmt"
	"strconv"
	"strings"
)

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

type ECWCHAisle struct {
	Channel  string `json:"channel"`
	Name     string `json:"name"`
	Battery  string `json:"battery"`
	Temp     string `json:"temp"`
	Unit     string `json:"unit"`
	Humidity string `json:"humidity"`
}

// ECWCHEc holds one channel of a WH52 soil sensor, reported by the gateway
// in the ch_ec array (added in EcoWitt local API v1.0.6). Unlike the
// moisture-only WH51 (ch_soil), the WH52 also reports soil temperature and
// electrical conductivity. The Humidity field is soil moisture (named
// "humidity" in the payload, matching ch_soil/ch_aisle). EC carries its unit
// inline (e.g. "470 uS/cm"). Battery/Voltage are parsed but not registered as
// sensors, matching how ch_soil/ch_aisle are handled.
type ECWCHEc struct {
	Channel  string `json:"channel"`
	Name     string `json:"name"`
	Battery  string `json:"battery"`
	Voltage  string `json:"voltage"`
	Humidity string `json:"humidity"`
	Temp     string `json:"temp"`
	Unit     string `json:"unit"`
	EC       string `json:"ec"`
}

type ECWCommonItem struct {
	ID      string `json:"id"`
	Val     string `json:"val"`
	Unit    string `json:"unit"`
	Battery string `json:"battery"`
}

// ECWCommonSensors maps normalised lowercase hex ID strings to sensor
// metadata used during both scan (registration) and poll (data ingestion).
// IDs 0x01/0x06 (indoor T&H — duplicates of wh25) and 0x18 (date/time
// string) are intentionally absent.
var ECWCommonSensors = map[string]struct {
	TypeKey string
	Name    string
	Unit    string
}{
	"0x02": {"Common.0x02", "EC (%s) Outdoor Temp",     "°F"},
	"0x03": {"Common.0x03", "EC (%s) Dew Point",        "°F"},
	"0x04": {"Common.0x04", "EC (%s) Wind Chill",       "°F"},
	"0x05": {"Common.0x05", "EC (%s) Heat Index",       "°F"},
	"0x07": {"Common.0x07", "EC (%s) Outdoor Humidity", "%"},
	"0x08": {"Common.0x08", "EC (%s) Abs Pressure",     "inHg"},
	"0x09": {"Common.0x09", "EC (%s) Rel Pressure",     "inHg"},
	"0x0a": {"Common.0x0a", "EC (%s) Wind Direction",   "°"},
	"0x0b": {"Common.0x0b", "EC (%s) Wind Speed",       "mph"},
	"0x0c": {"Common.0x0c", "EC (%s) Gust Speed",       "mph"},
	"0x0d": {"Common.0x0d", "EC (%s) Rain Event",       "in"},
	"0x0e": {"Common.0x0e", "EC (%s) Rain Rate",        "in/hr"},
	"0x0f": {"Common.0x0f", "EC (%s) Rain Gain",        "in"},
	"0x10": {"Common.0x10", "EC (%s) Rain Day",         "in"},
	"0x11": {"Common.0x11", "EC (%s) Rain Week",        "in"},
	"0x12": {"Common.0x12", "EC (%s) Rain Month",       "in"},
	"0x13": {"Common.0x13", "EC (%s) Rain Year",        "in"},
	"0x14": {"Common.0x14", "EC (%s) Rain Total",       "in"},
	"0x15": {"Common.0x15", "EC (%s) Light",            "W/m²"},
	"0x16": {"Common.0x16", "EC (%s) UV",               "W/m²"},
	"0x17": {"Common.0x17", "EC (%s) UVI",              ""},
	"0x19": {"Common.0x19", "EC (%s) Day Max Wind",     "mph"},
}

// NormalizeECWID converts a common_list id field to lowercase hex form
// for lookup in ECWCommonSensors. Handles both hex-string ("0x02",
// "0x0A") and decimal-string ("3") representations found in real
// gateway payloads.
func NormalizeECWID(id string) string {
	s := strings.ToLower(strings.TrimSpace(id))
	if strings.HasPrefix(s, "0x") {
		return s
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return s
	}
	return fmt.Sprintf("0x%02x", n)
}

type ECWAPIResponse struct {
	WH25       []ECWWH25       `json:"wh25"`
	CHSoil     []ECWCHSoil     `json:"ch_soil"`
	CHAisle    []ECWCHAisle    `json:"ch_aisle"`
	CHEc       []ECWCHEc       `json:"ch_ec"`
	CommonList []ECWCommonItem `json:"common_list"`
}
