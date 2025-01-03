package utils

import (
	"embed"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
	"isley/logger"
	"path/filepath"
	"strings"
	"sync"
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
	localizer := i18n.NewLocalizer(i.bundle, lang)

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
	}

	translations := make(map[string]string)

	// Fetch translations for predefined keys
	for _, key := range keys {
		translated, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
		if err != nil {
			translations[key] = key // Fallback to the key itself
		} else {
			translations[key] = translated
		}

	}
	return translations
}
