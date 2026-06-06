package utils

// Phase 6a tests for validation.go. Pure functions; table-driven.

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateStringLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   string
		maxLen  int
		wantErr bool
	}{
		{"empty under limit", "", 10, false},
		{"value within limit", "hello", 10, false},
		{"value at limit", "abcdefghij", 10, false},
		{"value over limit", "abcdefghijk", 10, true},
		{"value far over limit", strings.Repeat("x", 1024), 10, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateStringLength("field", tc.value, tc.maxLen)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "field")
				assert.Contains(t, err.Error(), "exceeds maximum length")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequiredString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		value     string
		maxLen    int
		wantErr   bool
		errSubstr string
	}{
		{"non-empty under limit", "hi", 10, false, ""},
		{"empty rejected", "", 10, true, "required"},
		{"whitespace-only rejected", "   ", 10, true, "required"},
		{"tabs and newlines rejected", "\t\n", 10, true, "required"},
		{"over limit rejected", strings.Repeat("x", 11), 10, true, "exceeds maximum length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateRequiredString("name", tc.value, tc.maxLen)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errSubstr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFiniteFloat64(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   float64
		wantErr bool
	}{
		{"zero", 0, false},
		{"positive", 21.5, false},
		{"negative", -3.14, false},
		{"max finite", math.MaxFloat64, false},
		{"smallest positive", math.SmallestNonzeroFloat64, false},
		{"NaN rejected", math.NaN(), true},
		{"+Inf rejected", math.Inf(1), true},
		{"-Inf rejected", math.Inf(-1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateFiniteFloat64("value", tc.value)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "value")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStreamURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		url       string
		wantErr   bool
		errSubstr string
	}{
		{"http allowed", "http://example.com/stream", false, ""},
		{"https allowed", "https://example.com/stream", false, ""},
		{"rtsp allowed", "rtsp://example.com/stream", false, ""},
		{"rtmp allowed", "rtmp://example.com/stream", false, ""},
		{"uppercase scheme allowed", "HTTPS://example.com/stream", false, ""},
		{"empty rejected", "", true, "required"},
		{"whitespace rejected", "   ", true, "required"},
		{"unsupported scheme rejected", "ftp://example.com", true, "scheme must be"},
		{"missing host rejected", "http://", true, "must include a host"},
		{"overlong rejected", "https://example.com/" + strings.Repeat("a", MaxURLLength), true, "exceeds maximum length"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateStreamURL(tc.url)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errSubstr != "" {
					assert.Contains(t, err.Error(), tc.errSubstr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNormalizeWebURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty unchanged", "", ""},
		{"whitespace collapses to empty", "   ", ""},
		{"schemeless host gets https", "www.seedfinder.eu", "https://www.seedfinder.eu"},
		{"schemeless host+path gets https", "seedfinder.eu/en/strain/x", "https://seedfinder.eu/en/strain/x"},
		{"surrounding whitespace trimmed then prefixed", "  www.seedbank.com/strain  ", "https://www.seedbank.com/strain"},
		{"existing https unchanged", "https://example.com/x", "https://example.com/x"},
		{"existing http unchanged", "http://example.com/x", "http://example.com/x"},
		{"uppercase scheme unchanged", "HTTPS://example.com", "HTTPS://example.com"},
		{"ftp scheme left for validator to reject", "ftp://example.com", "ftp://example.com"},
		{"javascript scheme left for validator to reject", "javascript:alert(1)", "javascript:alert(1)"},
		{"rtsp scheme unchanged", "rtsp://example.com/s", "rtsp://example.com/s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, NormalizeWebURL(tc.in))
		})
	}
}

// TestNormalizeWebURL_FeedsValidateWebURL proves the backward-compat contract:
// legacy schemeless URLs pass ValidateWebURL after normalization, while
// dangerous/non-http schemes are still rejected (normalization must not rescue
// them).
func TestNormalizeWebURL_FeedsValidateWebURL(t *testing.T) {
	t.Parallel()
	t.Run("schemeless legacy url becomes valid", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateWebURL("url", NormalizeWebURL("www.seedfinder.eu/en/strain/x")))
	})
	t.Run("javascript scheme still rejected", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, ValidateWebURL("url", NormalizeWebURL("javascript:alert(1)")))
	})
	t.Run("ftp scheme still rejected", func(t *testing.T) {
		t.Parallel()
		assert.Error(t, ValidateWebURL("url", NormalizeWebURL("ftp://example.com")))
	})
	t.Run("empty stays allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateWebURL("url", NormalizeWebURL("")))
	})
}
