package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"isley/logger"
	"isley/model/types"
	"isley/ratelimit"
	"isley/service"
	"isley/utils"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sirupsen/logrus"
)

// GetSensors lists every sensor for the sensor, settings and plant pages,
// in the map shape their scripts read. A sensor with no zone has zone ""
// and zone_id 0.
func GetSensors(ctx context.Context, svc *service.Services) []map[string]interface{} {
	sensors, _, err := svc.Sensors.List(ctx, service.SensorFilter{})
	if err != nil {
		logger.Log.WithError(err).Error("Failed to list sensors")
		return nil
	}
	out := make([]map[string]interface{}, len(sensors))
	for i, sn := range sensors {
		zone, zoneID := "", 0
		if sn.Zone != nil {
			zone, zoneID = sn.Zone.Name, sn.Zone.ID
		}
		out[i] = map[string]interface{}{
			"id": sn.ID, "name": sn.Name, "zone": zone, "zone_id": zoneID,
			"source": sn.Source, "device": sn.Device, "type": sn.Type,
			"visibility": sn.Visibility, "unit": sn.Unit,
			"create_dt": sn.CreatedAt.Format(utils.LayoutDB),
		}
	}
	return out
}

func ScanACInfinitySensors(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "ScanACInfinitySensors")
	var input struct {
		ZoneID  *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone string `json:"new_zone"` // Optional new zone name
	}
	fieldLogger = fieldLogger.WithField("input", input)
	fieldLogger.Info("Scanning AC Infinity sensors")
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		apiBadRequest(c, "api_invalid_input")
		return
	}
	// Validate string lengths
	if err := utils.ValidateStringLength("new_zone", input.NewZone, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	sensors := ServicesFromContext(c).Sensors
	store := ConfigStoreFromContext(c)

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert a new zone into the database
		zone, err := ServicesFromContext(c).Zones.Create(c.Request.Context(), service.ZoneInput{Name: input.NewZone})
		zoneID := zone.ID
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			apiInternalError(c, "api_zone_creation_failed")
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	// Query settings table and write result to console
	aciToken := store.ACIToken()
	url := "https://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + aciToken
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		apiInternalError(c, "api_error_creating_request")
		return
	}

	req.Header.Add("token", aciToken)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	resp, err := httpClient.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		apiError(c, http.StatusBadGateway, "api_error_sending_request_aci")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxSensorScanResponseBytes))
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		apiError(c, http.StatusBadGateway, "api_error_reading_response")
		return
	}

	var jsonResponse types.ACIResponse
	err = json.Unmarshal(respBody, &jsonResponse)
	if err != nil {
		fieldLogger.WithError(err).Error("Error unmarshalling JSON response")
		apiError(c, http.StatusBadGateway, "api_error_invalid_response")
		return
	}

	if len(jsonResponse.Data) > 0 {
		for _, deviceData := range jsonResponse.Data {
			device := deviceData.DevCode
			source := "acinfinity"
			sensorType := "ACI.tempF"
			name := "AC Infinity (" + device + ") Temp"
			unit := "°F"

			//Check to see if exists by type / device / source combo
			checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)

			sensorType = "ACI.humidity"
			name = "AC Infinity (" + device + ") Humidity"
			unit = "%"
			checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)

			sensorType = "ACI.tempC"
			name = "AC Infinity (" + device + ") Temp"
			unit = "°C"
			checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)

			for _, sensor := range deviceData.DeviceInfo.Sensors {
				sensorType := fmt.Sprintf("ACI.%d.%d", sensor.AccessPort, sensor.SensorType)
				switch sensor.SensorType {
				case 0: //Inside Temp
					name := "ACI (" + device + ") inside temp"
					unit := "\u00b0F"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				case 2: //Inside Humidity
					name := "ACI (" + device + ") inside humidity"
					unit := "%"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				case 3: //Inside VPD
					name := "ACI (" + device + ") inside VPD"
					unit := "kPa"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				case 4: //Outside Temp
					name := "ACI (" + device + ") outside temp"
					unit := "\u00b0F"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				case 6: //Outside Humidity
					name := "ACI (" + device + ") outside humidity"
					unit := "%"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				case 7: //Outside VPD
					name := "ACI (" + device + ") outside VPD"
					unit := "kPa"
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				default:
					// Unknown sensorType: create a generic sensor so users can see and rename it.
					// Determine a reasonable default unit:
					unit := ""
					// If device-level unit is present: 0 = °F, 1 = °C
					switch deviceData.DeviceInfo.Unit {
					case 0:
						// Fahrenheit default for temperature-like types
						if isTemperatureSensor(sensor.SensorType) {
							unit = "\u00b0F"
						}
					case 1:
						if isTemperatureSensor(sensor.SensorType) {
							unit = "\u00b0C"
						}
					}
					// Known percent/humidity-like types
					if isHumiditySensor(sensor.SensorType) {
						unit = "%"
					}
					name := fmt.Sprintf("ACI (%s) sensor %d.%d", device, sensor.AccessPort, sensor.SensorType)
					// Log for analytics so we can update mappings later
					logger.Log.WithFields(logrus.Fields{"device": device, "accessPort": sensor.AccessPort, "sensorType": sensor.SensorType, "sensorUnit": sensor.SensorUnit}).Info("Discovered unknown ACI sensor type - inserting generic sensor")
					checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
				}
			}

			for _, port := range deviceData.DeviceInfo.Ports {
				sensorType := fmt.Sprintf("ACIP.%d", port.Port)
				name := port.PortName
				unit := "%"
				checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
			}
		}
	}

	apiOK(c, "api_ac_infinity_sensors_scanned")
}

// prettyMarshalSorted marshals an unmarshaled JSON structure (interface{}) into
// a pretty-printed JSON byte slice where object keys are sorted alphabetically.
// It expects numbers to be json.Number when possible to preserve original formatting.
func prettyMarshalSorted(v interface{}, indent string) ([]byte, error) {
	var buf bytes.Buffer
	// recursive writer
	var writeValue func(interface{}, int) error
	writeIndent := func(level int) {
		for i := 0; i < level; i++ {
			buf.WriteString(indent)
		}
	}

	writeValue = func(val interface{}, level int) error {
		switch t := val.(type) {
		case map[string]interface{}:
			// sort keys
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			buf.WriteString("{")
			if len(keys) > 0 {
				buf.WriteString("\n")
			}
			for i, k := range keys {
				writeIndent(level + 1)
				// write key
				kb, _ := json.Marshal(k)
				buf.Write(kb)
				buf.WriteString(": ")
				if err := writeValue(t[k], level+1); err != nil {
					return err
				}
				if i != len(keys)-1 {
					buf.WriteString(",")
				}
				buf.WriteString("\n")
			}
			if len(keys) > 0 {
				writeIndent(level)
			}
			buf.WriteString("}")
		case []interface{}:
			buf.WriteString("[")
			if len(t) > 0 {
				buf.WriteString("\n")
			}
			for i, item := range t {
				writeIndent(level + 1)
				if err := writeValue(item, level+1); err != nil {
					return err
				}
				if i != len(t)-1 {
					buf.WriteString(",")
				}
				buf.WriteString("\n")
			}
			if len(t) > 0 {
				writeIndent(level)
			}
			buf.WriteString("]")
		case string:
			b, _ := json.Marshal(t)
			buf.Write(b)
		case json.Number:
			buf.WriteString(t.String())
		case float64:
			// fallback for plain float64 values
			buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
		case bool:
			if t {
				buf.WriteString("true")
			} else {
				buf.WriteString("false")
			}
		case nil:
			buf.WriteString("null")
		default:
			// final fallback: use json.Marshal
			b, err := json.Marshal(t)
			if err != nil {
				return err
			}
			buf.Write(b)
		}
		return nil
	}

	if err := writeValue(v, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// New handler: DumpACInfinityJSON
func DumpACInfinityJSON(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "DumpACInfinityJSON")
	fieldLogger.Info("Dumping raw AC Infinity JSON response")

	// Determine whether to redact sensitive fields (default: true)
	redact := true
	if v := c.Query("redact"); v != "" {
		if strings.ToLower(v) == "false" || v == "0" {
			redact = false
		}
	}

	aciToken := ConfigStoreFromContext(c).ACIToken()
	if aciToken == "" {
		fieldLogger.Error("ACIToken is not configured")
		apiInternalError(c, "api_aci_token_not_configured")
		return
	}

	url := "https://www.acinfinityserver.com/api/user/devInfoListAll?userId=" + aciToken
	reqBody := bytes.NewBuffer([]byte(""))

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		apiInternalError(c, "api_error_creating_request")
		return
	}

	req.Header.Add("token", aciToken)
	req.Header.Add("Host", "www.acinfinityserver.com")
	req.Header.Add("User-Agent", "okhttp/3.10.0")
	req.Header.Add("Content-Encoding", "gzip")

	resp, err := httpClient.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		apiError(c, http.StatusBadGateway, "api_error_sending_request_aci")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxSensorScanResponseBytes))
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		apiError(c, http.StatusBadGateway, "api_error_reading_response")
		return
	}

	// Try to unmarshal into a generic structure using UseNumber so we preserve numeric formatting.
	var data interface{}
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		// Not valid JSON: fall back to returning raw body
		fieldLogger.WithError(err).Warn("Response is not valid JSON; returning raw body")
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
		return
	}

	// Keys to redact
	redactKeys := map[string]struct{}{
		"devId":       {},
		"devCode":     {},
		"devMacAddr":  {},
		"appEmail":    {},
		"devTimeZone": {},
		"zoneId":      {},
		"wifiName":    {},
	}

	// recursive redaction
	var redactFunc func(interface{})
	redactFunc = func(v interface{}) {
		switch t := v.(type) {
		case map[string]interface{}:
			for k, val := range t {
				if _, ok := redactKeys[k]; ok {
					// Replace with a standard string marker
					t[k] = "[REDACTED]"
					continue
				}
				// Recurse into nested values
				switch val.(type) {
				case map[string]interface{}, []interface{}:
					redactFunc(val)
				}
			}
		case []interface{}:
			for _, item := range t {
				switch item.(type) {
				case map[string]interface{}, []interface{}:
					redactFunc(item)
				}
			}
		}
	}

	if redact {
		redactFunc(data)
		prettyBytes, err := prettyMarshalSorted(data, "  ")
		if err != nil {
			fieldLogger.WithError(err).Warn("Failed to marshal redacted JSON; returning raw body")
			c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
			return
		}

		c.Data(http.StatusOK, "application/json; charset=utf-8", prettyBytes)
		return
	}

	// Non-redacted: pretty print with sorted keys for consistent ordering
	prettyBytes, err := prettyMarshalSorted(data, "  ")
	if err != nil {
		fieldLogger.WithError(err).Warn("Failed to marshal pretty JSON; returning raw body")
		c.Data(http.StatusOK, "application/json; charset=utf-8", respBody)
		return
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", prettyBytes)
}

func ScanEcoWittSensors(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "ScanEcoWittSensors")
	var input struct {
		ZoneID        *int   `json:"zone_id"`  // Pointer to allow null values
		NewZone       string `json:"new_zone"` // Optional new zone name
		ServerAddress string `json:"server_address"`
	}
	// Bind the JSON payload to the input struct
	if err := c.ShouldBindJSON(&input); err != nil {
		fieldLogger.WithError(err).Error("Invalid input")
		apiBadRequest(c, "api_invalid_input")
		return
	}
	// Validate string lengths
	if err := utils.ValidateStringLength("new_zone", input.NewZone, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	if err := utils.ValidateStringLength("server_address", input.ServerAddress, utils.MaxNameLength); err != nil {
		apiBadRequest(c, err.Error())
		return
	}
	sensors := ServicesFromContext(c).Sensors
	store := ConfigStoreFromContext(c)

	if input.ZoneID == nil && input.NewZone != "" {
		// Insert a new zone into the database
		zone, err := ServicesFromContext(c).Zones.Create(c.Request.Context(), service.ZoneInput{Name: input.NewZone})
		zoneID := zone.ID
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			apiInternalError(c, "api_zone_creation_failed")
			return
		}
		input.ZoneID = &zoneID // Set the created zone ID
	}

	// Validate the input server address before constructing the request URL
	if !ValidateServerAddress(input.ServerAddress) {
		fieldLogger.Error("Invalid server address")
		apiBadRequest(c, "api_invalid_input")
		return
	}

	url := "http://" + input.ServerAddress + "/get_livedata_info"

	reqBody := bytes.NewBuffer([]byte(""))
	req, err := http.NewRequest("GET", url, reqBody)
	if err != nil {
		fieldLogger.WithError(err).Error("Error creating request")
		apiInternalError(c, "api_error_creating_request")
		return
	}

	resp, err := httpClientNoRedirect.Do(req)
	if err != nil {
		fieldLogger.WithError(err).Error("Error sending request")
		apiError(c, http.StatusBadGateway, "api_error_sending_request_ecowitt")
		return
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxSensorScanResponseBytes))
	if err != nil {
		fieldLogger.WithError(err).Error("Error reading response body")
		apiError(c, http.StatusBadGateway, "api_error_reading_response")
		return
	}

	// Parse the JSON into the struct
	var apiResponse types.ECWAPIResponse
	err = json.Unmarshal(respBody, &apiResponse)
	if err != nil {
		fieldLogger.WithError(err).Error("Error unmarshalling JSON response")
		apiError(c, http.StatusBadGateway, "api_error_invalid_response")
		return
	}

	//Add EcoWitt sensors to db
	sensorType := "WH25.InTemp"
	device := input.ServerAddress
	source := "ecowitt"
	name := "EC (" + input.ServerAddress + ") InTemp"
	unit := "°F"
	checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)

	sensorType = "WH25.InHumi"
	name = "EC (" + input.ServerAddress + ") InHumi"
	unit = "%"
	checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)

	for _, ch := range apiResponse.CHSoil {
		sensorType := "Soil." + ch.Channel
		device := input.ServerAddress
		source := "ecowitt"
		name := ch.Name
		unit := "%"
		checkInsertSensor(sensors, source, device, sensorType, name, input.ZoneID, unit)
	}

	for _, ch := range apiResponse.CHAisle {
		sensorType := "Aisle." + ch.Channel + ".Temp"
		unit := "°F"
		if ch.Unit == "C" {
			unit = "°C"
		}
		checkInsertSensor(sensors, source, device, sensorType,
			"EC ("+device+") Ch"+ch.Channel+" Temp", input.ZoneID, unit)
		sensorType = "Aisle." + ch.Channel + ".Humi"
		checkInsertSensor(sensors, source, device, sensorType,
			"EC ("+device+") Ch"+ch.Channel+" Humi", input.ZoneID, "%")
	}

	for _, ch := range apiResponse.CHEc {
		tempUnit := "°F"
		if ch.Unit == "C" {
			tempUnit = "°C"
		}
		checkInsertSensor(sensors, source, device, "SoilEC."+ch.Channel+".Moisture",
			"EC ("+device+") Ch"+ch.Channel+" Soil Moisture", input.ZoneID, "%")
		checkInsertSensor(sensors, source, device, "SoilEC."+ch.Channel+".Temp",
			"EC ("+device+") Ch"+ch.Channel+" Soil Temp", input.ZoneID, tempUnit)
		checkInsertSensor(sensors, source, device, "SoilEC."+ch.Channel+".EC",
			"EC ("+device+") Ch"+ch.Channel+" Soil EC", input.ZoneID, "mS/cm")
	}

	for _, item := range apiResponse.CommonList {
		meta, ok := types.ECWCommonSensors[types.NormalizeECWID(item.ID)]
		if !ok {
			continue
		}
		if strings.HasPrefix(item.Val, "--") {
			continue
		}
		unit := meta.Unit
		if item.Unit == "F" {
			unit = "°F"
		} else if item.Unit == "C" {
			unit = "°C"
		}
		checkInsertSensor(sensors, source, device, meta.TypeKey,
			fmt.Sprintf(meta.Name, device), input.ZoneID, unit)
	}

	//Update ECOWitt sensors
	//Set ECDevices
	strECDevices, err := LoadEcDevices(DBFromContext(c))
	if err == nil {
		store.SetECDevices(strECDevices)
	} else {
		fieldLogger.WithError(err).Error("Error loading EC devices")
	}

	apiOK(c, "api_ecowitt_sensors_scanned")
}

// findOrCreateSensor returns the id of the sensor (source, device, type),
// registering it if needed. See service.RegisterSensor.
// checkInsertSensor registers a sensor an integration scan found. Scans
// find many sensors per call and don't need the ids.
func checkInsertSensor(sensors *service.SensorService, source string, device string, sensorType string, name string, zoneId *int, unit string) {
	if _, err := sensors.Register(context.Background(), service.SensorRegistration{
		Name: name, Source: source, Device: device, Type: sensorType, ZoneID: zoneId, Unit: unit,
	}); err != nil {
		logger.Log.WithField("func", "checkInsertSensor").WithError(err).Error("Error registering sensor")
	}
}

// GetGroupedSensorsWithLatestReading returns the dashboard's sensors by
// zone and device, cached for a fraction of the polling interval.
func GetGroupedSensorsWithLatestReading(ctx context.Context, svc *service.Services, cache *SensorCacheService) map[string]map[string][]map[string]interface{} {
	if cached, ok := cache.GroupedGet(); ok {
		return cached
	}
	sensors, err := svc.Sensors.Dashboard(ctx)
	if err != nil {
		logger.Log.WithError(err).Error("Failed to load dashboard sensors")
		return nil
	}
	grouped := make(map[string]map[string][]map[string]interface{})
	for _, d := range sensors {
		if grouped[d.Zone] == nil {
			grouped[d.Zone] = make(map[string][]map[string]interface{})
		}
		entry := map[string]interface{}{
			"type": d.Type, "name": d.Name, "value": d.Value, "id": d.ID, "unit": d.Unit, "trend": d.Trend,
		}
		if d.PlantName != "" {
			entry["plant_name"] = d.PlantName
		}
		grouped[d.Zone][d.Device] = append(grouped[d.Zone][d.Device], entry)
	}
	cache.GroupedPut(grouped)
	return grouped
}

// EditSensor saves the sensor form: name, zone (0 for none), unit and
// visibility. Source, device and type identify the sensor to its
// integration and never change.
func EditSensor(c *gin.Context) {
	var input struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
		ZoneID     int    `json:"zone_id"`
		Unit       string `json:"unit"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	var zoneID *int
	if input.ZoneID > 0 {
		zoneID = &input.ZoneID
	}
	if _, err := ServicesFromContext(c).Sensors.Update(c.Request.Context(), input.ID, service.SensorUpdate{
		Name:       service.Put(&input.Name),
		ZoneID:     service.Put(zoneID),
		Unit:       service.Put(&input.Unit),
		Visibility: service.Put(&input.Visibility),
	}); err != nil {
		if e, ok := service.AsError(err); ok && e.Fields["visibility"] != "" {
			apiBadRequest(c, "api_invalid_visibility")
			return
		}
		apiServiceError(c, err, "api_failed_to_update_sensor")
		return
	}
	apiOK(c, "api_sensor_updated")
}

// DeleteSensor deletes a sensor and its readings.
func DeleteSensor(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		apiBadRequest(c, "api_invalid_input")
		return
	}
	if err := ServicesFromContext(c).Sensors.Delete(c.Request.Context(), id); err != nil {
		apiServiceError(c, err, "api_failed_to_delete_sensor")
		return
	}
	apiOK(c, "api_sensor_deleted")
}

// ValidateServerAddress ensures the input is a valid private IP or hostname
func ValidateServerAddress(address string) bool {
	// Check if it's a valid IP
	ip := net.ParseIP(address)
	if ip != nil {
		// Ensure the IP is private
		if ip.IsLoopback() || ip.IsPrivate() {
			return true
		}
		return false
	}

	// Check if it's a valid hostname (local hostnames only)
	validHostname := regexp.MustCompile(`^([a-zA-Z0-9_-]+\.)*[a-zA-Z0-9_-]+$`).MatchString
	return validHostname(address)
}

// SensorDataPayload is one reading. Value is a pointer so binding:"required"
// means "present": on a plain float64 it treated an exact 0 as missing and
// rejected legitimate zero readings.
type SensorDataPayload struct {
	Source  string   `json:"source" binding:"required"`
	Device  string   `json:"device" binding:"required"`
	Type    string   `json:"type" binding:"required"`
	Value   *float64 `json:"value" binding:"required"`
	Name    string   `json:"name"`
	Unit    string   `json:"unit"`
	ZoneID  *int     `json:"zone_id"`
	NewZone string   `json:"new_zone"`
}

const (
	// maxIngestBatch caps the readings one request may carry. It equals the
	// per-minute ingest allowance so that a full batch can actually succeed.
	maxIngestBatch = ratelimit.IngestPerMinute
	// maxIngestBodyBytes bounds the body before parsing; a full batch with
	// maximum-length names still fits.
	maxIngestBodyBytes = 256 << 10
)

// parseIngestBody accepts one reading as a JSON object or several as a JSON
// array, binding and validating each with the same rules. batch reports
// which form was sent, since the response shape follows it.
func parseIngestBody(body []byte) (payloads []SensorDataPayload, batch bool, err error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '[' {
		var p SensorDataPayload
		if err := binding.JSON.BindBody(body, &p); err != nil {
			return nil, false, err
		}
		return []SensorDataPayload{p}, false, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, true, err
	}
	payloads = make([]SensorDataPayload, len(items))
	for i, item := range items {
		if err := binding.JSON.BindBody(item, &payloads[i]); err != nil {
			return nil, true, fmt.Errorf("reading %d: %w", i, err)
		}
	}
	return payloads, true, nil
}

// validateIngestPayload applies the length and value checks to one reading
// and fills in its defaults: the canonical unit and a generated name.
func validateIngestPayload(p *SensorDataPayload) error {
	for _, f := range []struct {
		field, value string
		max          int
	}{
		{"source", p.Source, utils.MaxSourceLength},
		{"device", p.Device, utils.MaxDeviceLength},
		{"type", p.Type, utils.MaxTypeLength},
		{"name", p.Name, utils.MaxNameLength},
		{"unit", p.Unit, utils.MaxUnitLength},
		{"new_zone", p.NewZone, utils.MaxNameLength},
	} {
		if err := utils.ValidateStringLength(f.field, f.value, f.max); err != nil {
			return err
		}
	}
	// Validate sensor value is finite (reject NaN and Infinity)
	if err := utils.ValidateFiniteFloat64("value", *p.Value); err != nil {
		return err
	}
	p.Unit = utils.NormalizeUnit(p.Unit)
	if p.Name == "" {
		p.Name = fmt.Sprintf("%s (%s) %s", p.Source, p.Device, p.Type)
	}
	return nil
}

// resolveIngestSensor finds or creates the reading's zone (when new_zone is
// given) and its sensor. On failure it returns a translation key.
func resolveIngestSensor(ctx context.Context, svc *service.Services, p SensorDataPayload) (int, string) {
	fieldLogger := logger.Log.WithField("func", "resolveIngestSensor")

	zoneID := p.ZoneID
	if zoneID == nil && p.NewZone != "" {
		// Try to find an existing zone with the given new_zone name first
		zone, err := svc.Zones.FindOrCreate(ctx, p.NewZone)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to create new zone")
			return 0, "api_zone_creation_failed"
		}
		zoneID = &zone.ID
	}

	// Register serializes the SELECT-then-INSERT so concurrent ingests of
	// an unknown sensor don't each create it.
	sensorID, err := svc.Sensors.Register(ctx, service.SensorRegistration{
		Name: p.Name, Source: p.Source, Device: p.Device, Type: p.Type, ZoneID: zoneID, Unit: p.Unit,
	})
	if err != nil {
		fieldLogger.WithError(err).Error("Error upserting sensor")
		return 0, "api_failed_to_create_sensor"
	}
	return sensorID, ""
}

func IngestSensorData(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "IngestSensorData")

	store := ConfigStoreFromContext(c)

	// Check whether API ingest is enabled in config
	if store.APIIngestEnabled() == 0 {
		fieldLogger.Warn("API ingest is disabled via settings")
		apiForbidden(c, "api_api_ingest_disabled")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxIngestBodyBytes))
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid payload")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	payloads, batch, err := parseIngestBody(body)
	if err != nil {
		fieldLogger.WithError(err).Error("Invalid payload")
		apiBadRequest(c, "api_invalid_payload")
		return
	}
	if len(payloads) == 0 || len(payloads) > maxIngestBatch {
		apiBadRequest(c, "api_ingest_batch_size")
		return
	}

	// The middleware counted the request as one reading; count the rest.
	if extra := len(payloads) - 1; extra > 0 {
		key, logKey := ingestRateKey(c)
		if ok, retryAfter := RateLimiterServiceFromContext(c).Ingest().AllowN(key, extra); !ok {
			abortRateLimited(c, logKey, retryAfter)
			return
		}
	}

	for i := range payloads {
		if err := validateIngestPayload(&payloads[i]); err != nil {
			msg := err.Error()
			if batch {
				msg = fmt.Sprintf("reading %d: %s", i, msg)
			}
			apiBadRequest(c, msg)
			return
		}
	}

	svc := ServicesFromContext(c)
	sensorIDs := make([]int, len(payloads))
	readings := make([]service.NewReading, len(payloads))
	for i, p := range payloads {
		id, errKey := resolveIngestSensor(c.Request.Context(), svc, p)
		if errKey != "" {
			apiInternalError(c, errKey)
			return
		}
		sensorIDs[i] = id
		readings[i] = service.NewReading{SensorID: id, Value: *p.Value}
	}

	// A batch lands whole or not at all.
	if err := svc.Sensors.Record(c.Request.Context(), readings); err != nil {
		fieldLogger.WithError(err).Error("Error inserting sensor data")
		apiInternalError(c, "api_failed_to_save_sensor_data")
		return
	}

	if batch {
		c.JSON(http.StatusOK, gin.H{
			"message":    T(c, "api_sensor_data_ingested"),
			"sensor_ids": sensorIDs,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":   T(c, "api_sensor_data_ingested"),
		"sensor_id": sensorIDs[0],
	})
}
