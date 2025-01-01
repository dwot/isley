package utils

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
)

// I18nManager handles translations
type I18nManager struct {
	bundle *i18n.Bundle
	mu     sync.RWMutex
}

// Global translation service
var TranslationService *I18nManager

// Init initializes the translation service
func Init(defaultLang string) {
	TranslationService = &I18nManager{
		bundle: i18n.NewBundle(language.English),
	}

	// Register YAML format for translations
	TranslationService.bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)

	// Load all translation files
	err := filepath.WalkDir("./locales", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".yaml" {
			_, loadErr := TranslationService.bundle.LoadMessageFile(path)
			if loadErr != nil {
				logrus.WithError(loadErr).Errorf("Failed to load translation file: %s", path)
			} else {
				logrus.Infof("Loaded translation file: %s", path)
			}
		}
		return nil
	})

	if err != nil {
		logrus.WithError(err).Fatal("Failed to load locales directory")
	}
}

func (i *I18nManager) GetTranslations(lang string) map[string]string {
	localizer := i18n.NewLocalizer(i.bundle, lang)

	// Predefine the keys for translation
	keys := []string{
		"website",                        //"Website"
		"change_password",                //"Change Password"
		"sensors_overview",               //"Sensors Overview"
		"plants_overview",                //"Plants Overview"
		"title_plant",                    //"Plant"
		"title_plants",                   //"Plants"
		"title_strain",                   //"Strain"
		"title_strains",                  //"Strains"
		"title_status",                   //"Status"
		"title_start",                    //"Start"
		"title_sensors",                  //"Sensors"
		"title_settings",                 //"Settings"
		"title_logout",                   //"Logout"
		"title_login",                    //"Login"
		"title_last_water",               //"Last 💧"
		"title_last_feed",                //"Last 🍬"
		"title_days_flower",              //"Days 🪻"
		"title_week",                     //"Week"
		"title_day",                      //"Day"
		"na",                             //"N/A"
		"title_group_other",              //"Environment Sensors"
		"title_group_acip",               //"AC Infinity Devices"
		"title_group_soil",               //"EcoWitt Soil Sensors"
		"loading",                        //"Loading"
		"multiple_action_desc",           //"Record Activity for Multiple Plants"
		"title_close",                    //"Close"
		"activity_name",                  //"Activity Name"
		"activity_note",                  //"Activity Note"
		"title_date",                     //"Date"
		"select_plants",                  //"Select Plants"
		"title_zone",                     //"Zone"
		"multi_select_plant_note",        //"Hold Ctrl (Cmd on Mac) to select multiple plants."
		"record_activity",                //"Record Activity"
		"activity_success",               //"Activity recorded successfully!"
		"activity_error",                 //"Failed to record activity. Please try again."
		"new_password",                   //"New Password"
		"confirm_password",               //"Confirm Password"
		"title_time_range",               //"Select Time Range"
		"time_range_60",                  //"1 Hour"
		"time_range_360",                 //"6 Hours"
		"time_range_1440",                //"24 Hours"
		"time_range_2880",                //"48 Hours"
		"time_range_10080",               //"1 Week"
		"start_date",                     //"Start Date"
		"end_date",                       //"End Date"
		"apply",                          //"Apply"
		"reset_zoom",                     //"Reset Zoom"
		"username",                       //"Username"
		"password",                       //"Password"
		"in_stock",                       //"In Stock"
		"out_stock",                      //"Out of Stock"
		"search_strains",                 //"Search strains"
		"add_new_strain",                 //"Add New Strain"
		"strain_name",                    //"Strain Name"
		"strain_name_placeholder",        //"Enter strain name"
		"url",                            //"URL"
		"strain_url_placeholder",         //"Enter strain url"
		"breeder",                        //"Breeder"
		"add_new_breeder",                //"Add New Breeder"
		"new_breeder_placeholder",        //"Enter new breeder name"
		"i_s_ratio",                      //"Indica / Sativa Ratio"
		"autoflower",                     //"Autoflower"
		"yes",                            //"Yes"
		"no",                             //"No"
		"cycle_time",                     //"Cycle Time"
		"cycle_time_desc",                //"Cycle time in days.  For Autoflowers, total runtime. For Photos, flower runtime."
		"seed_count",                     //"Seed Count"
		"seed_count_placeholder",         //"Enter seed count"
		"description",                    //"Description"
		"strain_description_placeholder", //"Enter strain description"
		"add_strain",                     //"Add Strain"
		"edit_strain",                    //"Edit Strain"
		"new_breeder_name",               //"New Breeder Name"
		"save_changes",                   //"Save Changes"
		"delete_strain",                  //"Delete Strain"
		"strain_add_fail",                //"Failed to add strain."
		"strain_add_error",               //"Error adding strain", //"
		"strain_update_fail",             //"Failed to update strain"
		"strain_update_error",            //"Error updating strain", //"
		"strain_delete_fail",             //"Failed to delete strain"
		"strain_delete_error",            //"Error deleting strain", //"
		"title_is",                       //"I/S"
		"title_auto",                     //"Auto"

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
