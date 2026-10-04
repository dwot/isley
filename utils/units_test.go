package utils

import "testing"

func TestNormalizeUnit(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"celsius":    "°C",
		"Celsius":    "°C",
		" CELSIUS ":  "°C",
		"fahrenheit": "°F",
		"Fahrenheit": "°F",
		// Everything else is stored exactly as sent.
		"°C":      "°C",
		"°F":      "°F",
		"C":       "C",
		"degC":    "degC",
		"celcius": "celcius",
		"%":       "%",
		"PPM":     "PPM",
		" lux ":   " lux ",
		"":        "",
	}
	for in, want := range cases {
		if got := NormalizeUnit(in); got != want {
			t.Errorf("NormalizeUnit(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsCelsius(t *testing.T) {
	t.Parallel()
	for _, u := range []string{"°C", "C", "degC", "celsius", "Celsius"} {
		if !IsCelsius(u) {
			t.Errorf("IsCelsius(%q) = false, want true", u)
		}
	}
	// Everything else stays Fahrenheit, as before, including an empty unit.
	for _, u := range []string{"°F", "F", "fahrenheit", ""} {
		if IsCelsius(u) {
			t.Errorf("IsCelsius(%q) = true, want false", u)
		}
	}
}
