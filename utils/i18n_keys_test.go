package utils

// Guards against the class of bug where JavaScript or templates reference a
// translation key that no locale defines. uiMessages.t() used to return the
// key itself for a missing entry, so dialogs and toasts silently showed raw
// keys such as "confirm" or "confirm_delete_strain".

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// uiMessages.t('some_key' ...) / window.uiMessages.t("some_key" ...)
var jsTranslationKeyRe = regexp.MustCompile(`uiMessages\.t\(\s*['"]([A-Za-z0-9_]+)['"]`)

func loadLocaleKeys(t *testing.T, lang string) map[string]struct{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("locales", lang+".yaml"))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, yaml.Unmarshal(data, &m))
	keys := make(map[string]struct{}, len(m))
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

// Every key used from JS/templates via uiMessages.t() must exist in en.yaml.
func TestI18n_JSReferencedKeysExistInEnglish(t *testing.T) {
	t.Parallel()
	en := loadLocaleKeys(t, "en")

	missing := map[string][]string{}
	for _, root := range []string{filepath.Join("..", "web", "static", "js"), filepath.Join("..", "web", "templates")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !(strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".html")) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range jsTranslationKeyRe.FindAllStringSubmatch(string(b), -1) {
				if _, ok := en[m[1]]; !ok {
					missing[m[1]] = append(missing[m[1]], filepath.ToSlash(path))
				}
			}
			return nil
		})
		require.NoError(t, err)
	}

	keys := make([]string, 0, len(missing))
	for k := range missing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		assert.Failf(t, "missing translation key", "%q is used in %v but is not defined in locales/en.yaml", k, missing[k])
	}
}

// ui-messages.js showConfirm() needs these for its default title and buttons.
func TestI18n_ConfirmDialogDefaultKeysExist(t *testing.T) {
	t.Parallel()
	for _, lang := range []string{"en", "de", "es", "fr"} {
		keys := loadLocaleKeys(t, lang)
		for _, k := range []string{"confirm", "cancel", "ok", "delete"} {
			assert.Containsf(t, keys, k, "%s.yaml is missing %q", lang, k)
		}
	}
}

// Every locale should define exactly the keys English does.
func TestI18n_LocalesHaveSameKeysAsEnglish(t *testing.T) {
	t.Parallel()
	en := loadLocaleKeys(t, "en")
	for _, lang := range []string{"de", "es", "fr"} {
		other := loadLocaleKeys(t, lang)
		for k := range en {
			assert.Containsf(t, other, k, "%s.yaml is missing key %q", lang, k)
		}
		for k := range other {
			assert.Containsf(t, en, k, "%s.yaml has key %q that en.yaml lacks", lang, k)
		}
	}
}
