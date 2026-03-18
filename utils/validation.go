package utils

import (
	"fmt"
	"math"
	"net/url"
	"strings"
)

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
