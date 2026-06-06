package utils

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// schemePrefix matches a leading "scheme:" per RFC 3986
// (ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) ":"). Used to decide whether a
// URL already carries a scheme before NormalizeWebURL prepends one.
var schemePrefix = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// Maximum string lengths for text inputs across the application.
const (
	MaxNameLength        = 255
	MaxDescriptionLength = 2000
	MaxNotesLength       = 5000
	MaxURLLength         = 2048
	MaxUnitLength        = 50
	MaxDeviceLength      = 255
	MaxSourceLength      = 255
	MaxTypeLength        = 255
	MaxLogLevelLength    = 20
)

// ValidateStringLength checks that a string does not exceed the given maximum
// length. It returns a user-friendly error message when the limit is exceeded.
func ValidateStringLength(field, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	return nil
}

// ValidateRequiredString checks that a string is not empty and does not exceed
// the given maximum length.
func ValidateRequiredString(field, value string, maxLen int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return ValidateStringLength(field, value, maxLen)
}

// ValidateDate parses a date string emitted by the UI's <input type=date> or
// <input type=datetime-local> controls. Empty input is allowed; pair with
// ValidateRequiredString when the field is required.
func ValidateDate(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	for _, layout := range []string{LayoutDate, LayoutDateTimeLocal, LayoutDB, time.RFC3339} {
		if _, err := time.Parse(layout, value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s is not a valid date", field)
}

// ValidateFiniteFloat64 checks that a float64 value is finite (not NaN or Inf).
func ValidateFiniteFloat64(field string, value float64) error {
	if math.IsNaN(value) {
		return fmt.Errorf("%s must be a valid number (NaN is not allowed)", field)
	}
	if math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number (Infinity is not allowed)", field)
	}
	return nil
}

// ValidateWebURL checks that a URL string is well-formed and uses http or
// https. Empty input is allowed (returns nil) — wrap with ValidateRequiredString
// if the field is required.
func ValidateWebURL(field, rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	if len(rawURL) > MaxURLLength {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, MaxURLLength)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL", field)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%s must use http or https", field)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", field)
	}
	return nil
}

// NormalizeWebURL trims rawURL and, when it is non-empty and carries no scheme,
// prepends "https://". This keeps legacy schemeless values (e.g.
// "www.seedfinder.eu/x", saved before URL validation existed) usable instead of
// failing every strain add/update. Values that already start with a scheme —
// including non-http ones like "javascript:" or "ftp://" — are returned
// untouched so ValidateWebURL still rejects them (security intent preserved).
//
// Edge case: a bare "host:port/path" with no scheme is left as-is (the leading
// "host:" looks like a scheme), so it is not rescued; this is rare for the
// stored seed-bank URLs this targets and keeps the rule simple and safe.
func NormalizeWebURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return trimmed
	}
	if schemePrefix.MatchString(trimmed) {
		return trimmed
	}
	return "https://" + trimmed
}

// ValidateStreamURL checks that a URL string is well-formed and uses an
// acceptable scheme (http or https, or rtsp/rtmp for camera streams).
func ValidateStreamURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("URL is required")
	}
	if len(rawURL) > MaxURLLength {
		return fmt.Errorf("URL exceeds maximum length of %d characters", MaxURLLength)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL is not valid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "rtsp", "rtmp":
		// acceptable schemes
	default:
		return fmt.Errorf("URL scheme must be http, https, rtsp, or rtmp")
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}
