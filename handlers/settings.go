package handlers

// Settings: reading, saving and loading them into the config store.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"isley/config"
	"isley/logger"
	"isley/model/types"
	"isley/service"
	"isley/utils"
	"strconv"

	"github.com/gin-gonic/gin"
)

func SaveSettings(c *gin.Context) {
	fieldLogger := logger.Log.WithField("func", "SaveSettings")
	var settings types.Settings
	if err := c.ShouldBindJSON(&settings); err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiBadRequest(c, "api_invalid_settings_payload")
		return
	}

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	flag := func(on bool) string {
		if on {
			return "1"
		}
		return "0"
	}
	writes := [][2]string{
		{"aci.enabled", flag(settings.ACI.Enabled)},
		{"ec.enabled", flag(settings.EC.Enabled)},
		{"cannadb.enabled", flag(settings.Cannadb.Enabled)},
		{"guest_mode", flag(settings.GuestMode)},
		{"stream_grab_enabled", flag(settings.StreamGrabEnabled)},
		{"api_ingest_enabled", flag(!settings.DisableAPIIngest)},
		{"mcp_enabled", flag(settings.MCPEnabled)},
		{"polling_interval", settings.PollingInterval},
		{"stream_grab_interval", settings.StreamGrabInterval},
		{"sensor_retention_days", settings.SensorRetentionDays},
		{"log_level", settings.LogLevel},
		// Harvest weights are stored in grams; this only sets the unit the
		// web UI displays and offers first in the plant forms.
		{"weight_unit", utils.NormalizeWeightUnit(settings.WeightUnit)},
		// Empty means the default api.cannadb.net endpoint.
		{"cannadb.base_url", settings.Cannadb.BaseURL},
		// Empty means the system zone.
		{"timezone", settings.Timezone},
	}
	if mb, err := strconv.Atoi(settings.MaxBackupSizeMB); err == nil && mb >= MinBackupSizeMB {
		writes = append(writes, [2]string{"max_backup_size_mb", settings.MaxBackupSizeMB})
	}
	// API keys are managed through the dedicated /settings/api-keys
	// endpoints, not this form.

	// All or nothing, then one reload.
	tx, err := db.Begin()
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	for _, w := range writes {
		if err := writeSetting(tx, w[0], w[1]); err != nil {
			fieldLogger.WithError(err).Error("Failed to save settings")
			apiInternalError(c, "api_failed_to_save_settings")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fieldLogger.WithError(err).Error("Failed to save settings")
		apiInternalError(c, "api_failed_to_save_settings")
		return
	}
	// LoadSettings skips an empty zone (it keeps the default); a cleared
	// zone must still take effect now.
	store.SetTimezone(settings.Timezone)
	LoadSettings(db, store)

	apiOK(c, "api_settings_saved")
}

// UpdateSetting saves one setting and reloads the Store from the settings
// table. A nil Store skips the reload. To save several, write them with
// writeSetting and reload once.
func UpdateSetting(db *sql.DB, store *config.Store, name string, value string) error {
	if err := writeSetting(db, name, value); err != nil {
		return err
	}
	if store != nil {
		LoadSettings(db, store)
	}
	return nil
}

// execQuerier is a *sql.DB or *sql.Tx.
type execQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// writeSetting saves one setting, adding its row if there isn't one.
func writeSetting(q execQuerier, name, value string) error {
	var id int
	err := q.QueryRow("SELECT id FROM settings WHERE name = $1 ORDER BY id LIMIT 1", name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = q.Exec("INSERT INTO settings (name, value) VALUES ($1, $2)", name, value)
	case err == nil:
		_, err = q.Exec("UPDATE settings SET value = $1 WHERE id = $2", value, id)
	}
	if err != nil {
		return fmt.Errorf("save setting %s: %w", name, err)
	}
	return nil
}

func GetSettings(db *sql.DB) types.SettingsData {
	fieldLogger := logger.Log.WithField("func", "GetSettings")

	settingsData := types.SettingsData{WeightUnit: utils.WeightUnitGrams}

	rows, err := db.Query("SELECT * FROM settings")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read settings")
		return types.SettingsData{}
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name, value, createDt, updateDt string
		err = rows.Scan(&id, &name, &value, &createDt, &updateDt)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read settings")
			continue
		}

		switch name {
		case "aci.enabled":
			settingsData.ACI.Enabled = value == "1"
		case "ec.enabled":
			settingsData.EC.Enabled = value == "1"
		case "cannadb.enabled":
			settingsData.Cannadb.Enabled = value == "1"
		case "cannadb.base_url":
			settingsData.Cannadb.BaseURL = value
		case "aci.token":
			if value != "" {
				settingsData.ACI.TokenSet = true
			}
		case "polling_interval":
			settingsData.PollingInterval, _ = strconv.Atoi(value)
		case "guest_mode":
			settingsData.GuestMode = value == "1"
		case "stream_grab_enabled":
			settingsData.StreamGrabEnabled = value == "1"
		case "stream_grab_interval":
			iValue, err := strconv.Atoi(value)
			if err != nil {
				iValue = DefaultStreamGrabIntervalMs
			}
			settingsData.StreamGrabInterval = iValue
		case "api_ingest_enabled":
			settingsData.APIIngestEnabled = value == "1"
		case "mcp_enabled":
			settingsData.MCPEnabled = value == "1"
		case "sensor_retention_days":
			settingsData.SensorRetentionDays, _ = strconv.Atoi(value)
		case "log_level":
			settingsData.LogLevel = value
		case "max_backup_size_mb":
			settingsData.MaxBackupSizeMB, _ = strconv.Atoi(value)
		case "timezone":
			settingsData.Timezone = value
		case "weight_unit":
			settingsData.WeightUnit = utils.NormalizeWeightUnit(value)
		default:
			fieldLogger.WithField("name", name).Debug("Unrecognised setting skipped")
		}
	}

	return settingsData
}

func GetSetting(db *sql.DB, name string) (string, error) {
	fieldLogger := logger.Log.WithField("func", "GetSetting")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE name = $1", name).Scan(&value)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			fieldLogger.WithField("name", name).Debug("Setting not found")
			return "", nil
		} else {
			fieldLogger.WithError(err).Error("Failed to read setting")
			return "", err
		}
	}

	return value, nil
}

func ExistsSetting(db *sql.DB, s string) (bool, error) {
	fieldLogger := logger.Log.WithField("func", "ExistsSetting")

	var value string
	err := db.QueryRow("SELECT value FROM settings WHERE name = $1", s).Scan(&value)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return false, nil
		} else {
			fieldLogger.WithError(err).Error("Failed to read setting")
			return false, err
		}
	}

	return true, nil

}

// LoadSettings refreshes the supplied *config.Store from the settings
// table. It takes the *sql.DB and *config.Store explicitly so tests
// (and the in-process harness) construct one Store per engine and the
// production path threads the engine's Store through the same code.
// A setting that is missing or unreadable leaves the Store's value as it is.
func LoadSettings(db *sql.DB, store *config.Store) {
	fieldLogger := logger.Log.WithField("func", "LoadSettings")
	if db == nil || store == nil {
		fieldLogger.Error("LoadSettings needs a DB and a Store")
		return
	}

	ints := []struct {
		name string
		set  func(int)
	}{
		{"polling_interval", store.SetPollingInterval},
		{"aci.enabled", store.SetACIEnabled},
		{"ec.enabled", store.SetECEnabled},
		{"guest_mode", store.SetGuestMode},
		{"stream_grab_enabled", store.SetStreamGrabEnabled},
		{"stream_grab_interval", store.SetStreamGrabInterval},
		{"api_ingest_enabled", store.SetAPIIngestEnabled},
		{"mcp_enabled", store.SetMCPEnabled},
		{"sensor_retention_days", store.SetSensorRetention},
		{"cannadb.enabled", store.SetCannadbEnabled},
	}
	for _, s := range ints {
		if v, err := GetSetting(db, s.name); err == nil {
			if n, err := strconv.Atoi(v); err == nil {
				s.set(n)
			}
		}
	}

	if v, err := GetSetting(db, "aci.token"); err == nil {
		store.SetACIToken(v)
	}
	if v, err := GetSetting(db, "api_key"); err == nil {
		store.SetAPIKey(v)
	}
	if v, err := GetSetting(db, "cannadb.base_url"); err == nil {
		store.SetCannadbBaseURL(v)
	}
	if v, err := GetSetting(db, "timezone"); err == nil && v != "" {
		store.SetTimezone(v)
	}
	if v, err := GetSetting(db, "weight_unit"); err == nil {
		store.SetWeightUnit(utils.NormalizeWeightUnit(v))
	}
	if v, err := GetSetting(db, "log_level"); err == nil && v != "" {
		store.SetLogLevel(v)
		logger.SetLevel(v)
	}
	if v, err := GetSetting(db, "max_backup_size_mb"); err == nil {
		if mb, err := strconv.Atoi(v); err == nil && mb >= MinBackupSizeMB {
			store.SetMaxBackupSize(int64(mb) * 1024 * 1024)
		}
	}
	if devices, err := LoadEcDevices(db); err == nil {
		store.SetECDevices(devices)
	}

	if err := service.RefreshStore(context.Background(), db, store); err != nil {
		fieldLogger.WithError(err).Error("Failed to load statuses, metrics, activity types, zones, breeders, strains, streams and smoke criteria")
	}
}

func LoadEcDevices(db *sql.DB) ([]string, error) {
	fieldLogger := logger.Log.WithField("func", "LoadEcDevices")
	var ecDevices []string

	//Iterate over sensors table, looking for distinct device with type ecowitt
	rows, err := db.Query("SELECT DISTINCT device FROM sensors WHERE source = 'ecowitt'")
	if err != nil {
		fieldLogger.WithError(err).Error("Failed to read sensors")
		return ecDevices, err
	}
	//build a list of devices to scan

	for rows.Next() {
		var device string
		err = rows.Scan(&device)
		if err != nil {
			fieldLogger.WithError(err).Error("Failed to read sensors")
			return ecDevices, err
		}
		ecDevices = append(ecDevices, device)
	}

	return ecDevices, nil
}
