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
	bundle  *i18n.Bundle
	allKeys []string // all message IDs discovered from the English locale
	mu      sync.RWMutex
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

	// Discover all translation keys from the English locale file so
	// GetTranslations() doesn't need a hardcoded key list.
	enData, err := embeddedLocales.ReadFile("locales/en.yaml")
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to read en.yaml for key discovery")
	}
	var enMap map[string]interface{}
	if err := yaml.Unmarshal(enData, &enMap); err != nil {
		logger.Log.WithError(err).Fatal("Failed to parse en.yaml for key discovery")
	}
	keys := make([]string, 0, len(enMap))
	for k := range enMap {
		keys = append(keys, k)
	}
	TranslationService.allKeys = keys
	logger.Log.Infof("Discovered %d translation keys from en.yaml", len(keys))
}

func (i *I18nManager) GetTranslations(lang string) map[string]string {
	localizer := i18n.NewLocalizer(i.bundle, lang, "en")
	enFallback := i18n.NewLocalizer(i.bundle, "en")

	out := make(map[string]string, len(i.allKeys))
	for _, key := range i.allKeys {
		translation, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: key})
		if err != nil || translation == "" {
			translation, _ = enFallback.Localize(&i18n.LocalizeConfig{MessageID: key})
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
		for _, supported := range AvailableLanguages {
			if supported == queryLang {
				lang = queryLang
				sessions.Default(c).Set("lang", lang)
				sessions.Default(c).Save()
				break
			}
		}
	}
	return lang
}
