package utils

// Phase 6a tests for i18n.go. The package's Init function loads embedded
// YAML locale files; we test the loaded state plus the GetTranslations
// fallback behavior.

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initI18nOnce gates the one-time Init("en") call so parallel tests
// don't race on TranslationService and AvailableLanguages. Init reads
// embedded YAML files and reassigns both globals; calling it from two
// goroutines concurrently is unsafe even though the result is the same.
var initI18nOnce sync.Once

// initI18nForTests boots the package translation service exactly once
// for the test binary. Subsequent tests see a fully-populated
// TranslationService and only read it (read-only access is safe under
// t.Parallel()).
func initI18nForTests(t *testing.T) {
	t.Helper()
	silenceImageLogger() // shared "no-op logger" helper from image_test.go
	initI18nOnce.Do(func() { Init("en") })
}

func TestI18n_InitPopulatesAvailableLanguages(t *testing.T) {
	t.Parallel()
	initI18nForTests(t)

	// The repo ships locales/{en,de,es,fr}.yaml. Init reads the embedded
	// directory and exposes the list via AvailableLanguages.
	require.NotEmpty(t, AvailableLanguages)
	assert.Contains(t, AvailableLanguages, "en")
}

// TestI18n_GetTranslations_HasEnglishKeys confirms a non-empty key set
// was discovered from en.yaml during Init.
func TestI18n_GetTranslations_HasEnglishKeys(t *testing.T) {
	t.Parallel()
	initI18nForTests(t)

	got := TranslationService.GetTranslations("en")
	require.NotEmpty(t, got, "GetTranslations should return at least one key for English")

	// Spot-check a known message ID. Handlers/api_errors.go uses
	// api_database_error throughout, so it must exist.
	val, ok := got["api_database_error"]
	require.True(t, ok, "api_database_error should be a known key")
	assert.NotEmpty(t, val)
}

// TestI18n_GetTranslations_FallsBackToEnglish verifies that when a
// requested locale is missing a key (or the locale itself is unknown),
// the English value is returned rather than an empty string.
func TestI18n_GetTranslations_FallsBackToEnglish(t *testing.T) {
	t.Parallel()
	initI18nForTests(t)

	// "xx-YY" is not a registered locale; localizer falls back to en.
	got := TranslationService.GetTranslations("xx-YY")
	require.NotEmpty(t, got)

	en := TranslationService.GetTranslations("en")
	for k, v := range en {
		// Every English value should appear (or be replicated) under the
		// unknown locale's map. Empty values are tolerated only when
		// English itself was empty for that key.
		gotVal, ok := got[k]
		assert.Truef(t, ok, "fallback map missing key %q", k)
		if v == "" {
			continue
		}
		assert.NotEmptyf(t, gotVal, "key %q must fall back to English (was empty)", k)
	}
}

// TestI18n_GetTranslations_NonEnglishLocaleHasOverrides verifies a
// known non-English locale (de) returns at least one value that differs
// from the English equivalent. This catches regressions where the
// fallback path swallows real translations.
func TestI18n_GetTranslations_NonEnglishLocaleHasOverrides(t *testing.T) {
	t.Parallel()
	initI18nForTests(t)
	if !contains(AvailableLanguages, "de") {
		t.Skip("de.yaml not bundled in this build")
	}

	en := TranslationService.GetTranslations("en")
	de := TranslationService.GetTranslations("de")

	differs := 0
	for k, enVal := range en {
		if deVal, ok := de[k]; ok && enVal != "" && deVal != "" && deVal != enVal {
			differs++
		}
	}
	assert.Greater(t, differs, 0, "de.yaml should produce at least one non-English value")
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
