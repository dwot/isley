package utils

import "strings"

// NormalizeUnit maps the plain-word temperature units, which are easier for a
// device to send than a degree sign, to the symbols Isley stores and displays.
// Any other unit is returned exactly as sent.
func NormalizeUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "celsius":
		return "°C"
	case "fahrenheit":
		return "°F"
	}
	return unit
}

// IsCelsius reports whether a temperature unit means Celsius: any unit
// containing "C", as before, or the word "celsius" in any case, which that
// rule alone read as Fahrenheit.
func IsCelsius(unit string) bool {
	return NormalizeUnit(unit) == "°C" || strings.Contains(unit, "C")
}
