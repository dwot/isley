package utils

import (
	"embed"
	"isley/logger"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var embeddedLocales embed.FS

// I18nManager handles translations
type I18nManager struct {
	bundle *i18n.Bundle
	mu     sync.RWMutex
}

// Global variables
var (
	TranslationService *I18nManager
	AvailableLanguages []string // List of languages
)

// Init initializes the translation service
func Init(defaultLang string) {
	TranslationService = &I18nManager{
		bundle: i18n.NewBundle(language.English),
	}

	// Register YAML format for translations
	TranslationService.bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Track loaded languages
	AvailableLanguages = []string{}

	// Load translations from embedded filesystem
	files, err := embeddedLocales.ReadDir("locales")
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to read embedded locales directory")
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".yaml" {
			path := "locales/" + file.Name()
			lang := strings.TrimSuffix(file.Name(), ".yaml")
			AvailableLanguages = append(AvailableLanguages, lang)
			data, err := embeddedLocales.ReadFile(path)
			if err != nil {
				logger.Log.WithError(err).Errorf("Failed to read translation file: %s", path)
				continue
			}

			// Load the translation into the bundle
			if _, err := TranslationService.bundle.ParseMessageFileBytes(data, file.Name()); err != nil {
				logger.Log.WithError(err).Errorf("Failed to load translation file: %s", path)
			} else {
				logger.Log.Infof("Loaded translation file: %s", path)
			}
		}
	}

	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to load locales directory")
	}
}

func (i *I18nManager) GetTranslations(lang string) map[string]string {
	localizer := i18n.NewLocalizer(i.bundle, lang, "en")

	// Predefine the keys for translation
	keys := []string{
		"sensors_overview",
		"website",
		"change_password",
		"plants_overview",
		"title_plant",
		"title_plants",
		"title_strain",
		"title_strains",
		"title_status",
		"title_start",
		"title_sensors",
		"title_settings",
		"title_logout",
		"title_login",
		"title_last_water",
		"title_last_feed",
		"title_days_flower",
		"title_week",
		"title_day",
		"na",
		"title_group_other",
		"title_group_acip",
		"title_group_soil",
		"loading",
		"multiple_action_desc",
		"title_close",
		"activity_name",
		"activity_note",
		"title_date",
		"select_plants",
		"title_zone",
		"multi_select_plant_note",
		"record_activity",
		"activity_success",
		"activity_error",
		"new_password",
		"confirm_password",
		"title_time_range",
		"time_range_60",
		"time_range_360",
		"time_range_1440",
		"time_range_2880",
		"time_range_10080",
		"start_date",
		"end_date",
		"apply",
		"reset_zoom",
		"username",
		"password",
		"in_stock",
		"out_stock",
		"search_strains",
		"add_new_strain",
		"strain_name",
		"strain_name_placeholder",
		"url",
		"strain_url_placeholder",
		"breeder",
		"add_new_breeder",
		"new_breeder_placeholder",
		"i_s_ratio",
		"autoflower",
		"yes",
		"no",
		"cycle_time",
		"cycle_time_desc",
		"seed_count",
		"seed_count_placeholder",
		"description_txt",
		"strain_description_placeholder",
		"add_strain",
		"edit_strain",
		"new_breeder_name",
		"save_changes",
		"delete_strain",
		"strain_add_fail",
		"strain_add_error",
		"strain_update_fail",
		"strain_update_error",
		"strain_delete_fail",
		"strain_delete_error",
		"title_is",
		"title_auto",
		"title_tables",
		"set_logo_image",
		"aci_enabled_info",
		"token_set",
		"token_not_set",
		"retrieve_token",
		"retrieve_token_desc",
		"ecowitt_enabled_info",
		"ecowitt_enabled_desc",
		"guest_mode_info",
		"guest_mode_desc",
		"aci_dump_json",
		"aci_dump_json_desc",
		"aci_redact_label",
		"aci_dump_json_caution",
		"polling_interval",
		"time_seconds",
		"polling_interval_desc",
		"save_settings",
		"add_zone",
		"title_zones",
		"add_activity",
		"title_activity",
		"title_activities",
		"title_metric",
		"title_metrics",
		"add_metric",
		"title_unit",
		"breeders",
		"add_breeder",
		"add_logo_info",
		"add_logo_note",
		"add_logo",
		"upload_image_info",
		"save_logo",
		"title_email",
		"title_aci_email_placeholder",
		"title_aci_password_placeholder",
		"zone_name",
		"metric_name",
		"metric_unit",
		"edit_zone",
		"delete_zone",
		"edit_activity",
		"delete_activity",
		"edit_metric",
		"delete_metric",
		"delete_breeder",
		"failed_to_add_zone",
		"failed_to_update_zone",
		"failed_to_delete_zone",
		"delete_zone_confirm",
		"failed_to_fetch_token",
		"error_fetching_token",
		"generic_error",
		"failed_to_save_settings",
		"failed_to_add_activity",
		"failed_to_update_activity",
		"failed_to_delete_activity",
		"delete_activity_confirm",
		"failed_to_add_metric",
		"failed_to_update_metric",
		"failed_to_delete_metric",
		"delete_metric_confirm",
		"failed_to_add_breeder",
		"failed_to_update_breeder",
		"failed_to_delete_breeder",
		"delete_breeder_confirm",
		"logo_uploaded_successfully",
		"error_uploading_logo",
		"select_logo_image",
		"aci_scan_add",
		"ecowitt_scan_add",
		"ecowitt_scan",
		"server_address",
		"server_address_placeholder",
		"add_new_zone",
		"new_zone_name",
		"enter_zone_name",
		"scan_sensors",
		"title_id",
		"title_name",
		"title_source",
		"title_device",
		"title_type",
		"title_show_hide",
		"title_created",
		"title_updated",
		"title_show",
		"title_hide",
		"select_zone",
		"no_zones_available",
		"title_proceed",
		"edit_sensor",
		"edit_device_info",
		"title_visibility",
		"delete_sensor",
		"failed_sensor_scan",
		"no_scan_endpoint",
		"failed_save_changes",
		"failed_update_sensor",
		"confirm_delete_sensor",
		"failed_delete_sensor",
		"plant_details",
		"clone_of",
		"title_unknown",
		"est_harvest_date",
		"harvest_date",
		"harvest_weight",
		"title_grams",
		"title_height",
		"last_watered_or_fed",
		"title_watered",
		"title_fed",
		"add_measurement",
		"upload_images",
		"status_history",
		"growth_history",
		"current_growth_stage",
		"days_in_stage",
		"days_in_each_growth_stage",
		"growth_stage_timeline",
		"total_days",
		"growth_history_empty",
		"title_days",
		"title_measurements",
		"title_measurement",
		"title_value",
		"title_note",
		"image_gallery",
		"image_details",
		"select_font",
		"font_preview_text",
		"select_logo",
		"title_none",
		"top_text",
		"custom_text",
		"title_placement",
		"top_left",
		"top_right",
		"bottom_left",
		"bottom_right",
		"bottom_text",
		"text_color",
		"decorate_image",
		"title_previous",
		"title_next",
		"link_sensors",
		"title_select",
		"update_plant",
		"effective_date",
		"plant_description",
		"enter_strain_name",
		"enter_breeder_name",
		"is_clone_info",
		"enter_new_zone",
		"harvest_weight_g",
		"change_status",
		"measurement_name",
		"measurement_value",
		"edit_status",
		"delete_status",
		"edit_measurement",
		"delete_measurement",
		"failed_to_add_measurement",
		"failed_to_change_status",
		"failed_to_link_sensors",
		"failed_to_update_measurement",
		"failed_to_delete_measurement",
		"confirm_delete_measurement",
		"failed_to_update_status",
		"confirm_delete_status",
		"failed_to_delete_status",
		"confirm_delete_activity",
		"only_image_files",
		"living_plants",
		"harvested_plants",
		"dead_plants",
		"search_plants",
		"current_week",
		"current_day",
		"add_new_plant",
		"plant_name",
		"plant_name_placeholder",
		"is_clone",
		"parent_plant",
		"decrement_seed_count",
		"add_plant",
		"dead_date",
		"title_streams",
		"add_stream",
		"stream_name",
		"stream_name_placeholder",
		"stream_url",
		"stream_url_placeholder",
		"stream_zone",
		"capture_interval",
		"capture_interval_desc",
		"failed_to_add_stream",
		"failed_to_update_stream",
		"failed_to_delete_stream",
		"delete_stream_confirm",
		"stream_grab_enabled",
		"stream_grab_enabled_desc",
		"stream_grab_interval",
		"stream_grab_interval_desc",
		"time_minutes",
		"short_description_txt",
		"short_description_placeholder",
		"description_txt_desc",
		"generate_new_key",
		"api_key_desc",
		"generate_new_key_confirm",
		"api_key_placeholder",
		"toggle_visibility",
		"copy",
		"remember_me",
		"disable_api_ingest",
		"disable_api_ingest_desc",
	}

	out := make(map[string]string)
	for _, key := range keys {
		translation, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
		if err != nil || translation == "" {
			// fallback explicitly if needed
			translation, _ = i18n.NewLocalizer(i.bundle, "en").Localize(&i18n.LocalizeConfig{MessageID: key})
		}

		out[key] = translation
	}

	return out
}

func GetLanguage(c *gin.Context) string {
	lang := "en"
	queryLang := c.Query("lang")
	sessionLang := sessions.Default(c).Get("lang")
	if sessionLang != nil {
		lang = sessionLang.(string)
	}
	if queryLang != "" {
		lang = queryLang
		sessions.Default(c).Set("lang", lang)
		sessions.Default(c).Save()
	}
	return lang
}
